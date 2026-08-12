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

func newPanelOverlay() ovpkg.Overlay {
	vd, _ := ovpkg.NewVideoDetail(
		context.Background(), apitest.NopBackend{}, apitest.NopBackend{}, testKeyMap(),
		domain.Video{ID: "x"}, ovpkg.VideoDetailOpts{InitialView: ovpkg.InitialViewPanel},
	)
	return vd
}

// Switching tabs while a modal (e.g. Add-to-Playlist) is stacked over the info
// panel must close the modal but keep the panel, so the panel re-tracks the new
// tab instead of leaving a stale modal floating over an unrelated tab.
func TestReconcileAfterTabSwitch_ClosesStackedModalKeepsPanel(t *testing.T) {
	r := Root{
		keys:     testKeyMap(),
		cfg:      &config.Config{},
		router:   tabRouter{tabs: []tuipkg.Tab{fakeTab{}}},
		overlays: []ovpkg.Overlay{newPanelOverlay(), fakeOverlay{id: 7}},
	}

	r2, _ := r.reconcileOverlaysAfterTabSwitch()

	if len(r2.overlays) != 1 {
		t.Fatalf("expected only the info panel to remain, got %d overlays", len(r2.overlays))
	}
	if vd, ok := r2.overlays[0].(ovpkg.VideoDetail); !ok || !vd.IsPanel() {
		t.Fatalf("the surviving overlay should be the info panel, got %T", r2.overlays[0])
	}
}

// With no info panel open, a tab switch closes every stacked modal.
func TestReconcileAfterTabSwitch_ClosesAllModalsWhenNoPanel(t *testing.T) {
	r := Root{
		keys:     testKeyMap(),
		cfg:      &config.Config{},
		router:   tabRouter{tabs: []tuipkg.Tab{fakeTab{}}},
		overlays: []ovpkg.Overlay{fakeOverlay{id: 1}, fakeOverlay{id: 2}},
	}

	r2, _ := r.reconcileOverlaysAfterTabSwitch()

	if len(r2.overlays) != 0 {
		t.Fatalf("expected all modals closed with no panel open, got %d overlays", len(r2.overlays))
	}
}
