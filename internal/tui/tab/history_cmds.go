package tab

import (
	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

func (t History) loadCmd() tea.Cmd {
	return func() tea.Msg {
		entries, err := t.backend.HistoryVideos(t.ctx, 200)
		if err != nil {
			return histLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabHistory}, err: err}
		}
		return histLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabHistory}, entries: entries}
	}
}

func (t History) histLoadDetailCmd(videoID string) tea.Cmd {
	return func() tea.Msg {
		entries, err := t.backend.VideoHistory(t.ctx, videoID)
		if err != nil {
			return tuipkg.StatusMsg{Text: "history detail: " + err.Error(), IsErr: true}
		}
		return histDetailLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabHistory}, videoID: videoID, entries: entries}
	}
}

func (t History) histDeleteCmd(e domain.HistoryEntry) tea.Cmd {
	return func() tea.Msg {
		err := t.backend.DeleteVideoCompletely(t.ctx, e.VideoID)
		return histDeletedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabHistory}, entry: e, err: err}
	}
}
