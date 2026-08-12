package tab

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The recentSearches sub-model is pure list-navigation logic; test it directly.

func TestRecentSearches_MoveClampsAndWraps(t *testing.T) {
	r := recentSearches{queries: []string{"a", "b", "c"}}
	r.move(-1, 5) // non-circular: clamp at 0
	if r.cursor != 0 {
		t.Fatalf("clamp low: cursor=%d, want 0", r.cursor)
	}
	r.move(5, 5) // clamp at n-1
	if r.cursor != 2 {
		t.Fatalf("clamp high: cursor=%d, want 2", r.cursor)
	}

	rc := recentSearches{queries: []string{"a", "b", "c"}, listCursor: listCursor{circular: true}}
	rc.move(-1, 5) // wrap to last
	if rc.cursor != 2 {
		t.Fatalf("wrap down: cursor=%d, want 2", rc.cursor)
	}
	rc.move(1, 5) // wrap back to first
	if rc.cursor != 0 {
		t.Fatalf("wrap up: cursor=%d, want 0", rc.cursor)
	}
}

func TestRecentSearches_Window(t *testing.T) {
	r := recentSearches{queries: []string{"a", "b", "c", "d", "e"}}
	if s, e := r.window(10); s != 0 || e != 5 {
		t.Errorf("pageH>=n: window=(%d,%d), want (0,5)", s, e)
	}
	if s, e := r.window(2); s != 0 || e != 2 {
		t.Errorf("top window=(%d,%d), want (0,2)", s, e)
	}
	empty := recentSearches{}
	if s, e := empty.window(3); s != 0 || e != 0 {
		t.Errorf("empty window=(%d,%d), want (0,0)", s, e)
	}
}

func TestRecentSearches_PageJumpKeepCursorVisible(t *testing.T) {
	r := recentSearches{queries: make([]string, 10)}
	for i := range r.queries {
		r.queries[i] = string(rune('a' + i))
	}
	r.syncViewport(0, 3)
	r.page(1, 3) // page down moves the viewport off the top
	if r.vs == 0 {
		t.Errorf("page down should advance the viewport, vs=%d", r.vs)
	}
	r.jumpTo(9, 3)
	if r.cursor != 9 {
		t.Fatalf("jumpTo: cursor=%d, want 9", r.cursor)
	}
	if s, e := r.window(3); 9 < s || 9 >= e {
		t.Errorf("cursor 9 not visible in window [%d,%d)", s, e)
	}
}

// Smoke test: constructing Search and exercising its trivial accessors + Init
// loaders keeps the tab's simple surface covered.
func TestSearch_AccessorsAndInit(t *testing.T) {
	s := NewSearch(context.Background(), &fakeBackend{}, testKeys(), false)
	_ = s.ID()
	_ = s.Title()
	_ = s.ShortHelp()
	_ = s.InterceptsInput()
	_ = s.Loading()
	if _, ok := s.SelectedVideo(); ok {
		t.Error("a fresh Search should have no selected video")
	}
	if batch, ok := runCmd(s.Init()).(tea.BatchMsg); ok {
		for _, c := range batch {
			runCmd(c)
		}
	}
}
