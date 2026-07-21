package downloader

import (
	"sync"
	"testing"
	"time"
)

// TestEmitNeverBlocks verifies that concurrent emit calls never block, even
// when the event channel is full and no consumer is draining it.
func TestEmitNeverBlocks(t *testing.T) {
	d := &Downloader{
		eventCh: make(chan Event, 64),
	}

	const producers = 3
	const eventsEach = 200

	done := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		for range producers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range eventsEach {
					d.emit(Event{Kind: EventProgress, VideoID: "test"})
				}
			}()
		}
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// passed
	case <-time.After(2 * time.Second):
		t.Fatal("emit blocked: goroutines did not finish within 2s")
	}
}

// TestEmitCompletionEventDelivered verifies that a completion event is
// eventually present in the channel even when the buffer was briefly full.
func TestEmitCompletionEventDelivered(t *testing.T) {
	d := &Downloader{
		eventCh: make(chan Event, 64),
	}

	// Fill the buffer with progress events.
	for range 64 {
		d.eventCh <- Event{Kind: EventProgress}
	}

	// emit a completion event — must not block.
	done := make(chan struct{})
	go func() {
		d.emit(Event{Kind: EventComplete, VideoID: "v1"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("emit blocked on full channel")
	}

	// Drain and confirm completion event is present.
	var found bool
	timeout := time.After(time.Second)
	for !found {
		select {
		case ev := <-d.eventCh:
			if ev.Kind == EventComplete && ev.VideoID == "v1" {
				found = true
			}
		case <-timeout:
			t.Fatal("completion event not found in channel")
		}
	}
}
