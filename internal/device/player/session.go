package player

import (
	"errors"
	"os/exec"
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

	startedAt time.Time

	mu         sync.Mutex
	lastPos    time.Duration
	active     bool
	played     bool // a position update ever reported live playback
	result     Result
	haveResult bool
	superseded bool // killed on purpose to start another video
}

// Result records how a player process ended. It carries enough to tell "the user
// watched and closed it" apart from "it never played anything", and enough to
// explain the second case: Output holds the tail of what the player (and, through
// it, yt-dlp) said on the way down.
type Result struct {
	ExitCode int           // process exit status; -1 when it died on a signal
	Ran      time.Duration // how long the process lived
	Played   bool          // playback was observed while it ran
	Output   string        // tail of what the player printed (stdout and stderr)
}

func newSession(startAt time.Duration) *Session {
	return &Session{
		doneCh:    make(chan struct{}),
		stopCh:    make(chan struct{}),
		startedAt: time.Now(),
		lastPos:   startAt,
		active:    true,
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

// finishWith records how the process ended and then ends the session. Backends
// call it from their cmd.Wait reaper so the exit status and the captured stderr
// stay attached to the session that produced them.
func (s *Session) finishWith(exitErr error, output string) {
	s.mu.Lock()
	s.result = Result{
		ExitCode: exitStatus(exitErr),
		Ran:      time.Since(s.startedAt),
		Played:   s.played,
		Output:   output,
	}
	s.haveResult = true
	s.mu.Unlock()
	s.Finish()
}

// Result returns how the process ended, once it has. ok=false while it is still
// running, when the backend recorded no outcome, and for a session deliberately
// killed to start another video: from the outside an intentional kill is
// indistinguishable from a crash, so it must never be reported as one.
func (s *Session) Result() (Result, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.haveResult || s.superseded {
		return Result{}, false
	}
	return s.result, true
}

// supersede marks the session as intentionally terminated, suppressing its
// Result. Called by a backend just before it kills the process to launch another.
func (s *Session) supersede() {
	s.mu.Lock()
	s.superseded = true
	s.mu.Unlock()
}

// exitStatus maps cmd.Wait's error onto a process exit code: 0 for a clean exit,
// the reported status when there is one, and -1 when the process died on a signal
// or the wait itself failed.
func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
	}
	return -1
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
		s.played = true
	}
	s.active = active
	s.mu.Unlock()
}

// stop signals the poll goroutine to exit. Safe to call multiple times.
func (s *Session) stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}
