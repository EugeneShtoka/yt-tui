package downloader

import (
	"context"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
)

// TestSubscribeFanOutToAllSubscribers verifies the C-2 fix: N concurrent
// subscribers each receive every event emitted after they register, instead
// of the events being split between them off one shared channel.
func TestSubscribeFanOutToAllSubscribers(t *testing.T) {
	d := New(&config.Config{}, nil)

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	ch1 := d.Subscribe(ctx1)
	ch2 := d.Subscribe(ctx2)

	const n = 5
	for i := range n {
		d.emit(Event{Kind: EventProgress, VideoID: "v", Progress: float64(i)})
	}

	for _, ch := range []<-chan Event{ch1, ch2} {
		for i := range n {
			select {
			case ev := <-ch:
				if ev.Progress != float64(i) {
					t.Fatalf("subscriber got event %d out of order: %+v", i, ev)
				}
			case <-time.After(time.Second):
				t.Fatalf("subscriber did not receive event %d", i)
			}
		}
	}
}

// TestSubscribeCancelUnregistersAndCloses verifies that canceling a
// subscription's context removes it from the broadcaster's registry and
// closes its channel — the mechanism the Downloading tab relies on to avoid
// leaking a goroutine per resubscribe (C-2).
func TestSubscribeCancelUnregistersAndCloses(t *testing.T) {
	d := New(&config.Config{}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	ch := d.Subscribe(ctx)

	d.events.mu.Lock()
	n := len(d.events.subs)
	d.events.mu.Unlock()
	if n != 1 {
		t.Fatalf("expected 1 registered subscriber, got %d", n)
	}

	cancel()

	deadline := time.After(time.Second)
	for {
		d.events.mu.Lock()
		n := len(d.events.subs)
		d.events.mu.Unlock()
		if n == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("subscriber was not unregistered after cancel")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel delivered a value instead of closing")
		}
	case <-time.After(time.Second):
		t.Fatal("channel was not closed after cancel")
	}
}

// TestSubscribeCanceledSubscriberDoesNotBlockOthers verifies that a canceled
// subscriber never receives events emitted after cancellation, while a live
// sibling subscriber keeps receiving normally — a resubscribe cycle must not
// affect other listeners.
func TestSubscribeCanceledSubscriberDoesNotBlockOthers(t *testing.T) {
	d := New(&config.Config{}, nil)

	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	ch1 := d.Subscribe(ctx1)
	ch2 := d.Subscribe(ctx2)

	cancel1()
	// Drain ch1's close.
	select {
	case <-ch1:
	case <-time.After(time.Second):
		t.Fatal("ch1 was not closed after cancel")
	}

	d.emit(Event{Kind: EventComplete, VideoID: "v1"})

	select {
	case ev := <-ch2:
		if ev.VideoID != "v1" {
			t.Fatalf("unexpected event on live subscriber: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("live subscriber did not receive event after sibling canceled")
	}
}
