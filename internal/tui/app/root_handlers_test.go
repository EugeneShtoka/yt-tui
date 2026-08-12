package app

import (
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	ovpkg "github.com/EugeneShtoka/yt-tui/internal/tui/overlay"
)

func receivedContains[T tea.Msg](msgs []tea.Msg) bool {
	for _, m := range msgs {
		if _, ok := m.(T); ok {
			return true
		}
	}
	return false
}

// A successful enqueue emits EnqueueSucceededMsg carrying the video title.
func TestHandleEnqueue_Success(t *testing.T) {
	r := Root{backend: apitest.NopBackend{}}
	_, cmd := r.handleEnqueue(tuipkg.EnqueueMsg{Video: domain.Video{Title: "Vid"}, AudioOnly: true})

	got, ok := runCmd(cmd).(tuipkg.EnqueueSucceededMsg)
	if !ok {
		t.Fatalf("want EnqueueSucceededMsg, got %#v", runCmd(cmd))
	}
	if got.Title != "Vid" || !got.AudioOnly {
		t.Errorf("result = %#v, want {Title:Vid AudioOnly:true}", got)
	}
}

// A completed enqueue reports a status line and asks the downloads view to refresh.
func TestHandleEnqueueSucceeded_StatusAndDownloadsChanged(t *testing.T) {
	r := Root{}
	_, cmd := r.handleEnqueueSucceeded(tuipkg.EnqueueSucceededMsg{Title: "Vid"})

	msgs := flatten(runCmd(cmd))
	if !receivedContains[tuipkg.StatusMsg](msgs) {
		t.Error("expected a StatusMsg")
	}
	if !receivedContains[tuipkg.DownloadItemsChangedMsg](msgs) {
		t.Error("expected a DownloadItemsChangedMsg")
	}
}

// Copying a URL returns a (clipboard) command rather than a no-op.
func TestHandleCopyURL_ReturnsCommand(t *testing.T) {
	r := Root{}
	if _, cmd := r.handleCopyURL(tuipkg.CopyURLMsg{URL: "https://x"}); cmd == nil {
		t.Error("handleCopyURL should return a copy command")
	}
}

// A debounce whose token is stale (a newer selection superseded it) is dropped.
func TestHandleSelectionDebounce_StaleTokenDropped(t *testing.T) {
	r := Root{selectionDebounceToken: 2, overlays: []ovpkg.Overlay{newPanelOverlay()}}
	r2, cmd := r.handleSelectionDebounce(selectionDebounceMsg{token: 1, video: domain.Video{ID: "v"}})
	if cmd != nil {
		t.Errorf("stale-token debounce should be a no-op, got cmd %#v", cmd)
	}
	if _, ok := r2.overlays[0].(ovpkg.VideoDetail); !ok {
		t.Error("overlay stack should be untouched")
	}
}

// A current-token debounce delivers the selection to the info panel (which then
// kicks off its fetch), exercising updatePanelOverlay.
func TestHandleSelectionDebounce_DeliversToPanel(t *testing.T) {
	r := Root{selectionDebounceToken: 3, overlays: []ovpkg.Overlay{newPanelOverlay()}}
	_, cmd := r.handleSelectionDebounce(selectionDebounceMsg{token: 3, video: domain.Video{ID: "v", URL: "u"}})
	if cmd == nil {
		t.Error("delivering a new selection to the panel should trigger its fetch")
	}
}

// The shared spinner frame is fanned to the active tab AND every overlay, so a
// panel beneath a stacked modal keeps animating (regression guard).
func TestHandleSpinnerTick_FansFrameToTabAndOverlays(t *testing.T) {
	r := Root{
		spinner:  spinner.New(),
		router:   tabRouter{tabs: []tuipkg.Tab{fakeTab{}}},
		overlays: []ovpkg.Overlay{fakeOverlay{id: 1}},
	}
	r2, _ := r.handleSpinnerTick(spinner.TickMsg{})

	if !receivedContains[tuipkg.SpinnerFrameMsg](r2.router.tabs[0].(fakeTab).received) {
		t.Error("active tab did not receive the spinner frame")
	}
	if !receivedContains[tuipkg.SpinnerFrameMsg](r2.overlays[0].(fakeOverlay).received) {
		t.Error("overlay did not receive the spinner frame")
	}
}

// An unsubscribe result is broadcast to every tab (so optimistic removers can
// revert on failure) and surfaces a status line.
func TestHandleUnsubscribeResult_BroadcastsAndStatus(t *testing.T) {
	r := Root{router: tabRouter{tabs: []tuipkg.Tab{fakeTab{}}}}
	r2, cmd := r.handleUnsubscribeResult(tuipkg.UnsubscribeResultMsg{Channel: domain.Channel{Name: "C"}})

	if !receivedContains[tuipkg.UnsubscribeResultMsg](r2.router.tabs[0].(fakeTab).received) {
		t.Error("unsubscribe result should be broadcast to tabs")
	}
	if sm := findStatus(t, cmd); sm.IsErr {
		t.Errorf("want success status, got error %q", sm.Text)
	}
}
