package feed

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

func TestNewStarting(t *testing.T) {
	// Seeded from cache → loaded, and immediately fetching with refreshing set.
	f := NewStarting([]youtube.Video{{ID: "a"}})
	if !f.Loaded() || !f.Loading() || !f.Refreshing() {
		t.Errorf("cache-seeded feed: loaded=%v loading=%v refreshing=%v, want all true", f.Loaded(), f.Loading(), f.Refreshing())
	}
	if f.Len() != 1 {
		t.Errorf("len = %d, want 1", f.Len())
	}

	// Empty cache → not loaded, fetching but not "refreshing" (nothing underneath).
	e := NewStarting(nil)
	if e.Loaded() || !e.Loading() || e.Refreshing() {
		t.Errorf("empty feed: loaded=%v loading=%v refreshing=%v, want F/T/F", e.Loaded(), e.Loading(), e.Refreshing())
	}
}

func TestNew(t *testing.T) {
	// New holds videos with no fetch in flight (Subscriptions-style feed).
	f := New([]youtube.Video{{ID: "a"}, {ID: "b"}})
	if f.Loading() || f.Refreshing() {
		t.Errorf("New feed should not be fetching: loading=%v refreshing=%v", f.Loading(), f.Refreshing())
	}
	if !f.Loaded() || f.Len() != 2 {
		t.Errorf("New feed: loaded=%v len=%d, want true/2", f.Loaded(), f.Len())
	}
	if e := New(nil); e.Loaded() {
		t.Errorf("New(nil) loaded=true, want false")
	}
}

func TestFeedFetchLifecycle(t *testing.T) {
	var f Feed
	f.StartRefresh()
	if !f.Loading() || f.Refreshing() || f.Page() != 0 {
		t.Errorf("StartRefresh on empty: loading=%v refreshing=%v page=%d", f.Loading(), f.Refreshing(), f.Page())
	}
	f.FinishFetch()
	if f.Loading() || f.Refreshing() || !f.Loaded() || f.Page() != 1 {
		t.Errorf("FinishFetch: loading=%v refreshing=%v loaded=%v page=%d", f.Loading(), f.Refreshing(), f.Loaded(), f.Page())
	}
	// A second refresh over loaded content sets refreshing.
	f.StartRefresh()
	if !f.Refreshing() || f.Page() != 0 {
		t.Errorf("StartRefresh over loaded: refreshing=%v page=%d, want true/0", f.Refreshing(), f.Page())
	}
	f.ContinueFetch()
	if !f.Loading() || !f.Refreshing() {
		t.Errorf("ContinueFetch: loading=%v refreshing=%v, want true/true", f.Loading(), f.Refreshing())
	}
}

func TestFeedAtAndMutations(t *testing.T) {
	var f Feed
	f.SetVideos([]youtube.Video{{ID: "a"}, {ID: "b"}, {ID: "c"}})
	if v, ok := f.At(1); !ok || v.ID != "b" {
		t.Errorf("At(1) = %v,%v, want b,true", v.ID, ok)
	}
	if _, ok := f.At(9); ok {
		t.Errorf("At(9) ok=true, want false")
	}
	f.RemoveVideo("b")
	if f.Len() != 2 || f.hasID("b") {
		t.Errorf("after RemoveVideo(b): %v", ids(f.videos))
	}
	f.SetVideos([]youtube.Video{{ID: "a", ChannelID: "ch1"}, {ID: "b", ChannelID: "ch2"}})
	f.RemoveChannel("ch1", "")
	if f.Len() != 1 || f.videos[0].ID != "b" {
		t.Errorf("after RemoveChannel(ch1): %v", ids(f.videos))
	}
	f.Clear()
	if f.Len() != 0 || f.Loaded() {
		t.Errorf("after Clear: len=%d loaded=%v", f.Len(), f.Loaded())
	}
}

func TestFeedSort(t *testing.T) {
	var f Feed
	f.SetVideos([]youtube.Video{
		{ID: "a", ViewCount: 100},
		{ID: "b", ViewCount: 500},
		{ID: "c", ViewCount: 200},
	})
	f.Sort(SortViews)
	if ids(f.videos) != "b c a" {
		t.Errorf("Sort(views) = %q, want 'b c a'", ids(f.videos))
	}
}

// Merge must run the full filter pipeline, sort, store, and remap the cursor.
func TestFeedMerge(t *testing.T) {
	var f Feed
	f.SetVideos([]youtube.Video{{ID: "keep", ViewCount: 1000}})
	cfg := &config.Config{}
	incoming := []youtube.Video{
		{ID: "downloaded", ViewCount: 50},          // filtered: already local
		{ID: "hidden", ViewCount: 50},              // filtered: hidden
		{ID: "sub", ViewCount: 50, ChannelID: "s"}, // filtered: subscribed channel
		{ID: "new", ViewCount: 300},                // kept
	}
	newCursor := f.Merge(incoming, 0, MergeOpts{
		Downloaded: map[string]db.LocalVideo{"downloaded": {ID: "downloaded"}},
		Hidden:     map[string]bool{"hidden": true},
		Subscribed: map[string]bool{"s": true},
		Cfg:        cfg,
		Sort:       SortViews,
	})
	if ids(f.videos) != "keep new" { // 1000 > 300, both survive filters
		t.Fatalf("Merge result = %q, want 'keep new'", ids(f.videos))
	}
	// cursor was on "keep" (index 0); it stays index 0 after sort (highest views).
	if newCursor != 0 {
		t.Errorf("preserved cursor = %d, want 0", newCursor)
	}
}

// helpers
func (f *Feed) hasID(id string) bool {
	for _, v := range f.videos {
		if v.ID == id {
			return true
		}
	}
	return false
}
