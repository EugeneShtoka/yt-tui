package downloader

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// completionSink is the slice of persistence a finished download needs: record
// the video row, register the downloaded file, and log a history event. *db.DB
// satisfies it. The Downloader depends on this port rather than *db.DB so the
// queue/orchestrator is testable without a real database and the download's
// storage side-effects are confined to one named seam (H-4).
type completionSink interface {
	UpsertVideo(ctx context.Context, id, title, channel, channelID string, duration int, viewCount int64, uploadDate, url string) error
	AddLocalVideo(ctx context.Context, v domain.LocalVideo) error
	AddHistory(ctx context.Context, videoID, eventType, details string) error
}
