package tab

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
)

// sortColumnKeys maps each sort mode to the column keys that expose that sort
// dimension. A sort is selectable only while at least one of its columns is
// visible (Phase 22 per-panel columns): hide the column and its sort chord
// becomes a no-op — "no column, no sorting by it". Several modes list more than
// one key because conceptually-equal columns differ per tab (e.g. views is
// KeyCount on a video list but KeyChViews on the channel list; name lives in the
// flex title column, channel-name in the narrow channel-list column).
var sortColumnKeys = map[int][]string{
	feed.SortDate:        {videotable.KeyDate},
	feed.SortViews:       {videotable.KeyCount, videotable.KeyChViews},
	feed.SortName:        {videotable.KeyTitle, videotable.KeyTagLabel, videotable.KeyPlName},
	feed.SortChannel:     {videotable.KeyChannel, videotable.KeyChName},
	feed.SortDuration:    {videotable.KeyDuration},
	feed.SortSubscribers: {videotable.KeyChSubs},
	feed.SortTags:        {videotable.KeyChTags},
	feed.SortSize:        {videotable.KeySize},
}

// enabledSortModes returns the set of sort modes selectable given the visible
// column keys. A mode is enabled when at least one of its representing columns
// (see sortColumnKeys) is present. An empty key set enables every mode — the
// safe fallback for a tab that declares no columns to the sort layer, so its
// behavior is unchanged.
func enabledSortModes(visibleKeys []string) map[int]bool {
	enabled := make(map[int]bool, len(sortColumnKeys))
	if len(visibleKeys) == 0 {
		for mode := range sortColumnKeys {
			enabled[mode] = true
		}
		return enabled
	}
	visible := make(map[string]bool, len(visibleKeys))
	for _, k := range visibleKeys {
		visible[k] = true
	}
	for mode, cols := range sortColumnKeys {
		for _, k := range cols {
			if visible[k] {
				enabled[mode] = true
				break
			}
		}
	}
	return enabled
}

// resolveSort maps a sort-chord key press to a feed sort mode. One resolver
// serves every tab (video lists and the Channels tab alike): each tab only
// binds the sort keys it advertises, and a mode whose projection field is zero
// for an entity sorts as a stable no-op for it. ok is false when the key
// matched no sort binding, in which case the caller keeps the current mode
// (re-sorting with the same mode is a harmless no-op).
func resolveSort(msg tea.KeyPressMsg, sk keymap.SortKeyMap) (mode int, ok bool) {
	switch {
	case key.Matches(msg, sk.Date):
		return feed.SortDate, true
	case key.Matches(msg, sk.Views):
		return feed.SortViews, true
	case key.Matches(msg, sk.Name):
		return feed.SortName, true
	case key.Matches(msg, sk.Channel):
		return feed.SortChannel, true
	case key.Matches(msg, sk.Duration):
		return feed.SortDuration, true
	case key.Matches(msg, sk.Subscribers):
		return feed.SortSubscribers, true
	case key.Matches(msg, sk.Tags):
		return feed.SortTags, true
	case key.Matches(msg, sk.Size):
		return feed.SortSize, true
	}
	return 0, false
}

// sortModeOr resolves a panel's configured default-sort name to a sort mode,
// falling back to the tab's built-in default when the name is empty or
// unrecognized. Each list tab passes its own fallback so an unset panel `sort`
// preserves that tab's historical ordering.
func sortModeOr(name string, fallback int) int {
	if m, ok := feed.ParseSortName(name); ok {
		return m
	}
	return fallback
}

// sortState is the per-tab sort-chord state: the current sort mode, whether the
// chord is armed (the user pressed SortChord and the next key selects a mode),
// and the set of modes selectable given the panel's visible columns. Tabs embed
// it as a named field and drive it through handleChord, which owns the whole
// chord lifecycle so no tab repeats it.
type sortState struct {
	mode        int
	chordActive bool
	// enabled gates which modes the chord may switch to: a sort whose column is
	// hidden (per-panel column config) is off. nil means every mode is enabled
	// (a tab that opts out of column-aware sorting).
	enabled map[int]bool
}

// newSortState builds a sortState with an initial mode and the sort modes made
// selectable by the given visible column keys. The initial mode is honored even
// if its column is hidden (it is the panel's configured default ordering); only
// interactive chord switching is gated.
func newSortState(mode int, visibleKeys []string) sortState {
	return sortState{mode: mode, enabled: enabledSortModes(visibleKeys)}
}

// handleChord processes one key while a sort chord is armed. It disarms the
// chord, and if the key selected a sort mode, updates the mode and runs apply
// (which re-sorts the tab's own collection and rebuilds its rows). It reports
// whether the key was consumed — true whenever the chord was armed, so the tab
// swallows the keystroke even if it wasn't a sort key.
//
// apply runs only when a mode matched: for the video tabs this is visually
// identical to their old code (which re-sorted to the already-applied mode on a
// non-match — a no-op), and it matches the Channels tab's old changed-flag.
func (s *sortState) handleChord(msg tea.KeyPressMsg, sk keymap.SortKeyMap, apply func(mode int)) bool {
	if !s.chordActive {
		return false
	}
	s.chordActive = false
	if m, ok := resolveSort(msg, sk); ok && (s.enabled == nil || s.enabled[m]) {
		s.mode = m
		apply(m)
	}
	return true
}
