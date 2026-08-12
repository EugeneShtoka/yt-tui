package downloader

import (
	"context"
	"sync"
)

// fanout is a fan-out broadcaster: every value sent to it is delivered to each
// live subscriber channel non-blockingly (drop-newest per subscriber, so one
// slow or stalled consumer can never hold up delivery to the others). A single
// broadcast goroutine drains the input channel; Subscribe may be called
// concurrently. Extracted from Downloader so the pub/sub concern stands alone
// from the download queue (H-4).
type fanout[T any] struct {
	in   chan T
	mu   sync.Mutex
	subs map[chan T]struct{}
}

// newFanout starts a fanout whose input channel is buffered to buffer and
// launches its broadcast goroutine. The goroutine runs for the fanout's
// lifetime (process lifetime here); there is no explicit stop, matching the
// prior broadcaster.
func newFanout[T any](buffer int) *fanout[T] {
	f := &fanout[T]{
		in:   make(chan T, buffer),
		subs: make(map[chan T]struct{}),
	}
	go f.broadcast()
	return f
}

// broadcast is the sole reader of the input channel. It fans each value out to
// every currently-registered subscriber, non-blockingly.
func (f *fanout[T]) broadcast() {
	for v := range f.in {
		f.mu.Lock()
		for ch := range f.subs {
			select {
			case ch <- v:
			default:
			}
		}
		f.mu.Unlock()
	}
}

// emit publishes v non-blockingly, dropping it if the input buffer is full.
// Drop-newest is race-free (a single send with a default), unlike a
// receive-then-resend which could drop a value slotted in by another emitter
// between the two steps.
func (f *fanout[T]) emit(v T) {
	select {
	case f.in <- v:
	default:
	}
}

// subscribe registers a new listener and returns a channel (buffered to buffer)
// that receives every value published from now on, until ctx is canceled — at
// which point the registration is removed and the channel is closed. Callers
// must cancel ctx when done or the registration and its goroutine leak.
func (f *fanout[T]) subscribe(ctx context.Context, buffer int) <-chan T {
	ch := make(chan T, buffer)
	f.mu.Lock()
	f.subs[ch] = struct{}{}
	f.mu.Unlock()
	go func() {
		<-ctx.Done()
		f.mu.Lock()
		delete(f.subs, ch)
		f.mu.Unlock()
		close(ch)
	}()
	return ch
}
