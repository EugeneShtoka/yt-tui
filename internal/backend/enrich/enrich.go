// Package enrich runs the background pass that upgrades the cheap, approximate
// metadata from yt-dlp's flat channel listing into full, exact details. The
// flat listing yields no real upload date (yt-dlp synthesizes one from "N years
// ago" text, collapsing whole back-catalogs onto a single day); a full
// per-video fetch returns the true date plus description, chapters and links.
//
// The pass is ordered by value: thumbnails first (fast CDN fetches, most visible
// win), then recommended-feed details (small, and it makes the recommended age
// filter act on real dates), then the long subscribed-channel details grind. It
// is resumable — every step skips work already cached — and side-effect-safe, so
// an abrupt process exit merely pauses it.
package enrich

import (
	"context"
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/backend/thumbs"
	"github.com/EugeneShtoka/yt-tui/internal/backend/transcripts"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/media"
)

// thumbnailDelay paces thumbnail downloads. They hit Google's image CDN (not the
// yt-dlp scraping path YouTube throttles), so a short fixed delay is plenty; the
// configurable, longer EnrichmentDelaySeconds governs the yt-dlp detail fetches.
const thumbnailDelay = 300 * time.Millisecond

// Catalog is the slice of the DB the enricher reads and writes. *db.DB satisfies it.
type Catalog interface {
	RecommendedVideosWithoutDetails(ctx context.Context) ([]domain.VideoRef, error)
	SubscribedVideosWithoutDetails(ctx context.Context, limit int) ([]domain.VideoRef, error)
	ThumbnailEligibleIDs(ctx context.Context, perChannel int) (map[string]bool, error)
	UpdateVideoUploadDate(ctx context.Context, videoID, uploadDate string) error
	SaveVideoDetailsCache(ctx context.Context, videoID, description, thumbnailURL string, subscribers int64) error
	SaveVideoChapters(ctx context.Context, videoID string, chapters []domain.Chapter) error
	SaveVideoSBSegments(ctx context.Context, videoID string, segs []domain.SBSegment) error
	SaveVideoLinks(ctx context.Context, videoID string, links []domain.Link) error
}

// Fetcher performs full per-video yt-dlp operations. *youtube.Client satisfies it.
type Fetcher interface {
	VideoDetails(ctx context.Context, videoURL string) (domain.VideoDetails, error)
	// VideoDetailsWithTranscript fetches metadata AND writes the .srt sidecar in a
	// single call — the transcript pass uses it to build a note from one fetch.
	VideoDetailsWithTranscript(ctx context.Context, videoURL, langs, sbCats, outTemplate string) (domain.VideoDetails, error)
}

// Params are the tunables sourced from config.
type Params struct {
	DelaySeconds         int      // sleep between yt-dlp calls; <=0 disables the yt-dlp passes (details, transcripts)
	ThumbnailsPerChannel int      // newest videos per subscribed channel eligible for thumbnails/transcripts; also the eligibility N
	SaveTranscript       bool     // build transcript notes for the eligible set
	SubtitleLangs        []string // acceptable transcript languages, in priority order (empty → English)
	SponsorBlockCats     string   // --sponsorblock-mark category list; the spans excised from each note (empty → none)
}

// Enricher owns one background enrichment run. Construct with New.
type Enricher struct {
	cat   Catalog
	yt    Fetcher
	th    *thumbs.Store
	tr    *transcripts.Store
	p     Params
	sleep func(context.Context, time.Duration) bool // seam; real impl honors ctx cancellation

	// Which passes are enabled, decided once in New so Run doesn't re-derive the
	// gate expressions (a divergence risk if a gate ever gains a condition).
	thumbsOn, detailsOn, transcriptsOn bool
}

// New returns an Enricher, or nil when there is nothing to do. The details pass
// needs DelaySeconds>0; the thumbnail pass needs a cache (th) and
// ThumbnailsPerChannel>0; the transcript pass needs a store (tr), SaveTranscript,
// and DelaySeconds>0 (it makes yt-dlp calls that must be paced).
func New(cat Catalog, yt Fetcher, th *thumbs.Store, tr *transcripts.Store, p Params) *Enricher {
	thumbsOn := th != nil && p.ThumbnailsPerChannel > 0
	detailsOn := p.DelaySeconds > 0
	transcriptsOn := tr != nil && tr.MarkdownEnabled() && p.SaveTranscript && p.DelaySeconds > 0
	if !thumbsOn && !detailsOn && !transcriptsOn {
		return nil
	}
	return &Enricher{
		cat: cat, yt: yt, th: th, tr: tr, p: p, sleep: sleepCtx,
		thumbsOn: thumbsOn, detailsOn: detailsOn, transcriptsOn: transcriptsOn,
	}
}

// Run executes the pass to completion (or until ctx is canceled). Intended to
// be called in its own goroutine.
func (e *Enricher) Run(ctx context.Context) {
	if e.thumbsOn {
		e.runThumbnails(ctx)
	}
	if e.transcriptsOn {
		e.runTranscripts(ctx)
	}
	if e.detailsOn {
		e.runDetails(ctx, "recommended", e.cat.RecommendedVideosWithoutDetails)
		e.runDetails(ctx, "subscribed", func(ctx context.Context) ([]domain.VideoRef, error) {
			return e.cat.SubscribedVideosWithoutDetails(ctx, 0) // 0 = the full grind
		})
	}
	if ctx.Err() == nil {
		debug.Log("enrich: pass complete")
	}
}

// runTranscripts builds the canonical markdown note for every eligible video
// (same set as thumbnails) that doesn't already have one, pacing yt-dlp calls by
// DelaySeconds, then evicts .srt archives no longer eligible. Each note comes
// from one unified fetch (metadata + .srt), whose metadata is also applied to
// the DB — so the later details pass skips these videos.
func (e *Enricher) runTranscripts(ctx context.Context) {
	keep, err := e.cat.ThumbnailEligibleIDs(ctx, e.p.ThumbnailsPerChannel)
	if err != nil {
		debug.Log("enrich: transcript eligible set: %v", err)
		return
	}
	if removed, err := e.tr.Retain(keep); err != nil {
		debug.Log("enrich: transcript GC: %v", err)
	} else if removed > 0 {
		debug.Log("enrich: evicted %d stale transcripts", removed)
	}
	delay := time.Duration(e.p.DelaySeconds) * time.Second
	built := 0
	for id := range keep {
		if ctx.Err() != nil {
			return
		}
		if e.tr.HasMarkdown(id) {
			continue
		}
		d, err := e.yt.VideoDetailsWithTranscript(ctx, watchURL(id), strings.Join(e.p.SubtitleLangs, ","), e.p.SponsorBlockCats, e.tr.OutputTemplate())
		if err != nil {
			debug.Log("enrich: transcript %s: %v", id, err)
		} else {
			e.applyDetails(ctx, id, d) // populate DB so runDetails skips this video
			if e.writeNote(id, d) {
				built++
			}
		}
		if !e.sleep(ctx, delay) {
			return
		}
	}
	debug.Log("enrich: built %d new transcript notes (%d eligible)", built, len(keep))
}

// writeNote assembles and writes the markdown note for a freshly-fetched video,
// reading the transcript from the .srt the unified call just wrote. Returns
// false (building nothing) when the video has no captions, so metadata-only
// videos don't produce an empty note. The assembly itself lives in the
// transcripts store (BuildAndWriteNote); this only resolves the cached thumbnail.
func (e *Enricher) writeNote(id string, d domain.VideoDetails) bool {
	var imagePath string
	if e.th != nil && e.th.Has(id) {
		imagePath = e.th.Path(id)
	}
	built, err := e.tr.BuildAndWriteNote(id, watchURL(id), d, e.p.SubtitleLangs, imagePath)
	if err != nil {
		debug.Log("enrich: write note %s: %v", id, err)
	}
	return built
}

// watchURL builds the canonical watch URL for a video ID.
func watchURL(id string) string { return "https://www.youtube.com/watch?v=" + id }

// runThumbnails caches the thumbnail image for every eligible video that isn't
// already on disk, then evicts any cached image no longer eligible.
func (e *Enricher) runThumbnails(ctx context.Context) {
	keep, err := e.cat.ThumbnailEligibleIDs(ctx, e.p.ThumbnailsPerChannel)
	if err != nil {
		debug.Log("enrich: thumbnail eligible set: %v", err)
		return
	}
	if removed, err := e.th.Retain(keep); err != nil {
		debug.Log("enrich: thumbnail GC: %v", err)
	} else if removed > 0 {
		debug.Log("enrich: evicted %d stale thumbnails", removed)
	}
	fetched := 0
	for id := range keep {
		if ctx.Err() != nil {
			return
		}
		if e.th.Has(id) {
			continue
		}
		data, err := e.th.Download(ctx, thumbs.URLFor(id))
		if err != nil {
			debug.Log("enrich: thumbnail %s: %v", id, err)
			continue
		}
		if err := e.th.Put(id, data); err != nil { // Put crops letterbox on write
			debug.Log("enrich: thumbnail store %s: %v", id, err)
			continue
		}
		fetched++
		if !e.sleep(ctx, thumbnailDelay) {
			return
		}
	}
	debug.Log("enrich: cached %d new thumbnails (%d eligible)", fetched, len(keep))
}

// runDetails fetches full metadata for each ref in the batch, pacing yt-dlp
// calls by DelaySeconds. list is a thunk so the (possibly large) query runs at
// step time, not up front.
func (e *Enricher) runDetails(ctx context.Context, label string, list func(context.Context) ([]domain.VideoRef, error)) {
	refs, err := list(ctx)
	if err != nil {
		debug.Log("enrich: %s batch: %v", label, err)
		return
	}
	if len(refs) == 0 {
		return
	}
	delay := time.Duration(e.p.DelaySeconds) * time.Second
	done := 0
	for _, r := range refs {
		if ctx.Err() != nil {
			return
		}
		if r.URL == "" {
			continue
		}
		d, err := e.yt.VideoDetails(ctx, r.URL)
		if err != nil {
			debug.Log("enrich: %s details %s: %v", label, r.ID, err)
		} else {
			e.applyDetails(ctx, r.ID, d)
			done++
		}
		if !e.sleep(ctx, delay) {
			return
		}
	}
	debug.Log("enrich: %s details enriched %d/%d", label, done, len(refs))
}

// applyDetails persists a fetched video's full metadata: the exact upload date
// (replacing the approximate one), the details cache, and processed chapters,
// SponsorBlock segments and description links — mirroring what opening the video
// in the TUI caches.
func (e *Enricher) applyDetails(ctx context.Context, videoID string, d domain.VideoDetails) {
	if len(d.UploadDate) == 8 {
		if err := e.cat.UpdateVideoUploadDate(ctx, videoID, d.UploadDate); err != nil {
			debug.Log("enrich: upload_date %s: %v", videoID, err)
		}
	}
	if err := e.cat.SaveVideoDetailsCache(ctx, videoID, d.Description, d.ThumbnailURL, d.Subscribers); err != nil {
		debug.Log("enrich: details cache %s: %v", videoID, err)
	}
	chapters, sb := media.ProcessChapters(d.Chapters)
	if len(chapters) > 0 {
		if err := e.cat.SaveVideoChapters(ctx, videoID, chapters); err != nil {
			debug.Log("enrich: chapters %s: %v", videoID, err)
		}
	}
	if len(sb) > 0 {
		if err := e.cat.SaveVideoSBSegments(ctx, videoID, sb); err != nil {
			debug.Log("enrich: sb segments %s: %v", videoID, err)
		}
	}
	// Persist the extracted links (possibly empty) so the TUI's link modal never
	// has to re-parse the description for an enriched video.
	if err := e.cat.SaveVideoLinks(ctx, videoID, media.ExtractLinks(d.Description)); err != nil {
		debug.Log("enrich: links %s: %v", videoID, err)
	}
}

// sleepCtx waits d, returning false if ctx is canceled first. A non-positive d
// still checks for cancellation so a zero-delay loop stays interruptible.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return false
		default:
			return true
		}
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
