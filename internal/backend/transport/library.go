package transport

import (
	"context"

	"connectrpc.com/connect"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/protoconv"
)

type libraryHandler struct{ b api.LibraryBackend }

var _ backendv1connect.LibraryServiceHandler = (*libraryHandler)(nil)

func (h *libraryHandler) LocalVideos(ctx context.Context, _ *connect.Request[v1.LocalVideosRequest]) (*connect.Response[v1.LocalVideosResponse], error) {
	videos, err := h.b.LocalVideos(ctx)
	if err != nil {
		return nil, rpcErr(err)
	}
	pb := make([]*v1.LocalVideo, len(videos))
	for i := range videos {
		pb[i] = protoconv.LocalVideoToProto(videos[i])
	}
	return connect.NewResponse(&v1.LocalVideosResponse{Videos: pb}), nil
}

func (h *libraryHandler) AddLocalVideo(ctx context.Context, req *connect.Request[v1.AddLocalVideoRequest]) (*connect.Response[v1.AddLocalVideoResponse], error) {
	if err := h.b.AddLocalVideo(ctx, protoconv.ProtoToLocalVideo(req.Msg.Video)); err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.AddLocalVideoResponse{}), nil
}

func (h *libraryHandler) DeleteLocalVideo(ctx context.Context, req *connect.Request[v1.DeleteLocalVideoRequest]) (*connect.Response[v1.DeleteLocalVideoResponse], error) {
	if err := h.b.DeleteLocalVideo(ctx, req.Msg.Id); err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.DeleteLocalVideoResponse{}), nil
}

func (h *libraryHandler) DeleteAllLocalFiles(ctx context.Context, _ *connect.Request[v1.DeleteAllLocalFilesRequest]) (*connect.Response[v1.DeleteAllLocalFilesResponse], error) {
	deleted, err := h.b.DeleteAllLocalFiles(ctx)
	if err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.DeleteAllLocalFilesResponse{Deleted: int32(deleted)}), nil //nolint:gosec // G115: local-video count is bounded in practice
}

func (h *libraryHandler) HasLocalVideo(ctx context.Context, req *connect.Request[v1.HasLocalVideoRequest]) (*connect.Response[v1.HasLocalVideoResponse], error) {
	lv, found, err := h.b.HasLocalVideo(ctx, req.Msg.VideoId)
	if err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.HasLocalVideoResponse{
		Video: protoconv.LocalVideoToProto(lv),
		Found: found,
	}), nil
}
