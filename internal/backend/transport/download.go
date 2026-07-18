package transport

import (
	"context"

	"connectrpc.com/connect"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
)

type downloadHandler struct{ b api.Backend }

var _ backendv1connect.DownloadServiceHandler = (*downloadHandler)(nil)

func (h *downloadHandler) Enqueue(ctx context.Context, req *connect.Request[v1.EnqueueRequest]) (*connect.Response[v1.EnqueueResponse], error) {
	if err := h.b.Enqueue(ctx, protoToVideo(req.Msg.Video), req.Msg.AudioOnly); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.EnqueueResponse{}), nil
}

func (h *downloadHandler) CancelDownload(ctx context.Context, req *connect.Request[v1.CancelDownloadRequest]) (*connect.Response[v1.CancelDownloadResponse], error) {
	if err := h.b.CancelDownload(ctx, req.Msg.VideoId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.CancelDownloadResponse{}), nil
}

func (h *downloadHandler) DownloadItems(ctx context.Context, _ *connect.Request[v1.DownloadItemsRequest]) (*connect.Response[v1.DownloadItemsResponse], error) {
	items, err := h.b.DownloadItems(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pb := make([]*v1.DownloadItem, len(items))
	for i := range items {
		pb[i] = downloadItemToProto(items[i])
	}
	return connect.NewResponse(&v1.DownloadItemsResponse{Items: pb}), nil
}

func (h *downloadHandler) ClearDownloads(ctx context.Context, _ *connect.Request[v1.ClearDownloadsRequest]) (*connect.Response[v1.ClearDownloadsResponse], error) {
	paths, err := h.b.ClearDownloads(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.ClearDownloadsResponse{FilePaths: paths}), nil
}

func (h *downloadHandler) Events(ctx context.Context, _ *connect.Request[v1.EventsRequest], stream *connect.ServerStream[v1.EventsResponse]) error {
	ch, err := h.b.Events(ctx)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(&v1.EventsResponse{
				Event: &v1.Event{
					Kind:    string(ev.Kind),
					VideoId: ev.VideoID,
					Detail:  ev.Detail,
				},
			}); err != nil {
				return err
			}
		}
	}
}
