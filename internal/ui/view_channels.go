package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
)

// channelsView owns the Channels tab's private navigation state across its two
// modes (flat channel list / tags-grouped) and their panes. Everything the async
// fetch handlers or other tabs write — the channel/video slices, loading flags,
// the active channel ID, the latest-video map, and the alias/tag edit input —
// stays on the router (docs/TABVIEW_DESIGN.md, Finding 2).
//
// Pane model:
//   - flat mode:  pane 0 = channel list, pane 1 = selected channel's videos
//   - tags mode:  pane 0 = tag list,     pane 1 = aggregated videos for the tag
//
// cursor/vs are shared by the flat channel list and the tags-mode video pane
// (the original handler reused subChCursor/subChVS for both); the flat video
// pane has its own vidCursor/vidVS, and the tag list its own tagCursor/tagVS.
type channelsView struct {
	cursor int // flat channel list; also the tags-mode video pane
	vs     int

	vidCursor int // flat mode: selected channel's video pane
	vidVS     int

	tagCursor int // tags mode: tag list
	tagVS     int

	pane     int    // 0 or 1 (see pane model above)
	tagsMode bool   // true = grouped-by-tags view
	tagSel   string // selected tag name (tags mode, pane 1)

	sort    int // channel-list sort (one of subChSort*)
	vidSort int // flat video pane sort (one of vidSort*)
	tagSort int // tag video list sort (one of vidSort*)
}

// channelsActionIntent forwards a key for the router to act on. apply re-reads
// m.channels (mode/pane/cursor) to pick the right action, mirroring the original
// updateSubChannels dispatch, so the intent stays a lightweight key carrier.
type channelsActionIntent struct{ msg tea.KeyMsg }

func (in channelsActionIntent) apply(m *Model) tea.Cmd {
	msg := in.msg
	keys := m.keys

	// ── Tags-grouped view ──────────────────────────────────────────────────────
	// Pane 0 (tag list) navigation and drill-in are pure view state, handled in
	// update; only pane 1 (tag video list) forwards actions here.
	if m.channels.tagsMode {
		if m.channels.pane == 1 {
			if key.Matches(msg, keys.VideoInfo) {
				if v, ok := m.currentVideo(); ok && v.URL != "" {
					return m.openVideoDetail(v)
				}
				return nil
			}
			m.handleVideoAction(msg)
		}
		return nil
	}

	// ── Flat mode: channel list ─────────────────────────────────────────────────
	if m.channels.pane == 0 {
		sorted := m.sortedChannels()
		n := len(sorted)
		switch {
		case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
			if m.channels.cursor < n {
				return m.openChannelVideos(sorted[m.channels.cursor], false)
			}
		case key.Matches(msg, keys.RenameChannel):
			if m.channels.cursor < n {
				ch := sorted[m.channels.cursor]
				m.subChEditInput.SetValue(ch.Alias)
				m.subChEditInput.Placeholder = "alias (empty to clear)…"
				m.subChEditInput.Focus()
				m.mode = modeChannelEdit
				m.subChEditKind = 1
			}
		case key.Matches(msg, keys.TagChannel):
			if m.channels.cursor < n {
				ch := sorted[m.channels.cursor]
				m.subChEditInput.SetValue(strings.Join(ch.Tags, ", "))
				m.subChEditInput.Placeholder = "comma-separated tags…"
				m.subChEditInput.Focus()
				m.mode = modeChannelEdit
				m.subChEditKind = 2
			}
		case key.Matches(msg, keys.Unsubscribe):
			if m.channels.cursor < n {
				nm, cmd := m.unsubscribeCurrentChannel()
				*m = nm.(Model)
				return cmd
			}
		}
		return nil
	}

	// ── Flat mode: channel video pane ────────────────────────────────────────────
	if key.Matches(msg, keys.Unsubscribe) {
		nm, cmd := m.unsubscribeCurrentChannel()
		*m = nm.(Model)
		return cmd
	}
	m.handleVideoAction(msg)
	return nil
}

func (v channelsView) context(ctx viewCtx) ContextID {
	if v.tagsMode {
		if v.pane == 1 {
			return CtxVideoList
		}
		return CtxTagList
	}
	if v.pane == 0 {
		return CtxChannelList
	}
	return CtxVideoList
}

// update handles navigation and pure pane transitions directly, forwarding every
// action key to the router via channelsActionIntent (matching the original
// updateSubChannels structure: nav in the handler, effects on the Model).
func (v *channelsView) update(msg tea.KeyMsg, ctx viewCtx) viewIntent {
	keys := ctx.keys

	// ToggleMode flips between flat and tags-grouped views.
	if key.Matches(msg, keys.ToggleMode) {
		v.tagsMode = !v.tagsMode
		v.pane = 0
		v.tagCursor = 0
		v.tagVS = 0
		return nil
	}

	if v.tagsMode {
		return v.updateTags(msg, ctx)
	}

	// ── Flat mode: channel list ─────────────────────────────────────────────────
	if v.pane == 0 {
		n := len(ctx.chSorted)
		switch {
		case key.Matches(msg, keys.Up):
			v.cursor, v.vs = vsMove(v.cursor, v.vs, n, -1, ctx.pageSize, ctx.circular)
		case key.Matches(msg, keys.Down):
			v.cursor, v.vs = vsMove(v.cursor, v.vs, n, +1, ctx.pageSize, ctx.circular)
		default:
			return channelsActionIntent{msg: msg}
		}
		return nil
	}

	// ── Flat mode: channel video pane ────────────────────────────────────────────
	n := len(ctx.subChVideos)
	switch {
	case key.Matches(msg, keys.Left), key.Matches(msg, keys.Escape):
		v.pane = 0
	case key.Matches(msg, keys.Up):
		v.vidCursor, v.vidVS = vsMove(v.vidCursor, v.vidVS, n, -1, ctx.pageSize, ctx.circular)
	case key.Matches(msg, keys.Down):
		v.vidCursor, v.vidVS = vsMove(v.vidCursor, v.vidVS, n, +1, ctx.pageSize, ctx.circular)
	case key.Matches(msg, keys.PageUp):
		v.vidCursor, v.vidVS = vsPage(v.vidCursor, v.vidVS, n, -1, ctx.pageSize, ctx.circular)
	case key.Matches(msg, keys.PageDown):
		v.vidCursor, v.vidVS = vsPage(v.vidCursor, v.vidVS, n, +1, ctx.pageSize, ctx.circular)
	default:
		return channelsActionIntent{msg: msg}
	}
	return nil
}

// updateTags handles navigation for the tags-grouped view. Tag-list nav and the
// drill-into-tag transition are pure view state; the tag video pane forwards
// actions via the intent.
func (v *channelsView) updateTags(msg tea.KeyMsg, ctx viewCtx) viewIntent {
	keys := ctx.keys
	switch v.pane {
	case 0: // tag list
		n := len(ctx.chTagItems)
		switch {
		case key.Matches(msg, keys.Up):
			v.tagCursor, v.tagVS = vsMove(v.tagCursor, v.tagVS, n, -1, ctx.pageSize, ctx.circular)
		case key.Matches(msg, keys.Down):
			v.tagCursor, v.tagVS = vsMove(v.tagCursor, v.tagVS, n, +1, ctx.pageSize, ctx.circular)
		case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
			if v.tagCursor < n {
				v.tagSel = ctx.chTagItems[v.tagCursor]
				v.cursor = 0
				v.vs = 0
				v.pane = 1
			}
		}
		return nil

	case 1: // video list for selected tag (reuses cursor/vs)
		n := len(ctx.chTagVideos)
		switch {
		case key.Matches(msg, keys.Left), key.Matches(msg, keys.Escape):
			v.pane = 0
			v.cursor = 0
			v.vs = 0
		case key.Matches(msg, keys.Up):
			v.cursor, v.vs = vsMove(v.cursor, v.vs, n, -1, ctx.pageSize, ctx.circular)
		case key.Matches(msg, keys.Down):
			v.cursor, v.vs = vsMove(v.cursor, v.vs, n, +1, ctx.pageSize, ctx.circular)
		case key.Matches(msg, keys.PageUp):
			v.cursor, v.vs = vsPage(v.cursor, v.vs, n, -1, ctx.pageSize, ctx.circular)
		case key.Matches(msg, keys.PageDown):
			v.cursor, v.vs = vsPage(v.cursor, v.vs, n, +1, ctx.pageSize, ctx.circular)
		default:
			return channelsActionIntent{msg: msg}
		}
		return nil
	}
	return nil
}

// render draws the Channels tab via the router's bespoke renderer, captured per
// frame into viewCtx. The renderer reads the view's cursor/scroll off m.channels
// directly (it is deeply Model-coupled: spinner, channel rows, tag list, edit
// input), so nothing is threaded through as parameters here.
func (v channelsView) render(ctx viewCtx, height int) string {
	return ctx.renderChannels(height)
}

func (v channelsView) currentVideo(ctx viewCtx) (domain.Video, bool) {
	if v.tagsMode && v.pane == 1 {
		if i := v.cursor; i >= 0 && i < len(ctx.chTagVideos) {
			return ctx.chTagVideos[i], true
		}
	} else if !v.tagsMode && v.pane == 1 {
		if i := v.vidCursor; i >= 0 && i < len(ctx.subChVideos) {
			return ctx.subChVideos[i], true
		}
	}
	return domain.Video{}, false
}

func (v *channelsView) jumpTo(idx int, ctx viewCtx) {
	ps := ctx.pageSize
	if v.tagsMode {
		if v.pane == 1 {
			v.cursor, v.vs = vsJump(idx, len(ctx.chTagVideos), ps)
		} else {
			v.tagCursor, v.tagVS = vsJump(idx, len(ctx.chTagItems), ps)
		}
	} else if v.pane == 0 {
		v.cursor, v.vs = vsJump(idx, len(ctx.chSorted), ps)
	} else {
		v.vidCursor, v.vidVS = vsJump(idx, len(ctx.subChVideos), ps)
	}
}

func (v *channelsView) jumpToLast(ctx viewCtx) {
	ps := ctx.pageSize
	if v.tagsMode {
		if v.pane == 1 {
			n := len(ctx.chTagVideos)
			v.cursor, v.vs = vsJump(n-1, n, ps)
		} else {
			n := len(ctx.chTagItems)
			v.tagCursor, v.tagVS = vsJump(n-1, n, ps)
		}
	} else if v.pane == 0 {
		n := len(ctx.chSorted)
		v.cursor, v.vs = vsJump(n-1, n, ps)
	} else {
		n := len(ctx.subChVideos)
		v.vidCursor, v.vidVS = vsJump(n-1, n, ps)
	}
}

// unsubscribeCurrentChannel removes the focused channel from in-memory state
// and dispatches the backend unsubscribe command.
func (m Model) unsubscribeCurrentChannel() (tea.Model, tea.Cmd) {
	ch, ok := m.currentChannel()
	if !ok {
		return m, nil
	}
	m.subs.Remove(ch)
	m.subFeed.RemoveChannel(ch)
	m.subscriptions.reclamp(m.subFeed.Len(), m.pageSize())
	m.subChVideos = feed.RemoveChannelVideos(m.subChVideos, ch)
	m.channels.vidCursor, m.channels.vidVS = vsMove(clamp(m.channels.vidCursor, len(m.subChVideos)), m.channels.vidVS, len(m.subChVideos), 0, m.pageSize(), false)
	m.recFeed.StartRefresh()
	return m, tea.Batch(cmdUnsubscribe(m.backend, ch), cmdFetchRecommended(m.backend))
}
