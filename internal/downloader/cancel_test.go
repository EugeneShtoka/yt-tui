package downloader

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// A download still queued (blocked on the concurrency semaphore) must carry a
// cancel func so Remove/Stop can cancel it. Before H-4 the cancel was set only
// after the semaphore was acquired, so queued items were uncancellable and
// their goroutines leaked on shutdown.
func TestQueuedDownloadIsCancellable(t *testing.T) {
	d := New(&config.Config{}, nil)

	// Occupy every slot so a newly started item stays queued (never execs).
	for range cap(d.semaphore) {
		d.semaphore <- struct{}{}
	}

	d.Start(domain.Video{ID: "v1", URL: "http://example/v1"}, TypeVideo)

	d.mu.RLock()
	it, ok := d.items["v1"]
	hasCancel := ok && it.cancel != nil
	d.mu.RUnlock()

	if !ok {
		t.Fatal("item not registered after Start")
	}
	if !hasCancel {
		t.Fatal("queued item has nil cancel func; Remove/Stop cannot cancel it")
	}

	// Remove cancels the queued goroutine (via ctx.Done) and drops the item.
	d.Remove("v1")
	if d.IsDownloading("v1") {
		t.Fatal("item still present/downloading after Remove")
	}
}
