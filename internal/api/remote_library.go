//nolint:wrapcheck // Connect errors are already structured; pass through without re-wrapping.
package api

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/protoconv"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ── LibraryBackend ───────────────────────────────────────────────────────────

func (r *Remote) LocalVideos(ctx context.Context) ([]domain.LocalVideo, error) {
	resp, err := r.lib.LocalVideos(ctx, connect.NewRequest(&v1.LocalVideosRequest{}))
	if err != nil {
		return nil, err
	}
	out := make([]domain.LocalVideo, len(resp.Msg.Videos))
	for i, pb := range resp.Msg.Videos {
		out[i] = protoconv.ProtoToLocalVideo(pb)
	}
	return out, nil
}

// HasLocalVideo also satisfies VideoBackend (see remote_video.go) — the two
// role interfaces share this lookup by design.
func (r *Remote) HasLocalVideo(ctx context.Context, videoID string) (domain.LocalVideo, bool, error) {
	resp, err := r.lib.HasLocalVideo(ctx, connect.NewRequest(&v1.HasLocalVideoRequest{VideoId: videoID}))
	if err != nil {
		// See VideoPosition: a transport failure must not look like "not
		// downloaded", or a daemon hiccup re-streams a file that's actually
		// local. (H-8)
		return domain.LocalVideo{}, false, err
	}
	return protoconv.ProtoToLocalVideo(resp.Msg.Video), resp.Msg.Found, nil
}

func (r *Remote) AddLocalVideo(ctx context.Context, v domain.LocalVideo) error {
	_, err := r.lib.AddLocalVideo(ctx, connect.NewRequest(&v1.AddLocalVideoRequest{Video: protoconv.LocalVideoToProto(v)}))
	return err
}

func (r *Remote) DeleteLocalVideo(ctx context.Context, id string) error {
	_, err := r.lib.DeleteLocalVideo(ctx, connect.NewRequest(&v1.DeleteLocalVideoRequest{Id: id}))
	return err
}

func (r *Remote) DeleteAllLocalFiles(ctx context.Context) (int, error) {
	resp, err := r.lib.DeleteAllLocalFiles(ctx, connect.NewRequest(&v1.DeleteAllLocalFilesRequest{}))
	if err != nil {
		return 0, err
	}
	return int(resp.Msg.Deleted), nil
}
