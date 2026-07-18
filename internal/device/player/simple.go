package player

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type simpleBackend struct {
	driver Driver
	mu     sync.Mutex
	doneCh chan struct{}
}

func newSimpleBackend(driver Driver) *simpleBackend {
	return &simpleBackend{driver: driver}
}

func (s *simpleBackend) exec(args []string) error {
	null, err := os.Open(os.DevNull)
	if err != nil {
		return fmt.Errorf("exec: open devnull: %w", err)
	}
	defer null.Close()
	cmd := exec.CommandContext(context.Background(), s.driver.Path(), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = null
	cmd.Stderr = null
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("exec: %w", err)
	}
	done := make(chan struct{})
	s.mu.Lock()
	s.doneCh = done
	s.mu.Unlock()
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	return nil
}

func (s *simpleBackend) Launch(source, title string, startAt time.Duration) error {
	return s.exec(s.driver.Args(source, title, startAt))
}

func (s *simpleBackend) LaunchAudio(source, title string, startAt time.Duration) error {
	return s.exec(s.driver.AudioArgs(source, title, startAt))
}

func (s *simpleBackend) Position() (time.Duration, bool) { return 0, false }

func (s *simpleBackend) Wait() error {
	s.mu.Lock()
	ch := s.doneCh
	s.mu.Unlock()
	if ch == nil {
		return nil
	}
	<-ch
	return nil
}

func (s *simpleBackend) Close() {}
