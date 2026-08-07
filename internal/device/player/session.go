package player

import (
	"sync"
	"time"
)

// Session represents a single active playback instance.
// Position and lifecycle are owned per-session, so overlapping playbacks
// cannot corrupt each other's state.
type Session struct {
	doneOnce sync.Once
	doneCh   chan struct{} // closed when the player process exits

	stopOnce sync.Once
	stopCh   chan struct{} // closed to signal the poll goroutine to exit

	mu      sync.Mutex
	lastPos time.Duration
	active  bool
}

func newSession(startAt time.Duration) *Session {
	return &Session{
		doneCh:  make(chan struct{}),
		stopCh:  make(chan struct{}),
		lastPos: startAt,
		active:  true,
	}
}

// NewSession creates a Session initialized to startAt, active and not-yet-done.
// It is the construction seam for the position-tracking logic — used by backends
// and by tests that exercise that logic without launching a real player.
func NewSession(startAt time.Duration) *Session { return newSession(startAt) }

// Finish marks the session ended: it closes Done() (exactly once, so a poll
// goroutine and the process-exit reaper can't double-close) and stops the poll
// signal. Called by a backend's exit reaper when the player process exits.
func (s *Session) Finish() {
	s.doneOnce.Do(func() { close(s.doneCh) })
	s.stop()
}

// Position returns the last known playback position and whether it is valid.
func (s *Session) Position() (time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastPos, s.active
}

// Done returns a channel that is closed when the player process exits.
func (s *Session) Done() <-chan struct{} { return s.doneCh }

func (s *Session) setPosition(pos time.Duration, active bool) {
	s.mu.Lock()
	if active {
		s.lastPos = pos
	}
	s.active = active
	s.mu.Unlock()
}

// stop signals the poll goroutine to exit. Safe to call multiple times.
func (s *Session) stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}
