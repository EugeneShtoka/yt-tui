package transport

import (
	"github.com/EugeneShtoka/yt-tui/internal/api"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
)

// downloadItemToProto lives here (not in protoconv) because api.DownloadItem is
// an API-layer type; importing it into protoconv would create an import cycle
// (api → protoconv → api). It is only needed server-side.
func downloadItemToProto(it api.DownloadItem) *v1.DownloadItem {
	errStr := ""
	if it.Err != nil {
		errStr = it.Err.Error()
	}
	return &v1.DownloadItem{
		VideoId:   it.VideoID,
		Title:     it.Title,
		Channel:   it.Channel,
		Duration:  int32(it.Duration), //nolint:gosec // G115: durations are bounded
		Url:       it.URL,
		AudioOnly: it.AudioOnly,
		Status:    string(it.Status),
		Progress:  it.Progress,
		Speed:     it.Speed,
		Eta:       it.ETA,
		FilePath:  it.FilePath,
		Error:     errStr,
	}
}
