package api

import (
	"bytes"
	"context"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/backend/thumbs"
	"github.com/EugeneShtoka/yt-tui/internal/backend/transcache"
)

// fakeMedia is an inner MediaProvider double: it records thumbnail calls and
// returns canned results, so the wrapper's routing/caching is observable
// without any network.
type fakeMedia struct {
	thumbData  []byte
	thumbOK    bool
	thumbCalls int

	transcript      string
	transcriptOK    bool
	transcriptCalls int
}

func (f *fakeMedia) GetThumbnail(context.Context, string, string) ([]byte, bool, error) {
	f.thumbCalls++
	return f.thumbData, f.thumbOK, nil
}

func (f *fakeMedia) GetTranscript(context.Context, string, string) (string, bool, error) {
	f.transcriptCalls++
	return f.transcript, f.transcriptOK, nil
}

// TestNewMediaProviderUnwrapsWhenNothingToAdd: with no local cache and a daemon
// that serves thumbnails, the wrapper would add nothing, so inner is returned
// verbatim (the common single-binary path stays a plain pass-through).
func TestNewMediaProviderUnwrapsWhenNothingToAdd(t *testing.T) {
	inner := &fakeMedia{}
	if got := NewMediaProvider(inner, nil, nil, true); got != MediaProvider(inner) {
		t.Errorf("expected inner returned unwrapped, got %T", got)
	}
	// A local cache means there is something to add, so it must wrap.
	store, err := thumbs.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if got := NewMediaProvider(inner, store, nil, true); got == MediaProvider(inner) {
		t.Error("expected a wrapper when a local cache is configured")
	}
}

// TestCachingMediaServesFromLocalCache: a local-cache hit short-circuits the
// daemon entirely — inner is never consulted.
func TestCachingMediaServesFromLocalCache(t *testing.T) {
	store, err := thumbs.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	want := []byte("cached-bytes")
	if perr := store.Put("vid1", want); perr != nil {
		t.Fatalf("Put: %v", perr)
	}
	inner := &fakeMedia{thumbOK: false} // would be a miss if consulted
	m := NewMediaProvider(inner, store, nil, true)

	data, ok, err := m.GetThumbnail(context.Background(), "vid1", "")
	if err != nil || !ok || !bytes.Equal(data, want) {
		t.Fatalf("got (%q,%v,%v), want cached hit", data, ok, err)
	}
	if inner.thumbCalls != 0 {
		t.Errorf("daemon consulted on a local-cache hit (%d calls)", inner.thumbCalls)
	}
}

// TestCachingMediaRoutesThroughDaemonAndPopulatesLocal: a miss routes to the
// daemon (serverThumbs=true) and writes the result to the local cache, so the
// next open is served locally without a second daemon call.
func TestCachingMediaRoutesThroughDaemonAndPopulatesLocal(t *testing.T) {
	store, err := thumbs.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	inner := &fakeMedia{thumbData: []byte("daemon-bytes"), thumbOK: true}
	m := NewMediaProvider(inner, store, nil, true)

	if _, ok, _ := m.GetThumbnail(context.Background(), "vid2", ""); !ok {
		t.Fatal("first open: expected a hit via the daemon")
	}
	if inner.thumbCalls != 1 {
		t.Fatalf("first open: want 1 daemon call, got %d", inner.thumbCalls)
	}
	if !store.Has("vid2") {
		t.Error("daemon bytes were not written to the local cache")
	}
	if _, ok, _ := m.GetThumbnail(context.Background(), "vid2", ""); !ok {
		t.Fatal("second open: expected a local-cache hit")
	}
	if inner.thumbCalls != 1 {
		t.Errorf("second open re-consulted the daemon (%d calls); should be served locally", inner.thumbCalls)
	}
}

// TestCachingMediaThumbnailMiss: a daemon miss (serverThumbs=true) surfaces as a
// blank result, not an error — nothing is cached.
func TestCachingMediaThumbnailMiss(t *testing.T) {
	store, err := thumbs.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	inner := &fakeMedia{thumbOK: false}
	m := NewMediaProvider(inner, store, nil, true)

	if data, ok, err := m.GetThumbnail(context.Background(), "vid3", ""); ok || err != nil || data != nil {
		t.Errorf("got (%q,%v,%v), want a clean miss", data, ok, err)
	}
	if store.Has("vid3") {
		t.Error("a miss must not populate the local cache")
	}
}

// TestCachingMediaTranscriptPassThrough: transcripts always route to the daemon
// unchanged, regardless of the local thumbnail cache.
func TestCachingMediaTranscriptPassThrough(t *testing.T) {
	store, err := thumbs.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	inner := &fakeMedia{transcript: "hello", transcriptOK: true}
	m := NewMediaProvider(inner, store, nil, true)

	text, ok, err := m.GetTranscript(context.Background(), "vid4", "https://youtu.be/vid4")
	if err != nil || !ok || text != "hello" {
		t.Errorf("got (%q,%v,%v), want the daemon's transcript verbatim", text, ok, err)
	}
}

// TestCachingMediaCachesTranscriptLocally: with a local transcript cache, the
// first read routes to the backend and is cached; the second is served locally
// without consulting the backend again.
func TestCachingMediaCachesTranscriptLocally(t *testing.T) {
	tstore, err := transcache.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("transcache.NewStore: %v", err)
	}
	inner := &fakeMedia{transcript: "line one\nline two", transcriptOK: true}
	m := NewMediaProvider(inner, nil, tstore, true)

	if text, ok, _ := m.GetTranscript(context.Background(), "vid5", "u"); !ok || text != "line one\nline two" {
		t.Fatalf("first read: got (%q,%v), want the backend's text cached", text, ok)
	}
	if text, ok, _ := m.GetTranscript(context.Background(), "vid5", "u"); !ok || text != "line one\nline two" {
		t.Errorf("second read: got (%q,%v), want the locally-cached text", text, ok)
	}
	if inner.transcriptCalls != 1 {
		t.Errorf("backend consulted %d times; the second read should be a local hit", inner.transcriptCalls)
	}
}
