package player

import (
	"context"
	"fmt"
	"os"
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
	null, err := os.Open(os.DevNull)
	if err != nil {
		return nil, fmt.Errorf("exec: open devnull: %w", err)
	}
	defer null.Close()
	cmd := exec.CommandContext(context.Background(), s.driver.Path(), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = null
	cmd.Stderr = null
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("exec: %w", err)
	}
	sess := newSession(0)
	sess.setPosition(0, false) // simple backend has no position tracking
	go func() {
		_ = cmd.Wait()
		sess.stop()
		close(sess.doneCh)
	}()
	return sess, nil
}

func (s *simpleBackend) Launch(source, title string, startAt time.Duration) (*Session, error) {
	return s.exec(s.driver.Args(source, title, startAt))
}

func (s *simpleBackend) LaunchAudio(source, title string, startAt time.Duration) (*Session, error) {
	return s.exec(s.driver.AudioArgs(source, title, startAt))
}

func (s *simpleBackend) Close() {}
