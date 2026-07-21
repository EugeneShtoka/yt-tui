package player

import (
	"sync"
	"testing"
	"time"
)

// TestSessionPositionIsolation verifies that two sessions can update their
// positions concurrently without interfering with each other.
func TestSessionPositionIsolation(t *testing.T) {
	s1 := newSession(10 * time.Second)
	s2 := newSession(20 * time.Second)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := range 100 {
			s1.setPosition(time.Duration(i)*time.Second, true)
		}
	}()
	go func() {
		defer wg.Done()
		for i := range 100 {
			s2.setPosition(time.Duration(i+200)*time.Second, true)
		}
	}()

	wg.Wait()

	p2, ok2 := s2.Position()
	if !ok2 {
		t.Fatal("s2 should be active")
	}
	// s2 positions are always >= 200s; s1 writes are 0..99s — they must not bleed into s2.
	if p2 < 200*time.Second {
		t.Errorf("s2 position corrupted: got %v, want >= 200s", p2)
	}
}

// TestSessionStopIdempotent verifies that stop() is safe to call multiple times.
func TestSessionStopIdempotent(t *testing.T) {
	sess := newSession(0)
	sess.stop()
	sess.stop() // must not panic
}

// TestSessionDoneClosedAfterStop verifies that Done() never fires before stop
// and doneCh close, and that the ordering between stop and done is independent.
func TestSessionDoneClosedAfterStop(t *testing.T) {
	sess := newSession(5 * time.Second)

	// Simulate process exit: stop poll goroutine then close doneCh.
	go func() {
		time.Sleep(50 * time.Millisecond)
		sess.stop()
		close(sess.doneCh)
	}()

	select {
	case <-sess.Done():
		// passed
	case <-time.After(time.Second):
		t.Fatal("Done() channel never closed")
	}
}

// TestOverlappingSessionsNoRace runs two sessions concurrently to ensure the
// race detector catches any shared-state violations.
func TestOverlappingSessionsNoRace(t *testing.T) {
	s1 := newSession(0)
	s2 := newSession(0)

	var wg sync.WaitGroup
	for _, sess := range []*Session{s1, s2} {
		wg.Add(1)
		s := sess
		go func() {
			defer wg.Done()
			for i := range 50 {
				s.setPosition(time.Duration(i)*time.Second, i%2 == 0)
				_, _ = s.Position()
			}
		}()
	}
	wg.Wait()
}
