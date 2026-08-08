package app

import (
	tea "charm.land/bubbletea/v2"
	ovpkg "github.com/EugeneShtoka/yt-tui/internal/tui/overlay"
)

// overlayStack is the modal/panel overlay stack, extracted from Root (H-2) so the
// pure stack mechanics — push/pop, top-of-stack queries, delivering messages to
// the top or to the info panel, culling on tab switch — live in one cohesive type
// instead of scattered across Root's method set. It is a named slice so every
// slice operation (index, len, range, append, reslice) keeps working directly on
// it; the methods below add the behavior Root used to hand-roll.
//
// Root still owns overlay *orchestration* (building overlays from config/backend,
// resize coordination); this type owns only the stack itself.
type overlayStack []ovpkg.Overlay

// topPanel returns the top overlay when it is the info side panel — the
// stack-position + type check the overlay-open handlers share.
func (s overlayStack) topPanel() (ovpkg.VideoDetail, bool) {
	n := len(s)
	if n == 0 {
		return ovpkg.VideoDetail{}, false
	}
	vd, ok := s[n-1].(ovpkg.VideoDetail)
	if !ok || !vd.IsPanel() {
		return ovpkg.VideoDetail{}, false
	}
	return vd, true
}

// topIsHelp reports whether the help overlay is on top (pressing Help again while
// it is open is a no-op).
func (s overlayStack) topIsHelp() bool {
	n := len(s)
	if n == 0 {
		return false
	}
	_, ok := s[n-1].(ovpkg.Help)
	return ok
}

// updateTop delivers msg to the top overlay and replaces it with the result.
// Callers guard len>0. The element write lands on the shared backing array, so
// a value receiver is enough (no header change).
func (s overlayStack) updateTop(msg tea.Msg) tea.Cmd {
	n := len(s)
	updated, cmd := s[n-1].Update(msg)
	s[n-1] = updated.(ovpkg.Overlay)
	return cmd
}

// updatePanel delivers msg to the open info side panel wherever it sits in the
// stack (it may be beneath a stacked modal), so the panel can track the active
// tab even when it isn't the top overlay. No-op when no panel is open.
func (s overlayStack) updatePanel(msg tea.Msg) tea.Cmd {
	for i, o := range s {
		if vd, ok := o.(ovpkg.VideoDetail); ok && vd.IsPanel() {
			updated, cmd := o.Update(msg)
			s[i] = updated.(ovpkg.Overlay)
			return cmd
		}
	}
	return nil
}

// hasPanel reports whether the info side panel is currently open.
func (s overlayStack) hasPanel() bool {
	for _, o := range s {
		if vd, ok := o.(ovpkg.VideoDetail); ok && vd.IsPanel() {
			return true
		}
	}
	return false
}

// widthReduction is the first non-zero width an overlay reserves (the info panel
// reserves width; centered modals reserve none), so the layout can shrink the tab
// content beneath it.
func (s overlayStack) widthReduction() int {
	for _, o := range s {
		if red := o.WidthReduction(); red > 0 {
			return red
		}
	}
	return 0
}

// pop removes the top overlay and reports whether it reserved width, so the
// caller can decide to re-layout. No-op returning false when the stack is empty.
func (s *overlayStack) pop() (hadWidthReduction bool) {
	n := len(*s)
	if n == 0 {
		return false
	}
	hadWidthReduction = (*s)[n-1].WidthReduction() > 0
	*s = (*s)[:n-1]
	return hadWidthReduction
}

// cullNonPanel drops every non-panel overlay — used on a tab switch, where every
// stacked modal belongs to the tab/video just left — keeping any open info side
// panel (it follows the active tab). Reports whether a panel remained.
func (s *overlayStack) cullNonPanel() (hadPanel bool) {
	hadPanel = s.hasPanel()
	kept := (*s)[:0:0]
	for _, o := range *s {
		if vd, ok := o.(ovpkg.VideoDetail); ok && vd.IsPanel() {
			kept = append(kept, o)
		}
	}
	*s = kept
	return hadPanel
}
