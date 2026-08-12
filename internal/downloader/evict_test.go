package downloader

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/procexec"
)

// hasItem reports whether the queue currently holds an item for id.
func hasItem(d *Downloader, id string) bool {
	items := d.Items()
	for i := range items {
		if items[i].Video.ID == id {
			return true
		}
	}
	return false
}

// waitUntilGone polls the queue until id is absent or the deadline passes.
func waitUntilGone(t *testing.T, d *Downloader, id string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if !hasItem(d, id) {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("item %q still in queue after eviction window", id)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// Clear dismisses every queued item, canceling those still in flight. It is
// the bulk form of Remove and must leave the queue empty.
func TestClearDismissesEveryQueuedItem(t *testing.T) {
	d := New(&config.Config{}, nil)

	// Occupy every slot so started items stay queued (never exec / complete).
	for range cap(d.semaphore) {
		d.semaphore <- struct{}{}
	}
	for _, id := range []string{"v1", "v2", "v3"} {
		d.Start(domain.Video{ID: id, URL: "http://x/" + id}, TypeVideo)
	}
	if got := len(d.Items()); got != 3 {
		t.Fatalf("queued %d items, want 3", got)
	}

	d.Clear()

	if got := len(d.Items()); got != 0 {
		t.Fatalf("queue holds %d items after Clear, want 0", got)
	}
}

// A completed download must auto-evict from the queue after the grace period,
// so the queue self-cleans without a manual dismiss.
func TestCompletedItemAutoEvicts(t *testing.T) {
	d := New(&config.Config{}, newTestDB(t))
	d.evictAfter = 15 * time.Millisecond
	d.runner = procexec.FakeRunner{New: func([]string) procexec.Cmd {
		return &procexec.FakeCmd{Stdout: "[download] Destination: /tmp/x.mkv\n[download] 100.0% of ~5MiB at 2MiB/s ETA 00:00\n"}
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := d.Subscribe(ctx)

	d.Start(domain.Video{ID: "done1", Title: "T", Channel: "C", URL: "http://x/done1"}, TypeVideo)
	waitEvent(t, ch, EventComplete)

	waitUntilGone(t, d, "done1")
}

// A failed download must NOT auto-evict — it stays visible so the failure is
// seen, until a manual dismiss / Clear / restart.
func TestFailedItemNotEvicted(t *testing.T) {
	d := New(&config.Config{}, newTestDB(t))
	d.evictAfter = 15 * time.Millisecond
	d.runner = procexec.FakeRunner{New: func([]string) procexec.Cmd {
		return &procexec.FakeCmd{Stderr: "ERROR: nope", WaitErr: fmt.Errorf("exit status 1")}
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := d.Subscribe(ctx)

	d.Start(domain.Video{ID: "fail1", URL: "http://x/fail1"}, TypeVideo)
	waitEvent(t, ch, EventError)

	// Wait well past the eviction window; the failed item must survive.
	time.Sleep(60 * time.Millisecond)
	if !hasItem(d, "fail1") {
		t.Fatal("failed item was auto-evicted; failures must stay visible")
	}
}
