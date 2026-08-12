//nolint:wrapcheck // pass-through adapter; errors from backend/db/yt are already contextual
package api

import (
	"context"
	"fmt"
	"os"

	"github.com/EugeneShtoka/yt-tui/internal/backend/thumbs"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ── VideoBackend ─────────────────────────────────────────────────────────────
// HasLocalVideo, which VideoBackend also requires, is defined once in
// inproc_library.go (it's the same lookup LibraryBackend needs).

func (p *InProc) VideoDetails(ctx context.Context, videoURL string) (domain.VideoDetails, error) {
	d, err := p.yt.VideoDetails(ctx, videoURL)
	if err != nil {
		return d, err
	}
	// Lazy backfill: a full fetch carries the exact upload date, so replace the
	// approximate flat-listing date the videos row was stored with. Best-effort —
	// a failed write must not fail the fetch the caller asked for.
	if d.ID != "" && len(d.UploadDate) == 8 {
		if uerr := p.db.UpdateVideoUploadDate(ctx, d.ID, d.UploadDate); uerr != nil {
			debug.Log("VideoDetails: backfill upload_date %s: %v", d.ID, uerr)
		}
	}
	p.maybeSaveTranscript(ctx, d.ID, videoURL)
	return d, nil
}

// maybeSaveTranscript builds a video's transcript note when transcript saving is
// enabled and the video is eligible (same newest-N ∪ recommended set as
// thumbnails) and no note exists yet. Runs in the background so requesting video
// info never blocks on the extra yt-dlp call. Best-effort.
func (p *InProc) maybeSaveTranscript(ctx context.Context, videoID, videoURL string) {
	if !p.cfg.SaveTranscript || p.transcripts == nil || !p.transcripts.MarkdownEnabled() {
		return
	}
	if videoID == "" || p.transcripts.HasMarkdown(videoID) {
		return
	}
	eligible, err := p.db.ThumbnailEligible(ctx, videoID, p.cfg.ThumbnailsPerChannel)
	if err != nil || !eligible {
		return
	}
	// Deliberately detached from the request ctx: this fire-and-forget note build
	// must outlive the VideoDetails call that spawned it (that ctx cancels the
	// moment the request returns), so it gets a fresh background context. Routing
	// through the shared build means that if the user also opens the transcript,
	// the two collapse into one fetch instead of racing. The shared build tracks
	// itself on transcriptWG so shutdown (WaitEnrichment) can drain it.
	go func() { //nolint:gosec // G118: Background is intentional — the goroutine outlives the request
		if !p.buildTranscriptNoteShared(context.Background(), videoID, videoURL) {
			debug.Log("VideoDetails: transcript note %s not built", videoID)
		}
	}()
}

// buildTranscriptNoteShared collapses concurrent note builds for the same video
// (the interactive GetTranscript and the background maybeSaveTranscript) into a
// single yt-dlp fetch via singleflight, so opening a fresh video can't fire a
// burst of duplicate subtitle requests that YouTube rate-limits. The build runs
// under a detached context so one caller giving up (e.g. an interactive request
// whose client timed out) doesn't abort it for the others — it finishes and warms
// the store for the next open. Returns false if the caller's ctx ends before the
// shared build completes.
func (p *InProc) buildTranscriptNoteShared(ctx context.Context, videoID, videoURL string) bool {
	// Register the build on transcriptWG synchronously here, before spawning the
	// goroutine — a WaitGroup.Add must happen-before Wait, and singleflight's own
	// DoChan worker runs on a goroutine we don't sequence, so adding inside it
	// would race WaitEnrichment (H-1). The goroutine below carries the Done and
	// blocks on the shared build to completion, so the counter covers the whole
	// build even when this caller's ctx cancels first and we return early.
	p.transcriptWG.Add(1)
	done := make(chan bool, 1) // buffered: the goroutine must never block sending after an early return
	go func() {
		defer p.transcriptWG.Done()
		v, _, _ := p.transcriptSF.Do(videoID, func() (any, error) {
			return p.buildTranscriptNote(context.Background(), videoID, videoURL), nil
		})
		built, _ := v.(bool)
		done <- built
	}()
	select {
	case built := <-done:
		return built
	case <-ctx.Done():
		return false // caller gave up; the tracked goroutine finishes the build in the background
	}
}

// buildTranscriptNote performs the unified fetch (metadata + .srt) for a video
// and writes its canonical markdown note. Returns false when markdown is off,
// the fetch fails, or the video has no captions (so no empty note is written).
// It intentionally does not touch the DB details cache — that is the enrichment
// and VideoDetails paths' job; this only produces the note.
func (p *InProc) buildTranscriptNote(ctx context.Context, videoID, videoURL string) bool {
	if p.transcripts == nil || !p.transcripts.MarkdownEnabled() || videoID == "" {
		return false
	}
	if videoURL == "" {
		videoURL = "https://www.youtube.com/watch?v=" + videoID
	}
	d, err := p.yt.VideoDetailsWithTranscript(ctx, videoURL, p.cfg.SubtitleLangsArg(), p.cfg.SponsorBlockArg(), p.transcripts.OutputTemplate())
	if err != nil {
		debug.Log("buildTranscriptNote: fetch %s: %v", videoID, err)
		return false
	}
	var imagePath string
	if p.thumbs != nil && p.thumbs.Has(videoID) {
		imagePath = p.thumbs.Path(videoID)
	}
	built, werr := p.transcripts.BuildAndWriteNote(videoID, videoURL, d, p.cfg.SubtitleLangs, imagePath)
	if werr != nil {
		debug.Log("buildTranscriptNote: write %s: %v", videoID, werr)
	}
	return built
}

// EligibleThumbnailIDs returns the video IDs whose thumbnails are eligible for
// caching, using the daemon's own ThumbnailsPerChannel bound. It is the set a
// client pre-warms its local cache with.
func (p *InProc) EligibleThumbnailIDs(ctx context.Context) (map[string]bool, error) {
	return p.db.ThumbnailEligibleIDs(ctx, p.cfg.ThumbnailsPerChannel)
}

// ThumbnailsEnabled reports whether the daemon serves thumbnails — i.e. its
// on-disk store was constructed. The composition root uses it to decide the
// client's thumbnail egress: when false, the client fetches the CDN itself
// rather than routing a request the daemon can't satisfy through the backend.
func (p *InProc) ThumbnailsEnabled() bool { return p.thumbs != nil }

// GetThumbnail serves a video's thumbnail from the on-disk cache, fetching on a
// miss. It persists the fetched image only when the video is thumbnail-eligible
// (newest-N of a subscribed channel or in the recommended feed) — the lazy
// counterpart to the background thumbnail pass. A miss returns (nil,false,nil)
// so the caller can fall back to its own fetch.
func (p *InProc) GetThumbnail(ctx context.Context, videoID, fallbackURL string) ([]byte, bool, error) {
	if p.thumbs == nil || videoID == "" {
		return nil, false, nil
	}
	if data, ok, err := p.thumbs.Get(videoID); err == nil && ok {
		return data, true, nil // stored images are already cropped (Put crops on write)
	}
	url := fallbackURL
	if url == "" {
		url = thumbs.URLFor(videoID)
	}
	data, err := p.thumbs.Download(ctx, url)
	if err != nil {
		debug.Log("GetThumbnail: fetch %s: %v", videoID, err)
		return nil, false, nil
	}
	// Persist only when thumbnail caching is enabled and this video is eligible
	// (newest-N of a subscribed channel or in the recommended feed). Put crops on
	// write, so serve the stored (clean) bytes back on success.
	if p.cfg.ThumbnailsPerChannel > 0 {
		if eligible, eerr := p.db.ThumbnailEligible(ctx, videoID, p.cfg.ThumbnailsPerChannel); eerr == nil && eligible {
			if perr := p.thumbs.Put(videoID, data); perr != nil {
				debug.Log("GetThumbnail: store %s: %v", videoID, perr)
			} else if stored, ok, gerr := p.thumbs.Get(videoID); gerr == nil && ok {
				return stored, true, nil
			}
		}
	}
	// Not persisted (ineligible, or store failed): crop here so the render layer
	// still receives a clean image and never has to crop itself.
	if cropped, ok := thumbs.CropLetterboxJPEG(data); ok {
		data = cropped
	}
	return data, true, nil
}

func (p *InProc) GetVideoDetailsCache(ctx context.Context, videoID string) (domain.CachedDetails, bool, error) {
	return p.db.GetVideoDetailsCache(ctx, videoID)
}

func (p *InProc) VideoPosition(ctx context.Context, videoID string) (int64, bool, error) {
	return p.db.VideoPosition(ctx, videoID)
}

func (p *InProc) AllVideoPositions(ctx context.Context) (map[string]int64, error) {
	return p.db.AllVideoPositions(ctx)
}

func (p *InProc) UpsertVideo(ctx context.Context, id, title, channel, channelID string, duration int, viewCount int64, uploadDate, url string) error {
	return p.db.UpsertVideo(ctx, id, title, channel, channelID, duration, viewCount, uploadDate, url)
}

func (p *InProc) SetVideoStatus(ctx context.Context, id string, status domain.VideoStatus) error {
	return p.db.SetVideoStatus(ctx, id, status)
}

func (p *InProc) SaveVideoPosition(ctx context.Context, videoID string, ms int64) error {
	if err := p.db.SaveVideoPosition(ctx, videoID, ms); err != nil {
		return err
	}
	p.maybeAutoRemoveWatchLater(ctx, videoID, ms)
	return nil
}

// maybeAutoRemoveWatchLater removes a video from Watch Later once it has been
// watched at least WatchLaterAutoRemovePercent of its duration (0 disables).
// Best-effort — never fails the position save. The membership guard both avoids
// a spurious YouTube removal for a video that isn't queued and keeps the
// end-of-playback position-save burst from firing more than once (the removal
// leaves the video absent from both stores).
func (p *InProc) maybeAutoRemoveWatchLater(ctx context.Context, videoID string, ms int64) {
	pct := p.cfg.WatchLaterAutoRemovePercent
	if pct <= 0 || ms <= 0 {
		return
	}
	durSec, err := p.db.VideoDuration(ctx, videoID)
	if err != nil || durSec <= 0 {
		return
	}
	// watched% = ms / (durSec*1000); trigger when ms >= pct% of duration.
	if ms*100 < int64(durSec)*1000*int64(pct) {
		return
	}
	inWL, err := p.db.InWatchLater(ctx, videoID, domain.WatchLaterYTID, domain.WatchLaterPlaylistName)
	if err != nil || !inWL {
		return
	}
	if err := p.RemoveFromWatchLater(ctx, videoID); err != nil {
		debug.Log("auto-remove watch later %s: %v", videoID, err)
	}
}

func (p *InProc) DeleteVideoPosition(ctx context.Context, videoID string) error {
	return p.db.DeleteVideoPosition(ctx, videoID)
}

func (p *InProc) UpdateLastPosition(ctx context.Context, id string, ms int64) error {
	return p.db.UpdateLastPosition(ctx, id, ms)
}

func (p *InProc) SaveVideoDetailsCache(ctx context.Context, videoID, description, thumbnailURL string, subscribers int64) error {
	return p.db.SaveVideoDetailsCache(ctx, videoID, description, thumbnailURL, subscribers)
}

func (p *InProc) SaveVideoChapters(ctx context.Context, videoID string, chapters []domain.Chapter) error {
	return p.db.SaveVideoChapters(ctx, videoID, chapters)
}

func (p *InProc) SaveVideoSBSegments(ctx context.Context, videoID string, segs []domain.SBSegment) error {
	return p.db.SaveVideoSBSegments(ctx, videoID, segs)
}

func (p *InProc) SaveVideoLinks(ctx context.Context, videoID string, links []domain.Link) error {
	return p.db.SaveVideoLinks(ctx, videoID, links)
}

func (p *InProc) ClearVideoDetailsCache(ctx context.Context) error {
	return p.db.ClearVideoDetailsCache(ctx)
}

// GetTranscript returns a video's transcript as display-ready text. It prefers
// the canonical markdown note (frontmatter and image embed stripped), falls back
// to a legacy raw .srt/.txt, and on a full miss builds the note from a unified
// yt-dlp fetch before serving it. Returns ("", false) when no transcript is
// available. videoURL drives the on-miss fetch.
func (p *InProc) GetTranscript(ctx context.Context, videoID, videoURL string) (string, bool, error) {
	if p.transcripts == nil || videoID == "" {
		return "", false, nil
	}
	if text, ok := p.transcripts.ReadMarkdown(videoID); ok {
		return text, true, nil
	}
	if text, ok := p.transcripts.Read(videoID); ok { // legacy raw transcript
		return text, true, nil
	}
	if videoURL == "" {
		videoURL = "https://www.youtube.com/watch?v=" + videoID
	}
	// Miss: build the note (unified fetch + .srt), then serve it. When markdown is
	// disabled, buildTranscriptNote is a no-op and we fall through to the raw fetch.
	if p.buildTranscriptNoteShared(ctx, videoID, videoURL) {
		if text, ok := p.transcripts.ReadMarkdown(videoID); ok {
			return text, true, nil
		}
	}
	if p.transcripts.MarkdownEnabled() {
		// buildTranscriptNote already fetched; if it produced a raw .srt but no note
		// (e.g. it wrote captions we then couldn't build), serve that.
		if text, ok := p.transcripts.Read(videoID); ok {
			return text, true, nil
		}
		return "", false, nil
	}
	// Markdown disabled (the md dir couldn't be created): serve the raw .srt.
	if err := p.yt.FetchTranscript(ctx, videoURL, p.cfg.SubtitleLangsArg(), p.transcripts.OutputTemplate()); err != nil {
		debug.Log("GetTranscript: fetch %s: %v", videoID, err)
		return "", false, nil
	}
	text, ok := p.transcripts.Read(videoID)
	return text, ok, nil
}

// DeleteVideoCompletely removes every trace of a video — local file+row,
// history, saved position — as the single server-side operation Remote's RPC
// delegates to. (M-23)
func (p *InProc) DeleteVideoCompletely(ctx context.Context, videoID string) error {
	// Remove the on-disk media file first (best-effort): the DB rows below are the
	// authoritative record, so an orphaned file is self-healing (startup
	// reconciliation drops rows whose file is gone) while an orphaned row is not.
	if lv, ok, err := p.db.HasLocalVideo(ctx, videoID); err == nil && ok && lv.FilePath != "" {
		_ = os.Remove(lv.FilePath) // best-effort; ignore if already gone
	}
	// Delete the local-file row, history, and saved position atomically so a
	// mid-sequence failure can't wedge the record half-deleted (M-3).
	if err := p.db.DeleteVideoRecords(ctx, videoID); err != nil {
		return fmt.Errorf("DeleteVideoCompletely: %w", err)
	}
	if p.thumbs != nil {
		_ = p.thumbs.Delete(videoID) // best-effort cached-thumbnail cleanup
	}
	if p.transcripts != nil {
		_ = p.transcripts.Delete(videoID)         // best-effort raw .srt/.txt cleanup
		_ = p.transcripts.DeleteMarkdown(videoID) // best-effort note cleanup
	}
	return nil
}

func (p *InProc) ResolveSource(ctx context.Context, videoID, fallbackURL string) (PlayableSource, error) {
	lv, ok, err := p.db.HasLocalVideo(ctx, videoID)
	if err != nil {
		return PlayableSource{}, fmt.Errorf("ResolveSource: %w", err)
	}
	if ok && lv.FilePath != "" {
		return PlayableSource{URI: lv.FilePath}, nil
	}
	return PlayableSource{URI: fallbackURL}, nil
}
