package tab

import (
	"context"
	"testing"

	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// TestSpinnerFrameMsgSetsFrame verifies a tab stores the frame forwarded by the
// shared Root spinner (tabs no longer drive their own spinner tick loop).
func TestSpinnerFrameMsgSetsFrame(t *testing.T) {
	dl := NewDownloading(context.Background(), &fakeDlBackend{}, testKeys(), false)
	updated, _ := dl.Update(tuipkg.SpinnerFrameMsg{Frame: "SPIN"})
	if got := updated.(Downloading).spinnerFrame; got != "SPIN" {
		t.Errorf("spinnerFrame = %q, want %q", got, "SPIN")
	}
}

// TestLoadCmdAddressedToOwnerTab verifies a background-load command produces a
// message addressed to the owning tab, so Root routes it there instead of
// broadcasting to every tab.
func TestLoadCmdAddressedToOwnerTab(t *testing.T) {
	dl := NewDownloading(context.Background(), &fakeDlBackend{}, testKeys(), false)
	msg := dl.fetchItemsCmd()()

	am, ok := msg.(tuipkg.TabAddressedMsg)
	if !ok {
		t.Fatalf("%T does not implement tuipkg.TabAddressedMsg", msg)
	}
	if am.TargetTab() != tuipkg.TabDownloading {
		t.Errorf("TargetTab() = %v, want TabDownloading", am.TargetTab())
	}
}
