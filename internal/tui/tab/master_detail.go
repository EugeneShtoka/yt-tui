package tab

import (
	"charm.land/lipgloss/v2"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
)

// masterDetail is the shared two-pane skeleton for tabs that drill from a master
// list into a per-item video list (Channels, Tags, Playlists — M-1). It owns only
// the low-variance mechanics those tabs duplicated: the pane flag, the two
// TableNavs, the dimensions, and the resize / pane-transition / detail-render /
// drill-back-routing helpers.
//
// It deliberately does NOT own the master data, the drill selector, video
// fetching/caching, or list-pane overlays (mode picker / inline edit / create
// wizard) — those vary too much per tab and stay on the tab, which embeds this.
//
// Embedding note: the mutating methods take a pointer receiver. The embedding
// tabs are value types (Update returns tea.Model), but the embedded masterDetail
// field is addressable inside a value-receiver tab method, so `t.drillIn()`
// mutates the local copy and persists via the returned tab — the same pattern as
// overlayStack and listCursor.
type masterDetail struct {
	pane          int // 0 = master list, 1 = detail (video) list
	listNav       videotable.TableNav
	vidNav        videotable.TableNav
	width, height int
}

// inDetail reports whether the detail (video) pane is active.
func (m masterDetail) inDetail() bool { return m.pane == 1 }

// resize records the content size and forwards it to both navs.
func (m *masterDetail) resize(w, h int) {
	m.width, m.height = w, h
	m.listNav.Resize(w, h)
	m.vidNav.Resize(w, h)
}

// setBorderDimmed fades both panes' frames while a focused info panel sits over
// the tab. Both are set (only the active pane renders, so the flag is read on
// whichever is drawn) — the per-tab WithListBorderDimmed methods delegate here.
func (m *masterDetail) setBorderDimmed(dimmed bool) {
	m.listNav.SetBorderDimmed(dimmed)
	m.vidNav.SetBorderDimmed(dimmed)
}

// drillIn enters the detail pane at the top row.
func (m *masterDetail) drillIn() {
	m.pane = 1
	m.vidNav.GotoRow(0)
}

// drillOut returns to the master list. It does not reset the detail cursor, so a
// tab that re-drills into the same item (from a cache) can preserve the reading
// position; tabs that always reset on entry use drillIn, which does.
func (m *masterDetail) drillOut() { m.pane = 0 }

// handleDetailBack runs the shared drill-back key in the detail pane: nav keys
// and the numBuf-guarded Left/Escape that pops back to the master list. Returns
// handled=true when it consumed the key (the caller should stop). detailN is the
// detail item count for the numBuf/nav guard.
func (m *masterDetail) handleDetailBack(msg tea.KeyPressMsg, keys keymap.KeyMap, detailN int) bool {
	handled, back := handleDrillBackKey(&m.vidNav, msg, keys, detailN)
	if handled && back {
		m.drillOut()
	}
	return handled
}

// renderDetail composes the detail pane every two-pane tab renders identically:
// the tab's header, the drill sub-header ("← <name>" + optional suffix), the
// given body (usually vidNav.View(), or a spinner while the detail is loading),
// and the numBuf line when active.
func (m masterDetail) renderDetail(header, drillName, subSuffix, body string) string {
	parts := []string{header, drillSubHeader(drillName, m.width, subSuffix), body}
	if s := m.vidNav.NumBufView(); s != "" {
		parts = append(parts, s)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// renderList joins a tab-composed list body (the tab owns its header + any
// picker/edit/create overlay) with the shared list numBuf line.
func (m masterDetail) renderList(parts []string) string {
	if s := m.listNav.NumBufView(); s != "" {
		parts = append(parts, s)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
