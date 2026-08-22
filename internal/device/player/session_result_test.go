package player

import (
	"errors"
	"testing"
	"time"
)

// TestSessionResultUnavailableWhileRunning: nothing to report until the process
// has actually exited.
func TestSessionResultUnavailableWhileRunning(t *testing.T) {
	sess := NewSession(0)
	if _, ok := sess.Result(); ok {
		t.Error("a running session must have no result")
	}
}

// TestSessionResultRecordsExit: the reaper's exit error and stderr tail stay
// attached to the session that produced them.
func TestSessionResultRecordsExit(t *testing.T) {
	sess := NewSession(0)
	sess.finishWith(errors.New("wait failed"), "ERROR: 403")

	res, ok := sess.Result()
	if !ok {
		t.Fatal("a finished session must have a result")
	}
	if res.ExitCode != -1 || res.Output != "ERROR: 403" || res.Played {
		t.Errorf("result = %+v, want exit -1, the captured stderr, and Played=false", res)
	}
	if res.Ran <= 0 {
		t.Errorf("Ran = %v, want a positive duration", res.Ran)
	}
	select {
	case <-sess.Done():
	default:
		t.Error("finishWith must end the session")
	}
}

// TestSessionResultReportsPlayback: a session that ever reported a live position
// played, and a failure diagnosis keys off exactly that.
func TestSessionResultReportsPlayback(t *testing.T) {
	sess := NewSession(0)
	sess.setPosition(30*time.Second, true)
	sess.setPosition(0, false) // player closed; position no longer live
	sess.finishWith(nil, "")

	res, ok := sess.Result()
	if !ok {
		t.Fatal("a finished session must have a result")
	}
	if !res.Played {
		t.Error("a session that reported a live position must count as played")
	}
}

// TestSessionResultSuppressedWhenSuperseded: launching a second video kills the
// first player on purpose, which from the outside looks exactly like a crash. A
// superseded session must therefore report nothing.
func TestSessionResultSuppressedWhenSuperseded(t *testing.T) {
	sess := NewSession(0)
	sess.supersede()
	sess.finishWith(errors.New("signal: killed"), "")
	if _, ok := sess.Result(); ok {
		t.Error("a deliberately killed session must not report a result")
	}
}
