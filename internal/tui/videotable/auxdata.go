package videotable

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tea "charm.land/bubbletea/v2"
)

// AuxData carries per-video playback state used by all feed-showing tabs.
type AuxData struct {
	Positions   map[string]int64
	Watched     map[string]bool
	LocalStatus map[string]domain.VideoStatus
}

// AuxDataMsg is the message type returned by LoadAuxDataCmd.
type AuxDataMsg = AuxData

// LoadAuxDataCmd fetches positions, watched, and local video status from the backend.
func LoadAuxDataCmd(backend api.Backend) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		positions, _ := backend.AllVideoPositions(ctx)
		watched, _ := backend.WatchedVideoIDs(ctx)
		localVids, _ := backend.LocalVideos(ctx)
		localStatus := make(map[string]domain.VideoStatus, len(localVids))
		for i := range localVids {
			localStatus[localVids[i].ID] = localVids[i].Status
		}
		return AuxDataMsg{
			Positions:   positions,
			Watched:     watched,
			LocalStatus: localStatus,
		}
	}
}

