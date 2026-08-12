package tab

import (
	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

func (t Search) srchCmd(query string) tea.Cmd {
	return func() tea.Msg {
		ctx := t.ctx
		_ = t.backend.AddHistory(ctx, "", "search", query)
		channels, videos, err := t.backend.Search(ctx, query)
		if err != nil {
			return srchResultMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabSearch}, query: query, err: err}
		}
		return srchResultMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabSearch}, query: query, channels: channels, videos: videos}
	}
}

func (t Search) srchChannelVideosCmd(ch domain.Channel) tea.Cmd {
	return func() tea.Msg {
		videos, err := t.backend.ChannelVideos(t.ctx, ch.URL, ch.ID)
		if err != nil {
			return srchChannelVideosMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabSearch}, channelID: ch.ID, err: err}
		}
		return srchChannelVideosMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabSearch}, channelID: ch.ID, videos: videos}
	}
}

func (t Search) srchLoadRecentCmd() tea.Cmd {
	return func() tea.Msg {
		queries, err := t.backend.SearchQueries(t.ctx)
		if err != nil {
			return nil
		}
		return srchRecentLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabSearch}, queries: queries}
	}
}

func (t Search) srchDeleteRecentCmd(query string) tea.Cmd {
	return func() tea.Msg {
		_ = t.backend.DeleteSearchHistory(t.ctx, query)
		return nil
	}
}
