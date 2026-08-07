package prewarm

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeSource returns a fixed eligible set (or an error).
type fakeSource struct {
	ids map[string]bool
	err error
}

func (f fakeSource) EligibleThumbnailIDs(context.Context) (map[string]bool, error) {
	return f.ids, f.err
}

// fakeMedia records which IDs GetThumbnail was called for (thread-safe) and
// counts transcript calls to assert they are never made by the warmer.
type fakeMedia struct {
	mu          sync.Mutex
	thumbCalls  map[string]int
	transcripts int
}

func newFakeMedia() *fakeMedia { return &fakeMedia{thumbCalls: map[string]int{}} }

func (m *fakeMedia) GetThumbnail(_ context.Context, videoID, _ string) ([]byte, bool, error) {
	m.mu.Lock()
	m.thumbCalls[videoID]++
	m.mu.Unlock()
	return []byte("img"), true, nil
}

func (m *fakeMedia) GetTranscript(_ context.Context, _, _ string) (string, bool, error) {
	m.mu.Lock()
	m.transcripts++
	m.mu.Unlock()
	return "", false, nil
}

func TestWarmerWarmsEveryEligibleThumbnailOnce(t *testing.T) {
	ids := map[string]bool{"a": true, "b": true, "c": true, "d": true, "e": true}
	media := newFakeMedia()
	New(fakeSource{ids: ids}, media, 3).Run(context.Background())

	if len(media.thumbCalls) != len(ids) {
		t.Fatalf("warmed %d distinct ids, want %d", len(media.thumbCalls), len(ids))
	}
	for id := range ids {
		if media.thumbCalls[id] != 1 {
			t.Errorf("id %q warmed %d times, want 1", id, media.thumbCalls[id])
		}
	}
	if media.transcripts != 0 {
		t.Errorf("transcripts fetched %d times; the warmer must only warm thumbnails", media.transcripts)
	}
}

func TestWarmerSkipsPassOnListError(t *testing.T) {
	media := newFakeMedia()
	New(fakeSource{err: errors.New("daemon down")}, media, 4).Run(context.Background())
	if len(media.thumbCalls) != 0 {
		t.Errorf("warmed %d thumbnails despite a list error, want 0", len(media.thumbCalls))
	}
}

func TestWarmerStopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // canceled before Run
	media := newFakeMedia()
	New(fakeSource{ids: map[string]bool{"a": true, "b": true, "c": true}}, media, 2).Run(ctx)
	// The dispatch loop checks ctx before sending, so a pre-canceled context yields
	// no work — and Run returns promptly rather than hanging.
	if len(media.thumbCalls) != 0 {
		t.Errorf("warmed %d thumbnails after cancellation, want 0", len(media.thumbCalls))
	}
}
