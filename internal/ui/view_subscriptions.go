package ui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// subscriptionsView owns the Subscriptions (all-channel feed) tab's private
// cursor/scroll/sort. The feed slice (subVideos) is shared with the Channels
// tab (tag grouping) and rewritten by unsubscribe/refresh, so it stays on the
// router (docs/TABVIEW_DESIGN.md, Finding 2).
type subscriptionsView struct {
	cursor int
	vs     int
	sort   int // one of vidSort*; default vidSortDate
}

// subActionIntent carries a non-navigation key: Unsubscribe (router-owned,
// mutates shared channel/feed state) or the shared video actions.
type subActionIntent struct{ msg tea.KeyMsg }

func (in subActionIntent) apply(m *Model) tea.Cmd {
	if key.Matches(in.msg, m.keys.Unsubscribe) {
		nm, cmd := m.unsubscribeCurrentChannel()
		*m = nm.(Model)
		return cmd
	}
	m.handleVideoAction(in.msg)
	return nil
}

func (v subscriptionsView) context() ContextID { return CtxVideoList }

func (v *subscriptionsView) jumpTo(idx, n, pageSize int) {
	v.cursor, v.vs = vsJump(idx, n, pageSize)
}

func (v *subscriptionsView) jumpToLast(n, pageSize int) {
	v.cursor, v.vs = vsJump(n-1, n, pageSize)
}

// reclamp keeps cursor/scroll valid after the feed length changes.
func (v *subscriptionsView) reclamp(n, pageSize int) {
	v.cursor, v.vs = vsMove(clamp(v.cursor, n), v.vs, n, 0, pageSize, false)
}

// update handles navigation directly and forwards every key to the router via
// subActionIntent (matching the original updateSubAll, which called
// handleVideoAction after its navigation/unsubscribe switch).
func (v *subscriptionsView) update(msg tea.KeyMsg, ctx viewCtx) viewIntent {
	keys := ctx.keys
	n := len(ctx.subVideos)
	switch {
	case key.Matches(msg, keys.Up):
		v.cursor, v.vs = vsMove(v.cursor, v.vs, n, -1, ctx.pageSize, ctx.circular)
	case key.Matches(msg, keys.Down):
		v.cursor, v.vs = vsMove(v.cursor, v.vs, n, +1, ctx.pageSize, ctx.circular)
	case key.Matches(msg, keys.PageUp):
		v.cursor, v.vs = vsPage(v.cursor, v.vs, n, -1, ctx.pageSize, ctx.circular)
	case key.Matches(msg, keys.PageDown):
		v.cursor, v.vs = vsPage(v.cursor, v.vs, n, +1, ctx.pageSize, ctx.circular)
	}
	return subActionIntent{msg: msg}
}

// render draws the Subscriptions feed via the shared video-list renderer.
// Note: like the original renderSubscriptions, this does NOT apply the local
// filter to the rendered list (currentVideo does, preserving prior behavior).
func (v subscriptionsView) render(ctx viewCtx, height int) string {
	return ctx.renderList(ctx.subVideos, v.cursor, v.vs, false, ctx.subLoading, height, "Subscriptions")
}
