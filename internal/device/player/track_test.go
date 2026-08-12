package player

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestTrackLoopSavesUntilProcessExits verifies the loop polls+saves while the
// process is alive and stops once it isn't.
func TestTrackLoopSavesUntilProcessExits(t *testing.T) {
	aliveCalls := 0
	alive := func() bool { aliveCalls++; return aliveCalls <= 3 } // alive for 3 iterations

	pollN := 0
	poll := func() (int64, bool) { pollN++; return int64(pollN * 100), true }

	var saved []int64
	trackLoop(context.Background(), time.Millisecond, alive, poll, func(ms int64) { saved = append(saved, ms) })

	want := []int64{100, 200, 300}
	if len(saved) != len(want) {
		t.Fatalf("saved %v, want %v", saved, want)
	}
	for i := range want {
		if saved[i] != want[i] {
			t.Errorf("saved[%d] = %d, want %d", i, saved[i], want[i])
		}
	}
}

// TestTrackLoopStopsOnContextCancel verifies a canceled context ends the loop
// after the current iteration.
func TestTrackLoopStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled

	saves := 0
	trackLoop(ctx, time.Hour, func() bool { return true }, func() (int64, bool) { return 1, true }, func(int64) { saves++ })

	if saves != 1 {
		t.Errorf("with a pre-canceled ctx the loop should save once then stop, got %d saves", saves)
	}
}

// TestTrackLoopSkipsSaveOnPollMiss verifies a failed poll saves nothing but
// keeps looping.
func TestTrackLoopSkipsSaveOnPollMiss(t *testing.T) {
	n := 0
	alive := func() bool { n++; return n <= 2 }
	saved := 0
	trackLoop(context.Background(), time.Millisecond, alive, func() (int64, bool) { return 0, false }, func(int64) { saved++ })
	if saved != 0 {
		t.Errorf("poll misses should never save, got %d", saved)
	}
}

func TestPidAlive(t *testing.T) {
	if !pidAlive(os.Getpid()) {
		t.Error("pidAlive(self) = false, want true")
	}
	if pidAlive(0) {
		t.Error("pidAlive(0) = true, want false")
	}
	if pidAlive(-1) {
		t.Error("pidAlive(-1) = true, want false")
	}
}

func TestSimpleBackendActiveReportsNothing(t *testing.T) {
	b := newSimpleBackend(fakeDriver{path: "/bin/true"})
	if _, ok := b.Active(); ok {
		t.Error("simpleBackend.Active() ok = true; it has no position tracking")
	}
}

func TestMPRISBackendActiveReporting(t *testing.T) {
	// Active() is pure field logic (no D-Bus), so build the backend struct
	// directly and drive its state.
	b := &mprisBackend{}
	if _, ok := b.Active(); ok {
		t.Error("Active() ok = true with nothing playing, want false")
	}

	// A running process + a video id + an active session → reported.
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("no 'true' binary available")
	}
	cmd := exec.CommandContext(context.Background(), truePath)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start true: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Wait() })

	b.curVideoID = "vid1"
	b.curCmd = cmd
	b.curSess = newSession(0) // active by construction

	info, ok := b.Active()
	if !ok {
		t.Fatal("Active() ok = false with a live playback, want true")
	}
	if info.VideoID != "vid1" || info.PID != cmd.Process.Pid {
		t.Errorf("Active() = %+v, want VideoID=vid1 PID=%d", info, cmd.Process.Pid)
	}

	// Once the session ends, Active() stops reporting it.
	b.curSess.Finish()
	if _, ok := b.Active(); ok {
		t.Error("Active() ok = true after the session finished, want false")
	}
}
