package player

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

type simpleBackend struct {
	driver Driver
}

func newSimpleBackend(driver Driver) *simpleBackend {
	return &simpleBackend{driver: driver}
}

func (s *simpleBackend) exec(args []string) (*Session, error) {
	cmd := exec.CommandContext(context.Background(), s.driver.Path(), args...) //nolint:gosec // G204: the configured player binary; args go without a shell
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// Capture the player's console output when we can, so a launch that never
	// plays can be explained (see consoleLog). mpv reports yt-dlp's errors on
	// stdout and other players on stderr, so both go to the same log; /dev/null
	// stays the fallback.
	console := newConsoleLog()
	if f := console.file(); f != nil {
		cmd.Stdout = f
		cmd.Stderr = f
	}
	if err := cmd.Start(); err != nil {
		console.close()
		return nil, fmt.Errorf("exec: %w", err)
	}
	sess := newSession(0)
	sess.setPosition(0, false) // simple backend has no position tracking
	go func() {
		waitErr := cmd.Wait()
		sess.finishWith(waitErr, console.tail())
		console.close()
	}()
	return sess, nil
}

func (s *simpleBackend) Launch(_, source, title string, startAt time.Duration) (*Session, error) {
	return s.exec(s.driver.Args(source, title, startAt))
}

func (s *simpleBackend) LaunchAudio(_, source, title string, startAt time.Duration) (*Session, error) {
	return s.exec(s.driver.AudioArgs(source, title, startAt))
}

// Active always reports no trackable playback: the simple backend has no
// position tracking (no MPRIS), so there is nothing for a background tracker
// to hand off to.
func (s *simpleBackend) Active() (ActivePlayback, bool) { return ActivePlayback{}, false }

// Close is a no-op: the simple backend intentionally lets the player process
// survive the TUI quitting so the user can keep watching. The per-launch
// cmd.Wait() reaper goroutine in exec dies with the process in single-binary
// mode. The surviving player is by design, not a leak.
func (s *simpleBackend) Close() {}
