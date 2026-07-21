package videotable

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// AuxData carries per-video playback state used by all feed-showing tabs.
type AuxData struct {
	Positions   map[string]int64
	Watched     map[string]bool
	LocalStatus map[string]domain.VideoStatus
	Aliases     map[string]string // channelID → display alias
}

// AuxDataMsg is the message type returned by LoadAuxDataCmd.
type AuxDataMsg = AuxData

// LoadAuxDataCmd fetches positions, watched, local video status, and channel aliases from the backend.
func LoadAuxDataCmd(backend api.Backend) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		positions, _ := backend.AllVideoPositions(ctx)
		watched, _ := backend.WatchedVideoIDs(ctx)
		localVids, _ := backend.LocalVideos(ctx)
		channels, _ := backend.GetSubscribedChannels(ctx)
		localStatus := make(map[string]domain.VideoStatus, len(localVids))
		for i := range localVids {
			localStatus[localVids[i].ID] = localVids[i].Status
		}
		aliases := make(map[string]string, len(channels))
		for _, ch := range channels {
			if ch.Alias != "" {
				aliases[ch.ID] = ch.Alias
			}
		}
		return AuxDataMsg{
			Positions:   positions,
			Watched:     watched,
			LocalStatus: localStatus,
			Aliases:     aliases,
		}
	}
}
