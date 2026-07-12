package ui

import (
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// searchView owns the Search tab's private cursor/scroll/sort for both result
// modes: the combined channel+video results list (cursor/vs) and the channel
// drill-down video list (vidCursor/vidVS). The result slices, the drill-down
// selection (searchChSel), the loading flags, and the text input all stay on
// the router — they are written by the async SearchResultMsg / FetchChannelVideos
// handlers and by the pre-dispatch input gate (docs/TABVIEW_DESIGN.md, Finding 2).
type searchView struct {
	cursor    int // combined channels+videos results cursor
	vs        int // results viewport scroll (video sub-list only)
	vidCursor int // channel drill-down video-list cursor
	vidVS     int // channel drill-down video-list scroll
	sort      int // one of vidSort*; default vidSortNone
}

// searchActionIntent carries a key for the router to act on: input refocus, the
// drill-down back/action subset, drilling into a channel (async fetch), or the
// shared video actions. The router branches on searchChSel to pick the mode,
// exactly as the original updateSearch did.
type searchActionIntent struct{ msg tea.KeyMsg }

func (in searchActionIntent) apply(m *Model) tea.Cmd {
	msg := in.msg
	keys := m.keys
	// Filter key refocuses the search input when results are shown.
	if key.Matches(msg, keys.Filter) {
		m.mode = modeSearchInput
		m.searchInput.Focus()
		return textinput.Blink
	}
	// ── Channel drill-down ────────────────────────────────────────────────────
	// Only the explicit action subset fires here (no AddList / drill-to-play),
	// matching the original updateSearch drill-down branch.
	if m.searchChSel != nil {
		switch {
		case key.Matches(msg, keys.Left), key.Matches(msg, keys.Escape):
			m.searchChSel = nil
			m.searchChVideos = nil
			m.localFilter = ""
			m.localFilterInput.SetValue("")
		case key.Matches(msg, keys.Play):
			if v, ok := m.currentVideo(); ok {
				m.playVideo(v)
			}
		case key.Matches(msg, keys.PlayAudio):
			if v, ok := m.currentVideo(); ok {
				m.playAudio(v)
			}
		case key.Matches(msg, keys.Download):
			if v, ok := m.currentVideo(); ok {
				m.startDownload(v, downloader.TypeVideo)
			}
		case key.Matches(msg, keys.DownloadAudio):
			if v, ok := m.currentVideo(); ok {
				m.startDownload(v, downloader.TypeAudio)
			}
		case key.Matches(msg, keys.CopyURL):
			m.copyCurrentURL()
		}
		return nil
	}
	// ── Channel + video results ───────────────────────────────────────────────
	// DrillDown/Right on a channel row fetches its videos; on a video row (or any
	// other key) the shared video-action handler runs.
	if key.Matches(msg, keys.DrillDown) || key.Matches(msg, keys.Right) {
		if m.search.cursor < len(m.searchChannels) {
			ch := m.searchChannels[m.search.cursor]
			m.searchChSel = &ch
			m.searchChVideos = nil
			m.search.vidCursor = 0
			m.searchChLoading = true
			return youtube.FetchChannelVideos(m.cfg, ch.URL, ch.ID, "search")
		}
		m.handleVideoAction(msg)
		return nil
	}
	m.handleVideoAction(msg)
	return nil
}

func (v searchView) context(ctx viewCtx) ContextID {
	if ctx.searchChSel != nil {
		return CtxVideoList // channel drill-down shows a video list
	}
	if v.cursor < len(ctx.searchChannels) {
		return CtxSearchChannel
	}
	return CtxSearchVideo
}

// updateVS keeps vs in sync after cursor moves. Channels are always fully
// visible; vs only applies to the video sub-list.
func (v *searchView) updateVS(nCh, nVid, pageSize int) {
	if v.cursor >= nCh && nVid > 0 {
		_, v.vs = vsMove(v.cursor-nCh, v.vs, nVid, 0, pageSize, false)
	} else {
		v.vs = 0
	}
}

func (v searchView) currentVideo(ctx viewCtx) (youtube.Video, bool) {
	if ctx.searchChSel != nil {
		if i := v.vidCursor; i >= 0 && i < len(ctx.searchChVideos) {
			return ctx.searchChVideos[i], true
		}
		return youtube.Video{}, false
	}
	nCh := len(ctx.searchChannels)
	idx := v.cursor - nCh
	if idx >= 0 && idx < len(ctx.searchVideos) {
		return ctx.searchVideos[idx], true
	}
	return youtube.Video{}, false
}

func (v *searchView) jumpTo(idx int, ctx viewCtx) {
	nCh, nVid := len(ctx.searchChannels), len(ctx.searchVideos)
	v.cursor = clamp(nCh+idx, nCh+nVid)
	v.updateVS(nCh, nVid, ctx.pageSize)
}

func (v *searchView) jumpToLast(ctx viewCtx) {
	nCh, nVid := len(ctx.searchChannels), len(ctx.searchVideos)
	v.cursor = nCh + clamp(nVid-1, nVid)
	v.updateVS(nCh, nVid, ctx.pageSize)
}

// update handles navigation directly for whichever result mode is active, and
// forwards every key to the router via searchActionIntent so the mode-specific
// actions still run (matching the original updateSearch structure).
func (v *searchView) update(msg tea.KeyMsg, ctx viewCtx) viewIntent {
	keys := ctx.keys
	if ctx.searchChSel != nil {
		n := len(ctx.searchChVideos)
		switch {
		case key.Matches(msg, keys.Up):
			v.vidCursor, v.vidVS = vsMove(v.vidCursor, v.vidVS, n, -1, ctx.pageSize, ctx.circular)
		case key.Matches(msg, keys.Down):
			v.vidCursor, v.vidVS = vsMove(v.vidCursor, v.vidVS, n, +1, ctx.pageSize, ctx.circular)
		case key.Matches(msg, keys.PageUp):
			v.vidCursor, v.vidVS = vsPage(v.vidCursor, v.vidVS, n, -1, ctx.pageSize, ctx.circular)
		case key.Matches(msg, keys.PageDown):
			v.vidCursor, v.vidVS = vsPage(v.vidCursor, v.vidVS, n, +1, ctx.pageSize, ctx.circular)
		}
		return searchActionIntent{msg: msg}
	}
	nCh := len(ctx.searchChannels)
	nVid := len(ctx.searchVideos)
	total := nCh + nVid
	switch {
	case key.Matches(msg, keys.Up):
		v.cursor = clamp(v.cursor-1, total)
		v.updateVS(nCh, nVid, ctx.pageSize)
	case key.Matches(msg, keys.Down):
		v.cursor = clamp(v.cursor+1, total)
		v.updateVS(nCh, nVid, ctx.pageSize)
	case key.Matches(msg, keys.PageUp):
		v.cursor = clamp(v.cursor-ctx.pageSize, total)
		v.updateVS(nCh, nVid, ctx.pageSize)
	case key.Matches(msg, keys.PageDown):
		v.cursor = clamp(v.cursor+ctx.pageSize, total)
		v.updateVS(nCh, nVid, ctx.pageSize)
	}
	return searchActionIntent{msg: msg}
}

// render draws the Search tab via the router's bespoke search renderer, captured
// per frame into viewCtx (the renderer is deeply Model-coupled: input view,
// spinner, channel rows, video rows).
func (v searchView) render(ctx viewCtx, height int) string {
	return ctx.renderSearch(height, v.cursor, v.vs, v.vidCursor, v.vidVS)
}
