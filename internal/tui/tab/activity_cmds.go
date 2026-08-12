package tab

import (
	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

func (t Activity) actNavigateCmd(e domain.ActivityEntry) tea.Cmd {
	switch e.Type {
	case "subscribe":
		return func() tea.Msg {
			return tuipkg.NavigateToChannelMsg{ChannelID: e.ChannelID, ChannelName: e.ChannelName}
		}
	case "create_playlist", "add_to_playlist":
		return func() tea.Msg {
			return tuipkg.NavigateToPlaylistMsg{
				PlaylistID:      e.PlaylistID,
				PlaylistLocalID: e.PlaylistLocalID,
				PlaylistName:    e.PlaylistName,
			}
		}
	}
	return nil
}

func (t Activity) actLoadCmd() tea.Cmd {
	return func() tea.Msg {
		entries, err := t.backend.ActivityLog(t.ctx, 200)
		if err != nil {
			return tuipkg.StatusMsg{Text: "activity: " + err.Error(), IsErr: true}
		}
		return actLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabActivity}, entries: entries}
	}
}
