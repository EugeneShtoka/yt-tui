package ui

// overlayKind identifies a modal overlay layered above the normal tab content.
//
// Unlike inputMode (exactly one text-input mode at a time), overlays form a
// small stack: the links and chapters overlays open on top of the video-detail
// panel and are popped to reveal it again, so two can be active at once. Key
// dispatch goes to the frontmost overlay (topOverlay); the render layer composes
// them by membership (hasOverlay), drawing each active overlay in its own layer.
//
// The stack is shallow in practice — at most video-detail with one of
// links/chapters on top, or a single standalone overlay.
type overlayKind int

const (
	overlayNone        overlayKind = iota // sentinel: empty stack
	overlayAdd                            // add-to-playlist overlay
	overlayVideoDetail                    // video info panel (stack base for links/chapters)
	overlayLinks                          // description-links overlay
	overlayChapters                       // chapters overlay
)

// pushOverlay puts o on top of the overlay stack. It is idempotent at the top —
// re-opening the overlay that is already frontmost is a no-op — preserving the
// old `boolField = true` semantics against accidental double-opens.
func (m *Model) pushOverlay(o overlayKind) {
	if m.topOverlay() == o {
		return
	}
	m.overlays = append(m.overlays, o)
}

// popOverlay removes the frontmost overlay, revealing whatever was beneath.
func (m *Model) popOverlay() {
	if n := len(m.overlays); n > 0 {
		m.overlays = m.overlays[:n-1]
	}
}

// topOverlay reports the frontmost overlay, or overlayNone when none is open.
func (m Model) topOverlay() overlayKind {
	if n := len(m.overlays); n > 0 {
		return m.overlays[n-1]
	}
	return overlayNone
}

// hasOverlay reports whether o is anywhere in the stack.
func (m Model) hasOverlay(o overlayKind) bool {
	for _, x := range m.overlays {
		if x == o {
			return true
		}
	}
	return false
}

// closeOverlaysFrom removes o and every overlay stacked above it — used to tear
// down the video-detail panel together with any links/chapters opened over it.
// It is a no-op if o is not in the stack.
func (m *Model) closeOverlaysFrom(o overlayKind) {
	for i := len(m.overlays) - 1; i >= 0; i-- {
		if m.overlays[i] == o {
			m.overlays = m.overlays[:i]
			return
		}
	}
}
