package ui

import (
	"fmt"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// playlistsView owns the Playlists tab's private navigation state across its two
// panes: the playlist list (cursor/vs) and the selected playlist's video list
// (vidCursor/vidVS). Everything the async fetch handlers write — the local/YT
// playlist slices, the per-playlist video cache, the loading flags — plus the
// create/type/add-overlay text inputs (all pre-dispatch gated) stay on the router
// (docs/TABVIEW_DESIGN.md, Finding 2).
//
// Pane model:
//   - pane 0 = playlist list (YT playlists then local playlists)
//   - pane 1 = selected playlist's video list
type playlistsView struct {
	cursor int // playlist list
	vs     int

	vidCursor int // selected playlist's video pane
	vidVS     int

	pane int // 0 = playlist list, 1 = video list
	sort int // video pane sort (one of vidSort*)
}

func (v playlistsView) currentVideo(ctx viewCtx) (domain.Video, bool) {
	if v.pane == 1 {
		if i := v.vidCursor; i >= 0 && i < len(ctx.plVideos) {
			return ctx.plVideos[i], true
		}
	}
	return domain.Video{}, false
}

func (v *playlistsView) jumpTo(idx int, ctx viewCtx) {
	if v.pane == 0 {
		v.cursor, v.vs = vsJump(idx, ctx.plCount, ctx.pageSize)
	} else {
		v.vidCursor, v.vidVS = vsJump(idx, len(ctx.plVideos), ctx.pageSize)
	}
}

func (v *playlistsView) jumpToLast(ctx viewCtx) {
	if v.pane == 0 {
		v.cursor, v.vs = vsJump(ctx.plCount-1, ctx.plCount, ctx.pageSize)
	} else {
		n := len(ctx.plVideos)
		v.vidCursor, v.vidVS = vsJump(n-1, n, ctx.pageSize)
	}
}

func (v playlistsView) context(ctx viewCtx) ContextID {
	if v.pane == 0 {
		return CtxPlaylistList
	}
	return CtxVideoList
}

// update handles navigation and pure pane transitions directly, forwarding every
// action key to the router via playlistsActionIntent (matching the original
// updatePlaylists structure: nav in the handler, effects on the Model).
func (v *playlistsView) update(msg tea.KeyMsg, ctx viewCtx) viewIntent {
	keys := ctx.keys

	// ── Playlist list pane ──────────────────────────────────────────────────────
	if v.pane == 0 {
		n := ctx.plCount
		switch {
		case key.Matches(msg, keys.Up):
			v.cursor, v.vs = vsMove(v.cursor, v.vs, n, -1, ctx.pageSize, ctx.circular)
		case key.Matches(msg, keys.Down):
			v.cursor, v.vs = vsMove(v.cursor, v.vs, n, +1, ctx.pageSize, ctx.circular)
		default:
			return playlistsActionIntent{msg: msg}
		}
		return nil
	}

	// ── Video pane ──────────────────────────────────────────────────────────────
	// Guard against a stale cursor after the playlist list shrank.
	if v.cursor >= ctx.plCount {
		v.pane = 0
		return nil
	}
	n := len(ctx.plVideos)
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
		return playlistsActionIntent{msg: msg}
	}
	return nil
}

// render draws the Playlists tab via the router's bespoke renderer, captured per
// frame into viewCtx. The renderer reads the view's cursor/scroll off m.playlist
// directly (it is deeply Model-coupled: spinner, playlist rows, create/type input),
// so nothing is threaded through as parameters here.
func (v playlistsView) render(ctx viewCtx, height int) string {
	return ctx.renderPlaylists(height)
}

// playlistsActionIntent forwards a key for the router to act on. apply re-reads
// m.playlist (pane/cursor) to pick the right action, mirroring the original
// updatePlaylists dispatch, so the intent stays a lightweight key carrier.
type playlistsActionIntent struct{ msg tea.KeyMsg }

func (in playlistsActionIntent) apply(m *Model) tea.Cmd {
	msg := in.msg
	keys := m.keys

	// ── Playlist list pane ──────────────────────────────────────────────────────
	if m.playlist.pane == 0 {
		n := m.playlistCount()
		switch {
		case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
			if m.playlist.cursor < n {
				cmd := m.loadCurrentPlaylistVideos()
				m.playlist.pane = 1
				m.playlist.vidCursor = 0
				return cmd
			}
		case key.Matches(msg, keys.NewList):
			if m.ytClient != nil {
				m.mode = modeCreateType
				m.createTypeSel = 0
			} else {
				m.createModeYT = false
				m.createInput.SetValue("")
				m.createInput.Placeholder = "Playlist name…"
				m.createInput.Focus()
				m.mode = modeCreatePlaylist
				return textinput.Blink
			}
		case key.Matches(msg, keys.Delete):
			plKey := m.selectedPlaylistKey()
			if plKey == ytWatchLaterID {
				return nil // Watch Later cannot be deleted
			}
			idx := m.playlist.cursor
			var delCmd tea.Cmd
			if m.ytPlLoaded && m.ytClient != nil && idx < len(m.ytPlaylists) {
				pl := m.ytPlaylists[idx]
				delCmd = deletePlaylistCmd(m.ytClient, pl.ID)
				delete(m.playlistVidCache, pl.ID)
				m.ytPlaylists = append(m.ytPlaylists[:idx], m.ytPlaylists[idx+1:]...)
			} else {
				localIdx := idx
				if m.ytPlLoaded {
					localIdx -= len(m.ytPlaylists)
				}
				if localIdx >= 0 && localIdx < len(m.playlists) {
					pl := m.playlists[localIdx]
					_ = m.db.DeletePlaylist(pl.ID)
					delete(m.playlistVidCache, fmt.Sprintf("local:%d", pl.ID))
					playlists, _ := m.db.Playlists()
					m.playlists = playlists
				}
			}
			m.playlist.cursor, m.playlist.vs = vsMove(clamp(m.playlist.cursor, m.playlistCount()), m.playlist.vs, m.playlistCount(), 0, m.pageSize(), false)
			return delCmd
		}
		return nil
	}

	// ── Video pane ──────────────────────────────────────────────────────────────
	if m.playlist.cursor >= m.playlistCount() {
		m.playlist.pane = 0
		return nil
	}
	plKey := m.selectedPlaylistKey()
	vids := m.playlistVidCache[plKey]
	n := len(vids)

	switch {
	case key.Matches(msg, keys.DrillDown):
		if v, ok := m.currentVideo(); ok {
			m.downloadAndPlay(v)
		}
	case key.Matches(msg, keys.Delete):
		if m.playlist.vidCursor < n {
			vid := vids[m.playlist.vidCursor]
			var cmd tea.Cmd
			if m.selectedPlaylistIsYT() {
				cmd = youtube.RemoveYTPlaylistVideo(m.ytClient, plKey, vid.ID)
			} else {
				localID := parseLocalPlaylistID(plKey)
				_ = m.db.RemoveFromPlaylist(localID, vid.ID)
			}
			// Optimistic removal from cache.
			updated := make([]domain.Video, 0, len(vids)-1)
			for _, v := range vids {
				if v.ID != vid.ID {
					updated = append(updated, v)
				}
			}
			m.playlistVidCache[plKey] = updated
			m.playlist.vidCursor, m.playlist.vidVS = vsMove(clamp(m.playlist.vidCursor, len(updated)), m.playlist.vidVS, len(updated), 0, m.pageSize(), false)
			return cmd
		}
	case key.Matches(msg, keys.Download):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeVideo)
		}
	case key.Matches(msg, keys.DownloadAudio):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeAudio)
		}
	}
	return nil
}
