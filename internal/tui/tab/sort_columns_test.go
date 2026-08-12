package tab

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
)

// keyPress fabricates a key press matching the first key of a binding.
func keyPress(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Text: text}
}

// A sort mode is enabled only when a column representing it is visible.
func TestEnabledSortModes(t *testing.T) {
	// A rich video column set: title, channel, duration, views, date visible;
	// no size / subs / tags columns.
	keys := []string{videotable.KeyNum, videotable.KeyTitle, videotable.KeyChannel,
		videotable.KeyDuration, videotable.KeyCount, videotable.KeyDate}
	got := enabledSortModes(keys)
	for _, m := range []int{feed.SortName, feed.SortChannel, feed.SortDuration, feed.SortViews, feed.SortDate} {
		if !got[m] {
			t.Errorf("mode %d should be enabled", m)
		}
	}
	for _, m := range []int{feed.SortSize, feed.SortSubscribers, feed.SortTags} {
		if got[m] {
			t.Errorf("mode %d should be disabled (no column visible)", m)
		}
	}
}

// handleChord must ignore a sort whose column is hidden: the mode stays put and
// apply is not invoked, even though the chord is consumed.
func TestHandleChordGatedByVisibleColumns(t *testing.T) {
	sk := testKeys().Sort
	// Only the title column is visible → only SortName is selectable.
	s := newSortState(feed.SortName, []string{videotable.KeyTitle})

	applied := false
	s.chordActive = true
	// Attempt to sort by size (s.Size); size column is hidden → gated off.
	consumed := s.handleChord(keyPress(sk.Size.Keys()[0]), sk, func(int) { applied = true })
	if !consumed {
		t.Fatal("armed chord must consume the key")
	}
	if applied {
		t.Error("sort by a hidden column must not apply")
	}
	if s.mode != feed.SortName {
		t.Errorf("mode changed to %d despite gated sort", s.mode)
	}
}

// A sort whose column IS visible still applies normally.
func TestHandleChordAllowsVisibleColumn(t *testing.T) {
	sk := testKeys().Sort
	s := newSortState(feed.SortName, []string{videotable.KeyTitle, videotable.KeyDate})

	applied := false
	s.chordActive = true
	consumed := s.handleChord(keyPress(sk.Date.Keys()[0]), sk, func(int) { applied = true })
	if !consumed || !applied {
		t.Fatalf("visible-column sort should apply: consumed=%v applied=%v", consumed, applied)
	}
	if s.mode != feed.SortDate {
		t.Errorf("mode = %d, want SortDate", s.mode)
	}
}

// An empty key set (no columns known) enables every sort — the safe default so
// a tab that opts out of column-aware sorting behaves exactly as before.
func TestEnabledSortModesEmptyEnablesAll(t *testing.T) {
	got := enabledSortModes(nil)
	for _, m := range []int{feed.SortName, feed.SortDate, feed.SortViews, feed.SortSize} {
		if !got[m] {
			t.Errorf("mode %d should be enabled when no columns are declared", m)
		}
	}
}
