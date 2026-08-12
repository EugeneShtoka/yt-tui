package app

import (
	"context"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	ovpkg "github.com/EugeneShtoka/yt-tui/internal/tui/overlay"
)

// openInfoPanel builds a Root whose only overlay is a real info side panel.
func openInfoPanel(t *testing.T) (Root, domain.Video) {
	t.Helper()
	v := domain.Video{ID: "abc", URL: "https://youtu.be/abc", Title: "T"}
	be := apitest.NopBackend{}
	panel, _ := ovpkg.NewVideoDetail(context.Background(), be, be, testKeyMap(), v, ovpkg.VideoDetailOpts{
		InitialView: ovpkg.InitialViewPanel,
	})
	r := Root{
		keys:     testKeyMap(),
		cfg:      &config.Config{},
		backend:  be,
		media:    be,
		router:   tabRouter{tabs: []tuipkg.Tab{fakeTab{}}},
		overlays: []ovpkg.Overlay{panel},
	}
	return r, v
}

// Opening a sub-view (transcript/links/chapters) while the info panel is already
// open must route into that panel, not stack a second VideoDetail overlay — two
// stacked panels crossed their untagged async fetch messages (the reported
// "transcript flickers shut / info never loads" bug).
func TestHandleOpenOverlay_SubViewRoutesIntoOpenPanel(t *testing.T) {
	kinds := []tuipkg.OverlayKind{
		tuipkg.OverlayVideoDetailTranscript,
		tuipkg.OverlayVideoDetailLinks,
		tuipkg.OverlayVideoDetailChapters,
	}
	for _, kind := range kinds {
		r, v := openInfoPanel(t)
		r2, _ := r.handleOpenOverlay(tuipkg.OpenOverlayMsg{Kind: kind, Video: v})
		if len(r2.overlays) != 1 {
			t.Fatalf("kind %v: overlays = %d, want 1 (routed into panel, not stacked)", kind, len(r2.overlays))
		}
		if vd, ok := r2.overlays[0].(ovpkg.VideoDetail); !ok || !vd.IsPanel() {
			t.Fatalf("kind %v: top overlay is %T, want the info panel", kind, r2.overlays[0])
		}
	}
}

// With no panel open, the same action opens a standalone modal overlay (the
// else-branch must keep working).
func TestHandleOpenOverlay_TranscriptStandaloneWhenNoPanel(t *testing.T) {
	be := apitest.NopBackend{}
	r := Root{
		keys: testKeyMap(), cfg: &config.Config{}, backend: be, media: be,
		router: tabRouter{tabs: []tuipkg.Tab{fakeTab{}}},
	}
	v := domain.Video{ID: "abc", URL: "https://youtu.be/abc"}
	r2, _ := r.handleOpenOverlay(tuipkg.OpenOverlayMsg{Kind: tuipkg.OverlayVideoDetailTranscript, Video: v})
	if len(r2.overlays) != 1 {
		t.Fatalf("overlays = %d, want 1 (standalone modal opened)", len(r2.overlays))
	}
	if vd, ok := r2.overlays[0].(ovpkg.VideoDetail); !ok || vd.IsPanel() {
		t.Fatalf("top overlay is %T / IsPanel=%v, want a standalone (non-panel) modal", r2.overlays[0], ok && vd.IsPanel())
	}
}
