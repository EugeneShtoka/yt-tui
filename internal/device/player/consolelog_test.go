package player

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// TestConsoleLogCapturesTail: the point of the capture is the end of the output, so a
// log longer than consoleTailBytes keeps its last bytes, not its first.
func TestConsoleLogCapturesTail(t *testing.T) {
	log := newConsoleLog()
	if log == nil {
		t.Skip("no temp dir available for console capture")
	}
	defer log.close()

	if _, err := log.file().WriteString(strings.Repeat("a", consoleTailBytes) + "the real error"); err != nil {
		t.Fatalf("write: %v", err)
	}
	tail := log.tail()
	if !strings.HasSuffix(tail, "the real error") {
		t.Errorf("tail lost the end of the log: %q", tail[max(0, len(tail)-40):])
	}
	if len(tail) > consoleTailBytes {
		t.Errorf("tail is %d bytes, want at most %d", len(tail), consoleTailBytes)
	}
}

// TestConsoleLogIsUnlinked: the capture file must never be left on disk, since a
// player can outlive the TUI and nobody would be around to clean it up.
func TestConsoleLogIsUnlinked(t *testing.T) {
	log := newConsoleLog()
	if log == nil {
		t.Skip("no temp dir available for console capture")
	}
	defer log.close()
	if _, err := log.file().WriteString("still writable\n"); err != nil {
		t.Errorf("unlinked log is not writable: %v", err)
	}
	if got := log.tail(); got != "still writable" {
		t.Errorf("tail = %q after unlink", got)
	}
}

// TestConsoleLogNilSafe: capture is best-effort, so every method has to tolerate the
// nil consoleLog that a failed CreateTemp yields.
func TestConsoleLogNilSafe(t *testing.T) {
	var log *consoleLog
	if log.file() != nil {
		t.Error("nil consoleLog must offer no writer")
	}
	if got := log.tail(); got != "" {
		t.Errorf("nil consoleLog tail = %q", got)
	}
	log.close()
}

// TestConsoleLogEmpty: a player that said nothing yields no tail rather than junk.
func TestConsoleLogEmpty(t *testing.T) {
	log := newConsoleLog()
	if log == nil {
		t.Skip("no temp dir available for console capture")
	}
	defer log.close()
	if got := log.tail(); got != "" {
		t.Errorf("empty log tail = %q", got)
	}
}

func TestExitStatus(t *testing.T) {
	if got := exitStatus(nil); got != 0 {
		t.Errorf("clean exit = %d, want 0", got)
	}
	// A real non-zero exit, produced by running a command that fails.
	err := exec.CommandContext(context.Background(), "sh", "-c", "exit 3").Run()
	if got := exitStatus(err); got != 3 {
		t.Errorf("exit 3 = %d, want 3", got)
	}
	if got := exitStatus(errors.New("wait failed")); got != -1 {
		t.Errorf("non-exit error = %d, want -1", got)
	}
}
