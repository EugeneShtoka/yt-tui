//nolint:wrapcheck // Connect errors are already structured; pass through without re-wrapping.
package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/protoconv"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ── DownloadBackend ──────────────────────────────────────────────────────────

func (r *Remote) Enqueue(ctx context.Context, video domain.Video, audioOnly bool) error {
	_, err := r.dl.Enqueue(ctx, connect.NewRequest(&v1.EnqueueRequest{Video: protoconv.VideoToProto(video), AudioOnly: audioOnly}))
	return err
}

func (r *Remote) CancelDownload(ctx context.Context, videoID string) error {
	_, err := r.dl.CancelDownload(ctx, connect.NewRequest(&v1.CancelDownloadRequest{VideoId: videoID}))
	return err
}

func (r *Remote) DownloadItems(ctx context.Context) ([]DownloadItem, error) {
	resp, err := r.dl.DownloadItems(ctx, connect.NewRequest(&v1.DownloadItemsRequest{}))
	if err != nil {
		return nil, err
	}
	out := make([]DownloadItem, len(resp.Msg.GetItems()))
	for i, pb := range resp.Msg.GetItems() {
		out[i] = DownloadItem{
			VideoID:   pb.GetVideoId(),
			Title:     pb.GetTitle(),
			Channel:   pb.GetChannel(),
			Duration:  int(pb.GetDuration()),
			URL:       pb.GetUrl(),
			AudioOnly: pb.GetAudioOnly(),
			Status:    DownloadStatus(pb.GetStatus()),
			Progress:  pb.GetProgress(),
			Speed:     pb.GetSpeed(),
			ETA:       pb.GetEta(),
			FilePath:  pb.GetFilePath(),
		}
		// pb.Error is populated server-side (downloadItemToProto) but was
		// previously never read back here, so a failed download showed
		// "failed" with no reason in remote mode while InProc showed the
		// real cause. (H-9)
		if pb.GetError() != "" {
			out[i].Err = errors.New(pb.GetError())
		}
	}
	return out, nil
}

func (r *Remote) ClearDownloads(ctx context.Context) error {
	_, err := r.dl.ClearDownloads(ctx, connect.NewRequest(&v1.ClearDownloadsRequest{}))
	return err
}

// Events opens a server-streaming connection to the daemon and forwards events
// onto the returned channel. The channel is closed when ctx is canceled or the
// stream ends. A stream error (auth/TLS failure, connection drop) is logged
// rather than silently discarded — previously it was indistinguishable from a
// clean end-of-stream, so the Downloading tab's backoff-and-resubscribe loop
// ran forever with no diagnostic trace. (M-24)
func (r *Remote) Events(ctx context.Context) (<-chan Event, error) {
	stream, err := r.dlStream.Events(ctx, connect.NewRequest(&v1.EventsRequest{}))
	if err != nil {
		return nil, err
	}
	out := make(chan Event, 64)
	go func() {
		defer close(out)
		defer func() { _ = stream.Close() }()
		for stream.Receive() {
			ev := stream.Msg().GetEvent()
			if ev == nil {
				continue
			}
			select {
			case out <- Event{Kind: EventKind(ev.GetKind()), VideoID: ev.GetVideoId(), Detail: ev.GetDetail()}:
			case <-ctx.Done():
				return
			}
		}
		if err := stream.Err(); err != nil {
			debug.Log("Remote.Events: stream ended with error: %v", err)
		}
	}()
	return out, nil
}
