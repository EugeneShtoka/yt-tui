// Package prewarm eagerly warms a remote client's local thumbnail cache with the
// daemon's eligible set, so thumbnails render instantly — and offline — without
// waiting for each video to be opened. It walks the eligible IDs through the
// media seam with a bounded worker pool; the seam does the actual caching and
// deduplication (a local-cache hit is a no-op fetch), so this package holds no
// storage logic of its own.
//
// Only thumbnails are warmed: they are CDN image GETs (or cheap daemon reads),
// safe to parallelize hard. Transcripts are deliberately excluded — they hit
// yt-dlp on the daemon and, until a client-side transcript cache exists, warming
// them would do throttled work with nothing kept locally.
package prewarm

import (
	"context"
	"sync"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
)

// Source supplies the video IDs to warm — the daemon's eligible thumbnail set.
// api.VideoBackend satisfies it via EligibleThumbnailIDs.
type Source interface {
	EligibleThumbnailIDs(ctx context.Context) (map[string]bool, error)
}

// defaultConcurrency bounds the thumbnail worker pool when none is configured.
const defaultConcurrency = 16

// Warmer pulls eligible thumbnails through the media seam into the local cache.
type Warmer struct {
	src         Source
	media       api.MediaProvider
	concurrency int
}

// New builds a Warmer. concurrency <= 0 uses defaultConcurrency.
func New(src Source, media api.MediaProvider, concurrency int) *Warmer {
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}
	return &Warmer{src: src, media: media, concurrency: concurrency}
}

// Run lists the eligible set and warms the local thumbnail cache with a bounded
// worker pool, returning when the set is exhausted or ctx is canceled. It is
// best-effort: a failure to list the set skips the pass, and per-thumbnail
// misses are the seam's to log. Blocks — run it in a goroutine.
func (w *Warmer) Run(ctx context.Context) {
	if w == nil || w.src == nil || w.media == nil {
		return
	}
	ids, err := w.src.EligibleThumbnailIDs(ctx)
	if err != nil {
		debug.Log("prewarm: list eligible IDs: %v", err)
		return
	}
	if len(ids) == 0 {
		return
	}

	work := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < w.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range work {
				// The seam checks the local cache first (a hit is a no-op) and writes
				// the fetched bytes back, so this call both dedups and populates. We
				// only want the caching side effect; the returned image is discarded.
				_, _, _ = w.media.GetThumbnail(ctx, id, "")
			}
		}()
	}

	dispatched := 0
loop:
	for id := range ids {
		select {
		case <-ctx.Done():
			break loop
		case work <- id:
			dispatched++
		}
	}
	close(work)
	wg.Wait()
	debug.Log("prewarm: dispatched %d/%d eligible thumbnail(s)", dispatched, len(ids))
}
