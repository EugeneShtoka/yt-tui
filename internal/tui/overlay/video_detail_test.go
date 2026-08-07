package overlay

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
)

// fakeDetailBackend records whether the network VideoDetails path was hit and
// serves a canned cache result, so the cache-first flow can be asserted.
type fakeDetailBackend struct {
	apitest.NopBackend
	cache         domain.CachedDetails
	cacheOK       bool
	detailsCalled bool
}

func (b *fakeDetailBackend) GetVideoDetailsCache(context.Context, string) (domain.CachedDetails, bool, error) {
	return b.cache, b.cacheOK, nil
}

func (b *fakeDetailBackend) VideoDetails(context.Context, string) (domain.VideoDetails, error) {
	b.detailsCalled = true
	return domain.VideoDetails{}, nil
}

// TestVideoDetailCacheHitSkipsNetwork verifies the panel renders from the local
// cache without ever invoking the (slow) yt-dlp VideoDetails fetch — the fix for
// video info taking seconds to appear for already-seen videos.
func TestVideoDetailCacheHitSkipsNetwork(t *testing.T) {
	b := &fakeDetailBackend{cacheOK: true, cache: domain.CachedDetails{Description: "cached description"}}
	keys := keymap.Build(config.KeyBindings{Close: "esc"})
	v := domain.Video{ID: "abc", URL: "https://youtu.be/abc", Title: "T"}

	vd, cmd := NewVideoDetail(context.Background(), b, b, keys, v, VideoDetailOpts{InitialView: InitialViewPanel, TranscriptWidth: "50%"})
	if cmd == nil {
		t.Fatal("NewVideoDetail returned no command")
	}
	msg, ok := cmd().(vdCacheMsg)
	if !ok {
		t.Fatalf("first command must emit vdCacheMsg (cache-first), got %T", cmd())
	}

	model, _ := vd.Update(msg)
	got := model.(VideoDetail)
	if got.loading {
		t.Error("panel still loading after a cache hit")
	}
	if got.video == nil || got.video.Description != "cached description" {
		t.Errorf("panel not populated from cache: %+v", got.video)
	}
	if b.detailsCalled {
		t.Error("network VideoDetails fetched despite a cache hit")
	}
}

// TestVideoDetailCacheMissFallsBackToNetwork verifies a cache miss keeps the
// spinner up and issues the yt-dlp fetch (which also repopulates the cache).
func TestVideoDetailCacheMissFallsBackToNetwork(t *testing.T) {
	b := &fakeDetailBackend{cacheOK: false}
	keys := keymap.Build(config.KeyBindings{Close: "esc"})
	v := domain.Video{ID: "xyz", URL: "https://youtu.be/xyz", Title: "T"}

	vd, cmd := NewVideoDetail(context.Background(), b, b, keys, v, VideoDetailOpts{InitialView: InitialViewPanel, TranscriptWidth: "50%"})
	msg := cmd().(vdCacheMsg)

	model, fetch := vd.Update(msg)
	got := model.(VideoDetail)
	if !got.loading {
		t.Error("panel should stay loading on a cache miss")
	}
	if fetch == nil {
		t.Fatal("cache miss did not issue a network fetch command")
	}
	fetch() // drive the fallback fetch
	if !b.detailsCalled {
		t.Error("cache miss did not fall back to the network VideoDetails fetch")
	}
}

// TestVideoDetailRenderRectangular guards against the panel layout corruption
// where a rendered frame line ends up wider or narrower than the terminal (or
// gains an embedded newline), pushing the right border off its column and
// shifting subsequent rows. The composed frame must be a perfect rectangle:
// every line exactly `width` display columns, and exactly `height` lines.
func TestVideoDetailRenderRectangular(t *testing.T) {
	const width, height = 120, 40

	vd := VideoDetail{
		keys:     keymap.Build(config.KeyBindings{Close: "esc", Up: "k", Down: "j"}),
		subState: vdPanel,
	}
	details := domain.VideoDetails{
		Video: domain.Video{
			Title:      "Tan France Builds His Dream Home From Start to Finish | Architectural Digest → 🔥",
			Channel:    "Architectural Digest",
			ViewCount:  48800,
			Duration:   8262,
			UploadDate: "20260715",
			URL:        "https://www.youtube.com/watch?v=1CvWdg67luc",
		},
		Description: "Sit down and relax → ➤➤ 🔥🔥 then visit " +
			"https://www.example.com/some/really/long/path/that/overflows?q=1&x=2 for more, " +
			"and subscribe → http://bit.ly/abcdefghijklmnop 🔥 thanks for watching!",
		Subscribers: 1_000_000,
	}
	vd.video = &details
	vd.descLines = render.WordWrap(render.ShortenURLs(details.Description, panelW-2), panelW-2)

	// "behind" simulates the tab table. Deliberately include lines WIDER than
	// the remaining width (width-panelW) and containing wide runes — the case
	// that made lipgloss .Width() word-wrap a single row into two lines and
	// desync the panel border.
	behindLines := make([]string, height)
	for i := range behindLines {
		behindLines[i] = strings.Repeat("Инфо ", 25) + "→🔥 overflow tail"
	}
	behind := strings.Join(behindLines, "\n")

	out := vd.Render(behind, width, height)
	lines := strings.Split(out, "\n")

	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			t.Errorf("line %d width = %d, want %d: %q", i, w, width, l)
		}
	}
	if len(lines) != height {
		t.Errorf("composed line count = %d, want %d (embedded newline / wrap corruption)", len(lines), height)
	}

	// The info panel reserves panelW plus a blank gap so it doesn't touch the
	// tab content. (#5)
	if got := vd.WidthReduction(); got != panelW+panelGap {
		t.Errorf("panel WidthReduction = %d, want %d (panelW+panelGap)", got, panelW+panelGap)
	}
}

// TestStandaloneLinksModalIndependent verifies that a links modal opened
// directly from the table (InitialViewLinks) is fully independent of the info
// side panel: it reserves no width, always captures input, renders no side
// panel, and closing it pops the whole overlay.
func TestStandaloneLinksModalIndependent(t *testing.T) {
	const width, height = 120, 40
	keys := keymap.Build(config.KeyBindings{Close: "esc", Up: "k", Down: "j", OpenLinks: "L"})

	vd := VideoDetail{
		keys:        keys,
		initialView: InitialViewLinks,
		subState:    vdLinks,
		linksLoaded: true,
		links:       []domain.Link{{Label: "Example", URL: "https://example.com"}},
		video:       &domain.VideoDetails{Video: domain.Video{Title: "T", URL: "u"}},
	}

	if vd.IsPanel() {
		t.Error("standalone modal must not report IsPanel()")
	}
	if vd.WidthReduction() != 0 {
		t.Errorf("standalone modal WidthReduction = %d, want 0", vd.WidthReduction())
	}
	if !vd.HasFocus() {
		t.Error("standalone modal must always capture input")
	}

	behind := strings.Join(make([]string, height), "\n")
	out := vd.Render(behind, width, height)
	if strings.Contains(out, "Video Details") {
		t.Error("standalone modal must not render the info side panel")
	}
	for i, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w > width {
			t.Errorf("line %d width = %d, want <= %d: %q", i, w, width, l)
		}
	}

	// Escape must pop the whole overlay (no side panel to fall back to).
	model, cmd := vd.dismissModal()
	if got := model.subState; got != vdLinks {
		t.Errorf("subState mutated on dismiss = %d; standalone modal should pop, not switch panes", got)
	}
	if cmd == nil {
		t.Fatal("dismissModal returned no command for standalone modal")
	}
	if _, ok := cmd().(PopOverlayMsg); !ok {
		t.Error("dismissModal must emit PopOverlayMsg for standalone modal")
	}
}
