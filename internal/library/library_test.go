package library

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/feed"
)

func TestNewIndexesByID(t *testing.T) {
	l := New([]db.LocalVideo{{ID: "a"}, {ID: "b"}})
	if l.Len() != 2 {
		t.Fatalf("Len = %d, want 2", l.Len())
	}
	if v, ok := l.ByID("b"); !ok || v.ID != "b" {
		t.Errorf("ByID(b) = %v,%v", v.ID, ok)
	}
	if !l.Has("a") || l.Has("z") {
		t.Errorf("Has: a=%v z=%v, want true/false", l.Has("a"), l.Has("z"))
	}
}

// Set must rebuild the by-ID index — the bug this owner exists to prevent was
// sites reassigning the slice but leaving a stale index behind.
func TestSetRebuildsIndex(t *testing.T) {
	l := New([]db.LocalVideo{{ID: "a"}, {ID: "b"}})
	// Simulate a delete: reload with "a" gone.
	l.Set([]db.LocalVideo{{ID: "b"}})
	if l.Has("a") {
		t.Errorf("stale index: 'a' still present after Set without it")
	}
	if !l.Has("b") || l.Len() != 1 {
		t.Errorf("after Set: has(b)=%v len=%d", l.Has("b"), l.Len())
	}
}

func TestClear(t *testing.T) {
	l := New([]db.LocalVideo{{ID: "a"}})
	l.Clear()
	if l.Len() != 0 || l.Has("a") {
		t.Errorf("after Clear: len=%d has(a)=%v", l.Len(), l.Has("a"))
	}
}

func TestIDsMap(t *testing.T) {
	// IDs() exposes the by-ID index (consumed read-only by feed.FilterDownloaded).
	l := New([]db.LocalVideo{{ID: "dl"}})
	if _, ok := l.IDs()["dl"]; !ok {
		t.Errorf("IDs() missing downloaded id")
	}
}

func TestSort(t *testing.T) {
	l := New([]db.LocalVideo{
		{ID: "a", ViewCount: 100},
		{ID: "b", ViewCount: 500},
		{ID: "c", ViewCount: 200},
	})
	l.Sort(feed.SortViews)
	got := l.videos[0].ID + l.videos[1].ID + l.videos[2].ID
	if got != "bca" {
		t.Errorf("Sort(views) order = %q, want bca", got)
	}
}
