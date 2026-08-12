package tab

import (
	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
)

func (t Local) localDeleteCmd(lv domain.LocalVideo) tea.Cmd {
	return func() tea.Msg {
		ctx := t.ctx
		if err := t.backend.DeleteLocalVideo(ctx, lv.ID); err != nil {
			return tuipkg.StatusMsg{Text: "delete: " + err.Error(), IsErr: true}
		}
		_ = t.backend.AddHistory(ctx, lv.ID, "delete", "") // history is best-effort
		videos, err := t.backend.LocalVideos(ctx)
		if err != nil {
			return tuipkg.StatusMsg{Text: "local: " + err.Error(), IsErr: true}
		}
		return localLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabLocal}, videos: videos, status: "Deleted: " + render.Truncate(lv.Title, 50)}
	}
}

func (t Local) localLoadCmd(status string) tea.Cmd {
	return func() tea.Msg {
		videos, err := t.backend.LocalVideos(t.ctx)
		if err != nil {
			return tuipkg.StatusMsg{Text: "local: " + err.Error(), IsErr: true}
		}
		return localLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabLocal}, videos: videos, status: status}
	}
}
