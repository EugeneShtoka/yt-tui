package transport

import (
	"context"

	"connectrpc.com/connect"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
)

type feedHandler struct{ b api.Backend }

var _ backendv1connect.FeedServiceHandler = (*feedHandler)(nil)

func (h *feedHandler) Recommended(ctx context.Context, _ *connect.Request[v1.RecommendedRequest]) (*connect.Response[v1.RecommendedResponse], error) {
	videos, err := h.b.Recommended(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.RecommendedResponse{Videos: videosToProto(videos)}), nil
}

func (h *feedHandler) HideVideo(ctx context.Context, req *connect.Request[v1.HideVideoRequest]) (*connect.Response[v1.HideVideoResponse], error) {
	if err := h.b.HideRecVideo(ctx, req.Msg.VideoId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.HideVideoResponse{}), nil
}

func (h *feedHandler) HiddenVideoIDs(ctx context.Context, _ *connect.Request[v1.HiddenVideoIDsRequest]) (*connect.Response[v1.HiddenVideoIDsResponse], error) {
	ids, err := h.b.HiddenRecVideoIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.HiddenVideoIDsResponse{Ids: ids}), nil
}

func (h *feedHandler) WatchedVideoIDs(ctx context.Context, _ *connect.Request[v1.WatchedVideoIDsRequest]) (*connect.Response[v1.WatchedVideoIDsResponse], error) {
	ids, err := h.b.WatchedVideoIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.WatchedVideoIDsResponse{Ids: ids}), nil
}

func (h *feedHandler) GetFeedCache(ctx context.Context, req *connect.Request[v1.GetFeedCacheRequest]) (*connect.Response[v1.GetFeedCacheResponse], error) {
	videos, err := h.b.GetFeedCache(ctx, req.Msg.Feed)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.GetFeedCacheResponse{Videos: videosToProto(videos)}), nil
}

func (h *feedHandler) SaveFeedCache(ctx context.Context, req *connect.Request[v1.SaveFeedCacheRequest]) (*connect.Response[v1.SaveFeedCacheResponse], error) {
	if err := h.b.SaveFeedCache(ctx, req.Msg.Feed, protoToVideos(req.Msg.Videos)); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.SaveFeedCacheResponse{}), nil
}

func (h *feedHandler) PurgeFeedCache(ctx context.Context, req *connect.Request[v1.PurgeFeedCacheRequest]) (*connect.Response[v1.PurgeFeedCacheResponse], error) {
	if err := h.b.PurgeFeedCacheMissingChannelID(ctx, req.Msg.Feed); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.PurgeFeedCacheResponse{}), nil
}

func (h *feedHandler) ClearRecommended(ctx context.Context, _ *connect.Request[v1.ClearRecommendedRequest]) (*connect.Response[v1.ClearRecommendedResponse], error) {
	if err := h.b.ClearRecommended(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.ClearRecommendedResponse{}), nil
}
