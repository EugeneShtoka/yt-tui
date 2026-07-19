package videotable

import "github.com/EugeneShtoka/yt-tui/internal/domain"

// RenderContext carries per-video state used to decide styling during row construction.
type RenderContext struct {
	Positions   map[string]int64
	Watched     map[string]bool
	LocalStatus map[string]domain.VideoStatus
	Aliases     map[string]string // ChannelID → display alias
}
