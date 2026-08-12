package tab

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// These tests characterize the Search tab's three-mode input/recent state
// machine (M-10): entering recent mode, walking input history, navigating the
// recent list, and the histIdx↔recent.cursor clamping on reload.

func newSearchTab() Search {
	return NewSearch(context.Background(), &fakeBackend{}, testKeys(), false)
}

func asSearch(t *testing.T, m tea.Model) Search {
	t.Helper()
	s, ok := m.(Search)
	if !ok {
		t.Fatalf("routeKey returned %T, want Search", m)
	}
	return s
}

func TestSearchEscEntersRecentModeOnlyWithHistory(t *testing.T) {
	s := newSearchTab()
	s.input.Focus()
	s.recent.queries = []string{"a", "b"}
	s.height = 20

	m, _ := s.routeKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := asSearch(t, m)
	if !got.recentMode {
		t.Fatal("Esc with history should enter recent mode")
	}
	if got.recent.cursor != 0 {
		t.Errorf("recent cursor = %d, want 0", got.recent.cursor)
	}

	// No history → Esc must not enter recent mode (nothing to show).
	empty := newSearchTab()
	empty.input.Focus()
	m2, _ := empty.routeKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if asSearch(t, m2).recentMode {
		t.Error("Esc with no history should stay in input mode")
	}
}

func TestSearchInputHistoryWalk(t *testing.T) {
	s := newSearchTab()
	s.input.Focus()
	s.recent.queries = []string{"newest", "mid", "oldest"}
	s.height = 20
	s.histIdx = -1

	// Up walks toward older entries, mirroring the query into the input.
	m, _ := s.routeKey(tea.KeyPressMsg{Code: tea.KeyUp})
	g := asSearch(t, m)
	if g.histIdx != 0 || g.input.Value() != "newest" {
		t.Fatalf("after 1st Up: histIdx=%d value=%q, want 0/\"newest\"", g.histIdx, g.input.Value())
	}
	m, _ = g.routeKey(tea.KeyPressMsg{Code: tea.KeyUp})
	g = asSearch(t, m)
	if g.histIdx != 1 || g.input.Value() != "mid" {
		t.Fatalf("after 2nd Up: histIdx=%d value=%q, want 1/\"mid\"", g.histIdx, g.input.Value())
	}
	// Down walks back; stepping past the newest clears the input and resets.
	m, _ = g.routeKey(tea.KeyPressMsg{Code: tea.KeyDown})
	g = asSearch(t, m)
	if g.histIdx != 0 {
		t.Fatalf("after Down: histIdx=%d, want 0", g.histIdx)
	}
	m, _ = g.routeKey(tea.KeyPressMsg{Code: tea.KeyDown})
	g = asSearch(t, m)
	if g.histIdx != -1 || g.input.Value() != "" {
		t.Errorf("after Down past newest: histIdx=%d value=%q, want -1/\"\"", g.histIdx, g.input.Value())
	}
}

func TestSearchRecentModeNavigateAndExit(t *testing.T) {
	s := newSearchTab()
	s.recentMode = true
	s.recent.queries = []string{"a", "b", "c"}
	s.recent.cursor = 0
	s.height = 20

	m, _ := s.routeKey(tea.KeyPressMsg{Code: 'j', Text: "j"}) // Down
	g := asSearch(t, m)
	if g.recent.cursor != 1 {
		t.Fatalf("after Down: recent cursor = %d, want 1", g.recent.cursor)
	}
	m, _ = g.routeKey(tea.KeyPressMsg{Code: tea.KeyEscape}) // exit recent mode
	if asSearch(t, m).recentMode {
		t.Error("Esc in recent mode should exit back to input")
	}
}

func TestSearchOnRecentLoadedClampsCursors(t *testing.T) {
	s := newSearchTab()
	s.recent.cursor = 5
	s.histIdx = 4

	m, _ := s.onRecentLoaded(srchRecentLoadedMsg{queries: []string{"x", "y"}})
	g := asSearch(t, m)
	if g.recent.cursor != 1 {
		t.Errorf("cursor not clamped to last index: got %d, want 1", g.recent.cursor)
	}
	if g.histIdx != -1 {
		t.Errorf("histIdx past end not reset: got %d, want -1", g.histIdx)
	}
}
