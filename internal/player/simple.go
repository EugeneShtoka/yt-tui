package player

import (
	"os/exec"
	"syscall"
	"time"
)

// simpleBackend launches a player without position tracking.
type simpleBackend struct {
	path string
}

func newSimpleBackend(path string) *simpleBackend {
	return &simpleBackend{path: path}
}

func (s *simpleBackend) Launch(filePath string, startAt time.Duration) error {
	args := startArgs(s.path, filePath, startAt)
	cmd := exec.Command(s.path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

func (s *simpleBackend) Position() (time.Duration, bool) { return 0, false }
func (s *simpleBackend) Close()                          {}
