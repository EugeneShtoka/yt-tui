package enrich

import (
	"context"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/backend/thumbs"
)

// newThumbStore builds a real on-disk thumb store in a temp dir. Put passes
// non-JPEG bytes through unchanged, so tests can seed the cache without images.
func newThumbStore(t *testing.T) *thumbs.Store {
	t.Helper()
	s, err := thumbs.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

// TestRunThumbnailsSkipsCachedAndEvictsStale exercises the offline half of the
// thumbnail pass: already-cached eligible ids are skipped (no network) and cached
// ids that dropped out of the eligible set are evicted by Retain. Since every
// eligible id is pre-cached, Download is never reached.
func TestRunThumbnailsSkipsCachedAndEvictsStale(t *testing.T) {
	store := newThumbStore(t)
	if err := store.Put("keep1", []byte("img")); err != nil {
		t.Fatalf("seed keep1: %v", err)
	}
	if err := store.Put("stale1", []byte("img")); err != nil {
		t.Fatalf("seed stale1: %v", err)
	}

	cat := newFakeCat()
	cat.eligible = map[string]bool{"keep1": true} // stale1 no longer eligible
	e := &Enricher{cat: cat, th: store, p: Params{ThumbnailsPerChannel: 30}, sleep: noWait}

	e.runThumbnails(context.Background())

	if !store.Has("keep1") {
		t.Error("eligible cached thumbnail keep1 should be retained")
	}
	if store.Has("stale1") {
		t.Error("stale1 dropped from the eligible set should be evicted")
	}
}

// TestRunThumbnailsStopsOnCancelledContext: a pre-canceled context makes the
// pass return at the loop guard before any Download, so nothing is cached.
func TestRunThumbnailsStopsOnCancelledContext(t *testing.T) {
	store := newThumbStore(t)
	cat := newFakeCat()
	cat.eligible = map[string]bool{"x": true}
	e := &Enricher{cat: cat, th: store, p: Params{ThumbnailsPerChannel: 30}, sleep: sleepCtx}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e.runThumbnails(ctx)

	if store.Has("x") {
		t.Error("canceled pass must not download/cache any thumbnail")
	}
}

// TestSleepCtx covers the cancellation primitive that paces every enrichment
// loop: it must honor ctx even for a non-positive delay, return true only when
// the timer actually elapses, and return false the moment ctx is canceled.
func TestSleepCtx(t *testing.T) {
	t.Run("zero delay, live ctx → true immediately", func(t *testing.T) {
		if !sleepCtx(context.Background(), 0) {
			t.Error("want true for d<=0 with a live context")
		}
	})
	t.Run("zero delay, canceled ctx → false", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if sleepCtx(ctx, 0) {
			t.Error("want false for d<=0 with a canceled context")
		}
	})
	t.Run("positive delay elapses → true", func(t *testing.T) {
		if !sleepCtx(context.Background(), 5*time.Millisecond) {
			t.Error("want true when the timer elapses")
		}
	})
	t.Run("positive delay, pre-canceled → false", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if sleepCtx(ctx, time.Hour) {
			t.Error("want false when ctx is already canceled")
		}
	})
	t.Run("cancel during wait → false promptly", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		go func() { time.Sleep(5 * time.Millisecond); cancel() }()
		start := time.Now()
		if sleepCtx(ctx, time.Hour) {
			t.Error("want false when canceled mid-wait")
		}
		if time.Since(start) > time.Second {
			t.Error("sleepCtx did not return promptly on cancel")
		}
	})
}
