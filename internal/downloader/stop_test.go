package downloader

import (
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// TestStopJoinsQueuedGoroutine verifies Stop cancels queued downloads and
// blocks until their goroutines have exited (L-7 WaitGroup join), so daemon
// shutdown doesn't return while run goroutines are still live.
func TestStopJoinsQueuedGoroutine(t *testing.T) {
	d := New(&config.Config{}, nil)

	// Occupy every slot so the started item stays queued (blocked on the
	// semaphore) rather than execing yt-dlp.
	for range cap(d.semaphore) {
		d.semaphore <- struct{}{}
	}
	d.Start(domain.Video{ID: "v1", URL: "http://example/v1"}, TypeVideo)

	done := make(chan struct{})
	go func() {
		d.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Stop returned: the queued goroutine was canceled and joined.
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return; goroutine join blocked")
	}
}
