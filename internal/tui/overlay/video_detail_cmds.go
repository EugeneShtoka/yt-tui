package overlay

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/sys"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// transcriptFetchTimeout bounds how long the loading popup waits for a cold
// transcript fetch before giving up. It can never hang: on timeout we surface a
// retry hint while the daemon's shared build keeps running detached and warms
// the store, so a re-press resolves from cache. Saved transcripts are served
// from cache and never reach this. A var (not const) so tests can shrink it.
var transcriptFetchTimeout = 20 * time.Second

// cacheCmd reads the local details cache for v. A hit renders the panel
// instantly from disk; a miss triggers a network fetch (see the vdCacheMsg
// handler). This is what keeps re-opening a previously-seen video instant
// instead of blocking on yt-dlp every time.
func (vd VideoDetail) cacheCmd(v domain.Video) tea.Cmd {
	token := vd.fetchToken
	target := tuipkg.OverlayTarget{ID: vd.ID()}
	return func() tea.Msg {
		c, ok, err := vd.backend.GetVideoDetailsCache(vd.fetchContext(), v.ID)
		if err != nil {
			ok = false // treat a cache read error as a miss and fetch fresh
		}
		return vdCacheMsg{OverlayTarget: target, video: v, details: c, ok: ok, token: token}
	}
}

// parseWidthSpec resolves a width specification against the terminal width:
// "50%" → half the terminal, "80" → 80 columns. Invalid specs fall back to 50%.
// The result is the outer popup width (border included), clamped so it always
// fits inside the terminal and stays wide enough to read.
func parseWidthSpec(spec string, termW int) int {
	spec = strings.TrimSpace(spec)
	var v int
	switch {
	case spec == "":
		v = termW / 2
	case strings.HasSuffix(spec, "%"):
		pct, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(spec, "%")))
		if err != nil || pct <= 0 || pct > 100 {
			pct = 50
		}
		v = termW * pct / 100
	default:
		n, err := strconv.Atoi(spec)
		if err != nil || n <= 0 {
			v = termW / 2
		} else {
			v = n
		}
	}
	if max := termW - 4; v > max {
		v = max
	}
	if v < 24 {
		v = 24
	}
	return v
}

// transcriptBoxWidth is the lipgloss Width passed to placeOverlayBox; the
// rounded border adds two columns, so it is two less than the visible popup.
func transcriptBoxWidth(spec string, termW int) int { return parseWidthSpec(spec, termW) - 2 }

// transcriptTextWidth is the column width transcript text is wrapped/clamped to:
// the box content area (box width minus padding) less a small right margin.
func transcriptTextWidth(spec string, termW int) int { return transcriptBoxWidth(spec, termW) - 6 }

// transcriptLoadCmd fetches the video's transcript through the media seam (which
// always routes to the daemon — the client can't run yt-dlp — serving from its
// store and fetching on a miss) off the event loop.
func (vd VideoDetail) transcriptLoadCmd(v domain.Video) tea.Cmd {
	token := vd.fetchToken
	target := tuipkg.OverlayTarget{ID: vd.ID()}
	media := vd.media
	base := vd.fetchContext()
	retryKey := vd.keys.OpenTranscript.Help().Key
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(base, transcriptFetchTimeout)
		defer cancel()
		text, ok, err := media.GetTranscript(ctx, v.ID, v.URL)
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			// The fetch is still running on the daemon (shared build, detached);
			// invite a re-press, which will land on the warmed cache.
			err = fmt.Errorf("still fetching — press %s to retry", retryKey)
		case err == nil && !ok:
			err = fmt.Errorf("no transcript available")
		}
		return vdTranscriptMsg{OverlayTarget: target, text: text, err: err, token: token}
	}
}

// handleTranscriptMsg applies a completed transcript fetch: it wraps the text to
// the modal width and opens the transcript modal, or pops the overlay with a
// status error when none is available.
func (vd VideoDetail) handleTranscriptMsg(m vdTranscriptMsg) (tea.Model, tea.Cmd) {
	if m.token != vd.fetchToken {
		return vd, nil // stale — video changed while fetching
	}
	vd.loading = false
	vd.transcriptLoading = false
	if m.err != nil {
		status := func() tea.Msg { return tuipkg.StatusMsg{Text: "transcript: " + m.err.Error(), IsErr: true} }
		// A standalone transcript modal has nothing to fall back to, so close it;
		// when opened over the info panel, stay on the panel.
		if vd.initialView == InitialViewTranscript {
			return vd, tea.Batch(func() tea.Msg { return PopOverlayMsg{} }, status)
		}
		return vd, status
	}
	vd.transcriptText = m.text
	vd.transcriptVS = 0
	vd.transcriptLoaded = true
	vd.subState = vdTranscript
	return vd, nil
}

// transcriptTermWidth reconstructs the full terminal width from the stored
// content width plus whatever the panel reserves, so scroll bookkeeping wraps
// to the same width the centered popup is drawn at (which uses the full width).
func (vd VideoDetail) transcriptTermWidth() int { return vd.contentW + vd.WidthReduction() }

func (vd VideoDetail) fetchCmd(v domain.Video) tea.Cmd {
	token := vd.fetchToken
	target := tuipkg.OverlayTarget{ID: vd.ID()}
	return func() tea.Msg {
		details, err := vd.backend.VideoDetails(vd.fetchContext(), v.URL)
		return vdDetailsMsg{OverlayTarget: target, details: details, err: err, token: token}
	}
}

func (vd VideoDetail) refreshCmd() tea.Cmd {
	v := vd.fetchVideo
	token := vd.fetchToken
	target := tuipkg.OverlayTarget{ID: vd.ID()}
	return func() tea.Msg {
		details, err := vd.backend.VideoDetails(vd.fetchContext(), v.URL)
		return vdDetailsMsg{OverlayTarget: target, details: details, err: err, token: token}
	}
}

// openURLCmd opens a URL in the user's browser off the event loop, surfacing a
// status error on failure. sys.OpenURL execs xdg-open, which can hang, so it
// must never run inline in Update.
func openURLCmd(u string) tea.Cmd {
	return func() tea.Msg {
		if err := sys.OpenURL(u); err != nil {
			return tuipkg.StatusMsg{Text: "open: " + err.Error(), IsErr: true}
		}
		return nil
	}
}

// saveDetailsCacheCmd / saveChaptersCmd / saveLinksCmd persist non-authoritative
// caches off the event loop. Errors are swallowed (the caches are best-effort),
// but the writes must not block Update.
func saveDetailsCacheCmd(ctx context.Context, b api.VideoBackend, d domain.VideoDetails) tea.Cmd {
	return func() tea.Msg {
		_ = b.SaveVideoDetailsCache(ctx, d.ID, d.Description, d.ThumbnailURL, d.Subscribers)
		return nil
	}
}

func saveChaptersCmd(ctx context.Context, b api.VideoBackend, id string, chapters []domain.Chapter) tea.Cmd {
	return func() tea.Msg {
		_ = b.SaveVideoChapters(ctx, id, chapters)
		return nil
	}
}

func saveLinksCmd(ctx context.Context, b api.VideoBackend, id string, links []domain.Link) tea.Cmd {
	return func() tea.Msg {
		_ = b.SaveVideoLinks(ctx, id, links)
		return nil
	}
}

func fmtChapterTime(secs float64) string {
	s := int(secs)
	h, min, sec := s/3600, (s%3600)/60, s%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, min, sec)
	}
	return fmt.Sprintf("%d:%02d", min, sec)
}
