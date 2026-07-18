//nolint:gosec // G115: int→int32 proto field conversions are bounded in practice.
package transport

import (
	"context"

	"connectrpc.com/connect"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
)

type channelHandler struct{ b api.Backend }

var _ backendv1connect.ChannelServiceHandler = (*channelHandler)(nil)

func (h *channelHandler) Search(ctx context.Context, req *connect.Request[v1.SearchRequest]) (*connect.Response[v1.SearchResponse], error) {
	chs, vids, err := h.b.Search(ctx, req.Msg.Query)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.SearchResponse{
		Channels: channelsToProto(chs),
		Videos:   videosToProto(vids),
	}), nil
}

func (h *channelHandler) SubscribedChannels(ctx context.Context, _ *connect.Request[v1.SubscribedChannelsRequest]) (*connect.Response[v1.SubscribedChannelsResponse], error) {
	chs, err := h.b.SubscribedChannels(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.SubscribedChannelsResponse{Channels: channelsToProto(chs)}), nil
}

func (h *channelHandler) GetSubscribedChannels(ctx context.Context, _ *connect.Request[v1.GetSubscribedChannelsRequest]) (*connect.Response[v1.GetSubscribedChannelsResponse], error) {
	chs, err := h.b.GetSubscribedChannels(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.GetSubscribedChannelsResponse{Channels: channelsToProto(chs)}), nil
}

func (h *channelHandler) ChannelVideos(ctx context.Context, req *connect.Request[v1.ChannelVideosRequest]) (*connect.Response[v1.ChannelVideosResponse], error) {
	vids, err := h.b.ChannelVideos(ctx, req.Msg.ChannelUrl, req.Msg.ChannelId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.ChannelVideosResponse{Videos: videosToProto(vids)}), nil
}

func (h *channelHandler) ChannelLatestN(ctx context.Context, req *connect.Request[v1.ChannelLatestNRequest]) (*connect.Response[v1.ChannelLatestNResponse], error) {
	vids, err := h.b.ChannelLatestN(ctx, req.Msg.ChannelUrl, req.Msg.ChannelId, int(req.Msg.N))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.ChannelLatestNResponse{Videos: videosToProto(vids)}), nil
}

func (h *channelHandler) Subscribe(ctx context.Context, req *connect.Request[v1.SubscribeRequest]) (*connect.Response[v1.SubscribeResponse], error) {
	if err := h.b.Subscribe(ctx, protoToChannel(req.Msg.Channel)); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.SubscribeResponse{}), nil
}

func (h *channelHandler) Unsubscribe(ctx context.Context, req *connect.Request[v1.UnsubscribeRequest]) (*connect.Response[v1.UnsubscribeResponse], error) {
	if err := h.b.Unsubscribe(ctx, protoToChannel(req.Msg.Channel)); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.UnsubscribeResponse{}), nil
}

func (h *channelHandler) AddSubscribedChannel(ctx context.Context, req *connect.Request[v1.AddSubscribedChannelRequest]) (*connect.Response[v1.AddSubscribedChannelResponse], error) {
	if err := h.b.AddSubscribedChannel(ctx, protoToChannel(req.Msg.Channel)); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.AddSubscribedChannelResponse{}), nil
}

func (h *channelHandler) SaveSubscribedChannels(ctx context.Context, req *connect.Request[v1.SaveSubscribedChannelsRequest]) (*connect.Response[v1.SaveSubscribedChannelsResponse], error) {
	if err := h.b.SaveSubscribedChannels(ctx, protoToChannels(req.Msg.Channels)); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.SaveSubscribedChannelsResponse{}), nil
}

func (h *channelHandler) RemoveSubscribedChannel(ctx context.Context, req *connect.Request[v1.RemoveSubscribedChannelRequest]) (*connect.Response[v1.RemoveSubscribedChannelResponse], error) {
	if err := h.b.RemoveSubscribedChannel(ctx, req.Msg.ChannelId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.RemoveSubscribedChannelResponse{}), nil
}

func (h *channelHandler) DeleteChannelVideos(ctx context.Context, req *connect.Request[v1.DeleteChannelVideosRequest]) (*connect.Response[v1.DeleteChannelVideosResponse], error) {
	if err := h.b.DeleteChannelVideos(ctx, req.Msg.ChannelId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.DeleteChannelVideosResponse{}), nil
}

func (h *channelHandler) SetChannelAlias(ctx context.Context, req *connect.Request[v1.SetChannelAliasRequest]) (*connect.Response[v1.SetChannelAliasResponse], error) {
	if err := h.b.SetChannelAlias(ctx, req.Msg.ChannelId, req.Msg.Alias); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.SetChannelAliasResponse{}), nil
}

func (h *channelHandler) SetChannelTags(ctx context.Context, req *connect.Request[v1.SetChannelTagsRequest]) (*connect.Response[v1.SetChannelTagsResponse], error) {
	if err := h.b.SetChannelTags(ctx, req.Msg.ChannelId, req.Msg.Tags); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.SetChannelTagsResponse{}), nil
}

func (h *channelHandler) GetChannelVideos(ctx context.Context, req *connect.Request[v1.GetChannelVideosRequest]) (*connect.Response[v1.GetChannelVideosResponse], error) {
	vids, err := h.b.GetChannelVideos(ctx, req.Msg.ChannelId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.GetChannelVideosResponse{Videos: videosToProto(vids)}), nil
}

func (h *channelHandler) GetAllChannelVideos(ctx context.Context, req *connect.Request[v1.GetAllChannelVideosRequest]) (*connect.Response[v1.GetAllChannelVideosResponse], error) {
	vids, err := h.b.GetAllChannelVideos(ctx, req.Msg.ChannelIds)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.GetAllChannelVideosResponse{Videos: videosToProto(vids)}), nil
}

func (h *channelHandler) GetChannelLatestAll(ctx context.Context, _ *connect.Request[v1.GetChannelLatestAllRequest]) (*connect.Response[v1.GetChannelLatestAllResponse], error) {
	m, err := h.b.GetChannelLatestAll(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pb := make(map[string]*v1.Video, len(m))
	for k, v := range m {
		pb[k] = videoToProto(v)
	}
	return connect.NewResponse(&v1.GetChannelLatestAllResponse{Latest: pb}), nil
}

func (h *channelHandler) SaveChannelVideos(ctx context.Context, req *connect.Request[v1.SaveChannelVideosRequest]) (*connect.Response[v1.SaveChannelVideosResponse], error) {
	if err := h.b.SaveChannelVideos(ctx, req.Msg.ChannelId, protoToVideos(req.Msg.Videos)); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.SaveChannelVideosResponse{}), nil
}

func (h *channelHandler) ChannelHideStats(ctx context.Context, req *connect.Request[v1.ChannelHideStatsRequest]) (*connect.Response[v1.ChannelHideStatsResponse], error) {
	hidden, played, err := h.b.ChannelHideStats(ctx, req.Msg.ChannelId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.ChannelHideStatsResponse{
		Hidden: int32(hidden),
		Played: int32(played),
	}), nil
}
