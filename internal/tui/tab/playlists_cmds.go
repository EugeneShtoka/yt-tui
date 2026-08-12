package tab

import (
	tea "charm.land/bubbletea/v2"

	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

func statusMsg(text string) tea.Cmd {
	return func() tea.Msg { return tuipkg.StatusMsg{Text: text} }
}

func errMsg(text string) tea.Cmd {
	return func() tea.Msg { return tuipkg.StatusMsg{Text: text, IsErr: true} }
}

func (t Playlists) localLoadCmd() tea.Cmd {
	return func() tea.Msg {
		pls, err := t.backend.LocalPlaylists(t.ctx)
		if err != nil {
			return tuipkg.StatusMsg{Text: "local playlists: " + err.Error(), IsErr: true}
		}
		return plLocalLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabPlaylists}, playlists: pls}
	}
}

func (t Playlists) ytCachedLoadCmd() tea.Cmd {
	return func() tea.Msg {
		pls, err := t.backend.GetYTPlaylists(t.ctx)
		if err != nil || len(pls) == 0 {
			return nil
		}
		return plYTLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabPlaylists}, playlists: pls, fromCache: true}
	}
}

func (t Playlists) ytRefreshCmd(manual bool) tea.Cmd {
	tgt := tuipkg.TabTarget{Tab: tuipkg.TabPlaylists}
	return func() tea.Msg {
		ctx := t.ctx
		if !manual {
			// Auto refresh: throttled backend sync (serves cache when fresh, fetches
			// live only when stale, and persists internally). On a live-fetch error
			// it returns the cache, so fall back silently unless there's nothing.
			pls, err := t.backend.SyncYTPlaylists(ctx)
			if err != nil && pls == nil {
				return plYTLoadedMsg{TabTarget: tgt, err: err, background: true}
			}
			return plYTLoadedMsg{TabTarget: tgt, playlists: pls, background: true}
		}
		// Manual refresh (R): force a live fetch and persist, bypassing the throttle.
		pls, err := t.backend.YTPlaylists(ctx)
		if err != nil {
			return plYTLoadedMsg{TabTarget: tgt, err: err, background: false}
		}
		_ = t.backend.SaveYTPlaylists(ctx, pls)
		return plYTLoadedMsg{TabTarget: tgt, playlists: pls, background: false}
	}
}

func (t Playlists) ytVideosDrilldownCmd(playlistID string) tea.Cmd {
	return func() tea.Msg {
		ctx := t.ctx
		cached, err := t.backend.GetYTPlaylistVideos(ctx, playlistID)
		if err == nil && len(cached) > 0 {
			return plVideosCachedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabPlaylists}, playlistID: playlistID, videos: cached}
		}
		vids, err := t.backend.YTPlaylistVideos(ctx, playlistID)
		if err != nil {
			return plVideosLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabPlaylists}, playlistID: playlistID, err: err}
		}
		_ = t.backend.SaveYTPlaylistVideos(ctx, playlistID, vids)
		return plVideosLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabPlaylists}, playlistID: playlistID, videos: vids}
	}
}

func (t Playlists) ytVideosRefreshCmd(playlistID string) tea.Cmd {
	return func() tea.Msg {
		ctx := t.ctx
		vids, err := t.backend.YTPlaylistVideos(ctx, playlistID)
		if err != nil {
			return plVideosLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabPlaylists}, playlistID: playlistID, err: err, background: true}
		}
		_ = t.backend.SaveYTPlaylistVideos(ctx, playlistID, vids)
		return plVideosLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabPlaylists}, playlistID: playlistID, videos: vids, background: true}
	}
}

func (t Playlists) localVideosCmd(playlistID string) tea.Cmd {
	return func() tea.Msg {
		vids, err := t.backend.LocalPlaylistVideos(t.ctx, playlistID)
		return plVideosLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabPlaylists}, playlistID: playlistID, videos: vids, err: err}
	}
}

func (t Playlists) createYTPlaylistCmd(name string) tea.Cmd {
	return func() tea.Msg {
		id, err := t.backend.CreateYTPlaylist(t.ctx, name)
		return plYTCreatedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabPlaylists}, name: name, id: id, err: err}
	}
}

func (t Playlists) createLocalPlaylistCmd(name string) tea.Cmd {
	return func() tea.Msg {
		id, err := t.backend.CreatePlaylist(t.ctx, name)
		return plLocalCreatedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabPlaylists}, name: name, id: id, err: err}
	}
}
