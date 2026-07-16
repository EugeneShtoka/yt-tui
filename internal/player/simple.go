package player

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

type simpleBackend struct{ driver Driver }

func newSimpleBackend(driver Driver) *simpleBackend {
	return &simpleBackend{driver: driver}
}

func (s *simpleBackend) exec(args []string) error {
	cmd := exec.CommandContext(context.Background(), s.driver.Path(), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("exec: %w", err)
	}
	return nil
}

func (s *simpleBackend) Launch(source string, startAt time.Duration) error {
	return s.exec(s.driver.Args(source, startAt))
}

func (s *simpleBackend) LaunchAudio(source string, startAt time.Duration) error {
	return s.exec(s.driver.AudioArgs(source, startAt))
}

func (s *simpleBackend) Position() (time.Duration, bool) { return 0, false }
func (s *simpleBackend) Close()                          {}
