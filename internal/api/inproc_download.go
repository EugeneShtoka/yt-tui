//nolint:wrapcheck // pass-through adapter; errors from backend/db/yt are already contextual
package api

import (
	"context"
	"fmt"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
)

// ── DownloadBackend ──────────────────────────────────────────────────────────

func (p *InProc) Enqueue(ctx context.Context, video domain.Video, audioOnly bool) error {
	dlType := downloader.TypeVideo
	if audioOnly {
		dlType = downloader.TypeAudio
	}
	p.dl.Start(video, dlType)
	return nil
}

func (p *InProc) CancelDownload(ctx context.Context, videoID string) error {
	p.dl.Remove(videoID)
	return nil
}

func (p *InProc) DownloadItems(ctx context.Context) ([]DownloadItem, error) {
	raw := p.dl.Items()
	out := make([]DownloadItem, len(raw))
	for i := range raw {
		it := &raw[i]
		var ds DownloadStatus
		switch it.Status {
		case downloader.StatusPending:
			ds = DownloadPending
		case downloader.StatusActive:
			ds = DownloadActive
		case downloader.StatusComplete:
			ds = DownloadComplete
		default:
			ds = DownloadFailed
		}
		out[i] = DownloadItem{
			VideoID:   it.Video.ID,
			Title:     it.Video.Title,
			Channel:   it.Video.Channel,
			Duration:  it.Video.Duration,
			URL:       it.Video.URL,
			AudioOnly: it.Type == downloader.TypeAudio,
			Status:    ds,
			Progress:  it.Progress,
			Speed:     it.Speed,
			ETA:       it.ETA,
			FilePath:  it.FilePath,
			Err:       it.Err,
		}
	}
	return out, nil
}

// ClearDownloads dismisses the whole queue (cancel-if-active), the bulk form of
// CancelDownload. It touches neither files, the DB, nor history — the Local tab
// (DeleteAllLocalFiles) owns file/record deletion.
func (p *InProc) ClearDownloads(ctx context.Context) error {
	p.dl.Clear()
	return nil
}

// Events bridges the downloader's event channel into api.Event values. It
// registers a dedicated subscription with the downloader's broadcaster (C-2),
// so each call — one per streaming RPC client in daemon mode, or one per
// TUI resubscribe — gets its own copy of every event instead of racing other
// subscribers for events off one shared channel. The returned channel stays
// open until ctx is canceled.
func (p *InProc) Events(ctx context.Context) (<-chan Event, error) {
	in := p.dl.Subscribe(ctx)
	out := make(chan Event, 64)
	go func() {
		defer close(out)
		for ev := range in {
			var kind EventKind
			var detail string
			switch ev.Kind {
			case downloader.EventProgress:
				kind = EventDownloadProgress
				detail = fmt.Sprintf("%.0f%% %s ETA %s", ev.Progress, ev.Speed, ev.ETA)
			case downloader.EventComplete:
				kind = EventDownloadDone
				detail = ev.FilePath
			case downloader.EventError:
				kind = EventDownloadError
				if ev.Err != nil {
					detail = ev.Err.Error()
				}
			default:
				continue
			}
			select {
			case out <- Event{Kind: kind, VideoID: ev.VideoID, Detail: detail}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}
