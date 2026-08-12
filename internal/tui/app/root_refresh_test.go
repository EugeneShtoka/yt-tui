package app

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
)

// Regression test for M-10: RefreshPositionsMsg must trigger exactly one
// LoadAuxDataCmd at the root level, not one per tab. Every tab should then
// receive the single resulting AuxDataMsg via the default broadcast, and none
// should see the raw RefreshPositionsMsg directly.
func TestHandleRefreshPositions_IssuesSingleAuxLoad(t *testing.T) {
	r := Root{backend: apitest.NopBackend{}, router: tabRouter{tabs: []tuipkg.Tab{fakeTab{}, fakeTab{}}}}

	updated, cmd := r.Update(tuipkg.RefreshPositionsMsg{})
	r = updated.(Root)
	if cmd == nil {
		t.Fatal("expected a non-nil command")
	}
	msg := cmd()
	if _, ok := msg.(videotable.AuxDataMsg); !ok {
		t.Fatalf("expected AuxDataMsg, got %T", msg)
	}

	for i, tb := range r.router.tabs {
		ft := tb.(fakeTab)
		if len(ft.received) != 0 {
			t.Errorf("tab %d: expected no direct RefreshPositionsMsg, got %v", i, ft.received)
		}
	}

	updated, _ = r.Update(msg)
	r = updated.(Root)
	for i, tb := range r.router.tabs {
		ft := tb.(fakeTab)
		if len(ft.received) != 1 {
			t.Errorf("tab %d: expected exactly 1 received AuxDataMsg, got %d", i, len(ft.received))
		}
		if _, ok := ft.received[0].(videotable.AuxDataMsg); !ok {
			t.Errorf("tab %d: expected AuxDataMsg, got %T", i, ft.received[0])
		}
	}
}
