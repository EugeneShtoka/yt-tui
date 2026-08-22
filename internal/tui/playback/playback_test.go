package playback

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/device/player"
)

// fakeBackend records SaveVideoPosition calls; the other Backend methods are
// inert stubs (the position-loop tests don't exercise them).
type fakeBackend struct {
	calls   int
	savedID string
	savedMs int64
}

func (f *fakeBackend) ResolveSource(context.Context, string, string) (api.PlayableSource, error) {
	return api.PlayableSource{}, nil
}
func (f *fakeBackend) VideoPosition(context.Context, string) (int64, bool, error) {
	return 0, false, nil
}
func (f *fakeBackend) AddHistory(context.Context, string, string, string) error { return nil }
func (f *fakeBackend) SaveVideoPosition(_ context.Context, id string, ms int64) error {
	f.calls++
	f.savedID, f.savedMs = id, ms
	return nil
}

func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

// TestHandleStarted_SchedulesWaitAndTick verifies that a StartedMsg fans out to
// exactly the four expected commands (status, history-changed, player-wait, and
// the first position-save tick) without executing the blocking wait or the timer.
func TestHandleStarted_SchedulesWaitAndTick(t *testing.T) {
	c := New(context.Background(), &fakeBackend{}, nil, YtdlpInfo{})
	sess := player.NewSession(0)

	cmd, ok := c.Update(StartedMsg{VideoID: "v1", Sess: sess, Text: "Playing: X"})
	if !ok {
		t.Fatal("Update did not handle StartedMsg")
	}
	// Running the batch cmd yields the child cmd list without executing them, so
	// the 5s tick and the blocking wait never run here.
	batch, isBatch := runCmd(cmd).(tea.BatchMsg)
	if !isBatch {
		t.Fatalf("want tea.BatchMsg, got %#v", runCmd(cmd))
	}
	if len(batch) != 4 {
		t.Errorf("scheduled %d cmds, want 4 (status, history, wait, tick)", len(batch))
	}
}

// TestSavePositionTick_SavesAndRearms verifies that a tick for a live session at
// a positive position saves that position and re-arms the next tick.
func TestSavePositionTick_SavesAndRearms(t *testing.T) {
	be := &fakeBackend{}
	c := New(context.Background(), be, nil, YtdlpInfo{})
	sess := player.NewSession(30 * time.Second) // active, position 30s, not done

	cmd := c.handleSavePositionTick(savePositionTickMsg{id: "v1", sess: sess})
	if cmd == nil {
		t.Fatal("handleSavePositionTick returned nil cmd for a live session")
	}
	batch, ok := runCmd(cmd).(tea.BatchMsg)
	if !ok {
		t.Fatalf("want tea.BatchMsg (save + re-armed tick), got %#v", runCmd(cmd))
	}
	if len(batch) != 2 {
		t.Fatalf("batch has %d cmds, want 2 (save + re-armed tick)", len(batch))
	}
	// Run only the save child (batch[0]); batch[1] is the 5s tick, left unrun.
	runCmd(batch[0])
	if be.calls != 1 {
		t.Errorf("SaveVideoPosition called %d times, want 1", be.calls)
	}
	if be.savedID != "v1" || be.savedMs != 30_000 {
		t.Errorf("saved (%q, %dms), want (\"v1\", 30000ms)", be.savedID, be.savedMs)
	}
}

// TestSavePositionTick_StopsWhenSessionDone verifies that once the session has
// ended, the tick loop stops (nil cmd) and does not save — the final save is
// waitCmd's job, so a dead session must not busy-loop re-arming.
func TestSavePositionTick_StopsWhenSessionDone(t *testing.T) {
	be := &fakeBackend{}
	c := New(context.Background(), be, nil, YtdlpInfo{})
	sess := player.NewSession(30 * time.Second)
	sess.Finish() // player process exited

	cmd := c.handleSavePositionTick(savePositionTickMsg{id: "v1", sess: sess})
	if cmd != nil {
		t.Errorf("want nil cmd for a finished session, got %#v", runCmd(cmd))
	}
	if be.calls != 0 {
		t.Errorf("SaveVideoPosition called %d times for a finished session, want 0", be.calls)
	}
}
