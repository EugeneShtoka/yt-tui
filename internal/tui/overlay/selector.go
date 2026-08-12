package overlay

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
)

// scrollPageSize is the number of lines a PageUp/PageDown moves a text pager.
const scrollPageSize = 10

// scrollKey applies a vertical-text-scroll key (Down/Up/PageDown/PageUp/
// GotoPrefix/GotoBottom) to a scroll offset, returning the new offset and whether
// the key was a scroll key. maxOffset is the largest valid offset (the caller's
// own last-line/last-page position); every result is clamped to [0, maxOffset]
// so no key can park the offset past the viewport — a dead zone that made j/k
// look frozen after G. GotoPrefix (gg) jumps to the top, GotoBottom (G) to the
// bottom. Single implementation behind the VideoDetail description and transcript
// pagers (M-5).
func scrollKey(offset, maxOffset int, msg tea.KeyPressMsg, keys keymap.KeyMap) (int, bool) {
	if maxOffset < 0 {
		maxOffset = 0
	}
	switch {
	case key.Matches(msg, keys.Down):
		offset++
	case key.Matches(msg, keys.Up):
		offset--
	case key.Matches(msg, keys.PageDown):
		offset += scrollPageSize
	case key.Matches(msg, keys.PageUp):
		offset -= scrollPageSize
	case key.Matches(msg, keys.GotoPrefix):
		offset = 0
	case key.Matches(msg, keys.GotoBottom):
		offset = maxOffset
	default:
		return offset, false
	}
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	return offset, true
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
