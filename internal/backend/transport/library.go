package transport

import (
	"context"

	"connectrpc.com/connect"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
)

type libraryHandler struct{ b api.Backend }

var _ backendv1connect.LibraryServiceHandler = (*libraryHandler)(nil)

func (h *libraryHandler) LocalVideos(ctx context.Context, _ *connect.Request[v1.LocalVideosRequest]) (*connect.Response[v1.LocalVideosResponse], error) {
	videos, err := h.b.LocalVideos(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pb := make([]*v1.LocalVideo, len(videos))
	for i := range videos {
		pb[i] = localVideoToProto(videos[i])
	}
	return connect.NewResponse(&v1.LocalVideosResponse{Videos: pb}), nil
}

func (h *libraryHandler) AddLocalVideo(ctx context.Context, req *connect.Request[v1.AddLocalVideoRequest]) (*connect.Response[v1.AddLocalVideoResponse], error) {
	if err := h.b.AddLocalVideo(ctx, protoToLocalVideo(req.Msg.Video)); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.AddLocalVideoResponse{}), nil
}

func (h *libraryHandler) DeleteLocalVideo(ctx context.Context, req *connect.Request[v1.DeleteLocalVideoRequest]) (*connect.Response[v1.DeleteLocalVideoResponse], error) {
	if err := h.b.DeleteLocalVideo(ctx, req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.DeleteLocalVideoResponse{}), nil
}

func (h *libraryHandler) HasLocalVideo(ctx context.Context, req *connect.Request[v1.HasLocalVideoRequest]) (*connect.Response[v1.HasLocalVideoResponse], error) {
	lv, found := h.b.HasLocalVideo(ctx, req.Msg.VideoId)
	return connect.NewResponse(&v1.HasLocalVideoResponse{
		Video: localVideoToProto(lv),
		Found: found,
	}), nil
}
