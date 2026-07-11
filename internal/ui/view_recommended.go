package ui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// recommendedView owns the Recommended tab's private cursor/scroll/sort. The
// feed slice (recVideos), the hide set, and the fetch-lifecycle flags are
// written across tabs and by async fetches, so they stay on the router
// (docs/TABVIEW_DESIGN.md, Finding 2).
type recommendedView struct {
	cursor int
	vs     int
	sort   int // one of vidSort*; default vidSortViews
}

// recActionIntent carries a non-navigation key for the router to act on: the
// tab-specific hide keys plus the shared video actions (play/download/copy/add).
type recActionIntent struct{ msg tea.KeyMsg }

func (in recActionIntent) apply(m *Model) tea.Cmd {
	switch {
	case key.Matches(in.msg, m.keys.HideVideo):
		if v, ok := m.currentVideo(); ok {
			_ = m.db.HideRecVideo(v.ID)
			m.recHidden[v.ID] = true
			m.recVideos = removeVideoByID(m.recVideos, v.ID)
			m.recommended.reclamp(len(m.recVideos), m.pageSize())
			m.setStatus("Hidden: "+truncate(v.Title, 50), false)
			m.checkVideoHideAutoBlacklist(v.ChannelID, v.Channel)
		}
	case key.Matches(in.msg, m.keys.HideChannel):
		if v, ok := m.currentVideo(); ok {
			m.hideChannel(v.ChannelID, v.Channel)
		}
	}
	m.handleVideoAction(in.msg)
	return nil
}

func (v recommendedView) context(ctx viewCtx) ContextID { return CtxVideoList }

func (v *recommendedView) jumpTo(idx, n, pageSize int) {
	v.cursor, v.vs = vsJump(idx, n, pageSize)
}

func (v *recommendedView) jumpToLast(n, pageSize int) {
	v.cursor, v.vs = vsJump(n-1, n, pageSize)
}

// reclamp keeps cursor/scroll valid after the feed length changes.
func (v *recommendedView) reclamp(n, pageSize int) {
	v.cursor, v.vs = vsMove(clamp(v.cursor, n), v.vs, n, 0, pageSize, false)
}

// update handles navigation directly; every key is also forwarded to the router
// via recActionIntent so the shared video-action handler still runs (matching
// the original updateRecommended, which called handleVideoAction after its
// navigation/hide switch).
func (v *recommendedView) update(msg tea.KeyMsg, ctx viewCtx) viewIntent {
	keys := ctx.keys
	n := len(ctx.recVideos)
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
	return recActionIntent{msg: msg}
}

// render draws the Recommended feed via the shared video-list renderer.
func (v recommendedView) render(ctx viewCtx, height int) string {
	videos, cur := ctx.recVideos, v.cursor
	if ctx.localFilter != "" {
		videos = filterText(ctx.recVideos, ctx.localFilter)
		cur = ctx.localFilterCursor
	}
	return ctx.renderList(videos, cur, v.vs, ctx.recLoading, ctx.recRefreshing, height, "Recommended for you")
}
