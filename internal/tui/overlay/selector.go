package overlay

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
)

// scrollPageSize is the number of lines a PageUp/PageDown moves a text pager.
const scrollPageSize = 10

// scrollKey applies a vertical-text-scroll key (Down/Up/PageDown/PageUp/
// GotoBottom) to a scroll offset, returning the new offset and whether the key
// was a scroll key. Down is unbounded (the renderer clamps); Up/PageUp floor at
// 0; GotoBottom jumps to bottom. It is the single implementation behind the
// VideoDetail description and transcript pagers, which carried byte-identical
// copies (M-5).
func scrollKey(offset, bottom int, msg tea.KeyPressMsg, keys keymap.KeyMap) (int, bool) {
	switch {
	case key.Matches(msg, keys.Down):
		return offset + 1, true
	case key.Matches(msg, keys.Up):
		if offset > 0 {
			offset--
		}
		return offset, true
	case key.Matches(msg, keys.PageDown):
		return offset + scrollPageSize, true
	case key.Matches(msg, keys.PageUp):
		offset -= scrollPageSize
		if offset < 0 {
			offset = 0
		}
		return offset, true
	case key.Matches(msg, keys.GotoBottom):
		return bottom, true
	}
	return offset, false
}

// moveVertical applies Up/Down (and, when gotoBottom is set, G/GotoBottom)
// selection movement over an n-item list with optional wrap-around. It is the
// single implementation behind the overlay selectors (AddToPlaylist and the
// VideoDetail link/chapter lists), which previously carried byte-identical
// copies. Returns the new index and whether the key was consumed.
func moveVertical(sel, n int, msg tea.KeyPressMsg, keys keymap.KeyMap, circular, gotoBottom bool) (int, bool) {
	switch {
	case key.Matches(msg, keys.Up):
		if sel > 0 {
			return sel - 1, true
		}
		if circular && n > 0 {
			return n - 1, true
		}
		return sel, true
	case key.Matches(msg, keys.Down):
		if sel < n-1 {
			return sel + 1, true
		}
		if circular {
			return 0, true
		}
		return sel, true
	case gotoBottom && key.Matches(msg, keys.GotoBottom):
		if n > 0 {
			return n - 1, true
		}
		return sel, true
	}
	return sel, false
}
