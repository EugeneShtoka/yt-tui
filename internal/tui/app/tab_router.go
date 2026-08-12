package app

import tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"

// tabRouter owns the panel set and the active-tab pointer, plus the pure
// navigation logic Root used to inline: resolving a TabID / panel name /
// tab-chord key to an index, and computing the wrapped cycle target. It holds no
// UI (tab bar, status bar) and no overlay state — Root keeps that orchestration
// and asks the router only for the "which tab" decisions. This mirrors how
// overlayStack and backendActions were split out of Root.
//
// Tabs are value types stored behind the tuipkg.Tab interface. The mutators take
// a pointer receiver and are called on Root's addressable router field, so the
// change persists via the returned Root — the same value-semantics pattern as
// overlayStack.
type tabRouter struct {
	tabs        []tuipkg.Tab
	activeIdx   int
	chordActive bool
	// chordKeys maps a tab-chord second key to a panel index (config named
	// hotkeys + the positional 1..9 fallback); panelIdx maps a panel name to its
	// index for name-based navigation (the :tab command).
	chordKeys map[string]int
	panelIdx  map[string]int
}

// active returns the current tab. Trivial slice ops (range, len, tabs[i]=…) are
// done directly on tr.tabs by Root, matching the overlayStack precedent; the
// methods here carry only the non-trivial navigation logic.
func (tr tabRouter) active() tuipkg.Tab { return tr.tabs[tr.activeIdx] }

// valid reports whether i is a selectable panel index.
func (tr tabRouter) valid(i int) bool { return i >= 0 && i < len(tr.tabs) }

// indexOfID returns the first panel index whose tab has the given ID.
func (tr tabRouter) indexOfID(id tuipkg.TabID) (int, bool) {
	for i, t := range tr.tabs {
		if t.ID() == id {
			return i, true
		}
	}
	return 0, false
}

// panelIndex resolves a panel name (the :tab command) to its index.
func (tr tabRouter) panelIndex(name string) (int, bool) {
	i, ok := tr.panelIdx[name]
	return i, ok
}

// wrapIndex returns the active index advanced by dir, wrapping at both ends.
func (tr tabRouter) wrapIndex(dir int) int {
	n := len(tr.tabs)
	return ((tr.activeIdx+dir)%n + n) % n
}

func (tr *tabRouter) setActive(i int) { tr.activeIdx = i }

// chordArmed reports whether a tab-chord is waiting for its second key.
func (tr tabRouter) chordArmed() bool { return tr.chordActive }

// armChord starts a tab-chord (waiting for the second key).
func (tr *tabRouter) armChord() { tr.chordActive = true }

// resolveChord consumes an armed tab-chord: it disarms and, when key names a
// bound panel, returns its index. Callers gate on chordArmed first so an armed
// chord always consumes the key even when it doesn't match.
func (tr *tabRouter) resolveChord(key string) (int, bool) {
	tr.chordActive = false
	i, ok := tr.chordKeys[key]
	return i, ok
}
