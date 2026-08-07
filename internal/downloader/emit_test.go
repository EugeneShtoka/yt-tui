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
		events: &fanout[Event]{in: make(chan Event, 64), subs: map[chan Event]struct{}{}},
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

// TestEmitCompletionEventDelivered verifies that a completion event emitted
// while the channel has room is delivered to a consumer.
func TestEmitCompletionEventDelivered(t *testing.T) {
	d := &Downloader{
		events: &fanout[Event]{in: make(chan Event, 64), subs: map[chan Event]struct{}{}},
	}

	d.emit(Event{Kind: EventComplete, VideoID: "v1"})

	select {
	case ev := <-d.events.in:
		if ev.Kind != EventComplete || ev.VideoID != "v1" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("completion event not delivered")
	}
}

// TestEmitDropsNewestWhenFull verifies the race-free drop-newest contract: a
// full buffer causes emit to drop the incoming event rather than block or evict
// a buffered one. Dropped completion/error events self-heal via fetchItemsCmd
// polling, so losing one here is acceptable.
func TestEmitDropsNewestWhenFull(t *testing.T) {
	d := &Downloader{
		events: &fanout[Event]{in: make(chan Event, 64), subs: map[chan Event]struct{}{}},
	}

	// Fill the buffer with progress events.
	for range 64 {
		d.events.in <- Event{Kind: EventProgress}
	}

	// emit onto the full channel — must not block.
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

	// The buffered events must be untouched — drop-newest never evicts.
	if len(d.events.in) != 64 {
		t.Fatalf("expected 64 buffered events, got %d", len(d.events.in))
	}
	for range 64 {
		if ev := <-d.events.in; ev.Kind != EventProgress {
			t.Fatalf("buffered event was evicted: got %+v", ev)
		}
	}
}
