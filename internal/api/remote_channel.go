//nolint:wrapcheck,gosec // Connect errors are already structured; pass through without re-wrapping. gosec G115: proto int32 fields are bounded in practice (durations, counts).
package api

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/protoconv"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ── ChannelBackend ───────────────────────────────────────────────────────────

func (r *Remote) SubscribedChannels(ctx context.Context) ([]domain.Channel, error) {
	resp, err := r.ch.SubscribedChannels(ctx, connect.NewRequest(&v1.SubscribedChannelsRequest{}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToChannels(resp.Msg.Channels), nil
}

func (r *Remote) ChannelVideos(ctx context.Context, channelURL, channelID string) ([]domain.Video, error) {
	resp, err := r.ch.ChannelVideos(ctx, connect.NewRequest(&v1.ChannelVideosRequest{ChannelUrl: channelURL, ChannelId: channelID}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) ChannelLatestN(ctx context.Context, channelURL, channelID string, n int) ([]domain.Video, error) {
	resp, err := r.ch.ChannelLatestN(ctx, connect.NewRequest(&v1.ChannelLatestNRequest{ChannelUrl: channelURL, ChannelId: channelID, N: int32(n)}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) Search(ctx context.Context, query string) ([]domain.Channel, []domain.Video, error) {
	resp, err := r.ch.Search(ctx, connect.NewRequest(&v1.SearchRequest{Query: query}))
	if err != nil {
		return nil, nil, err
	}
	return protoconv.ProtoToChannels(resp.Msg.Channels), protoconv.ProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) GetChannelVideos(ctx context.Context, channelID string) ([]domain.Video, error) {
	resp, err := r.ch.GetChannelVideos(ctx, connect.NewRequest(&v1.GetChannelVideosRequest{ChannelId: channelID}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) GetAllChannelVideos(ctx context.Context, channelIDs []string) ([]domain.Video, error) {
	resp, err := r.ch.GetAllChannelVideos(ctx, connect.NewRequest(&v1.GetAllChannelVideosRequest{ChannelIds: channelIDs}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) GetChannelLatestAll(ctx context.Context) (map[string]domain.Video, error) {
	resp, err := r.ch.GetChannelLatestAll(ctx, connect.NewRequest(&v1.GetChannelLatestAllRequest{}))
	if err != nil {
		return nil, err
	}
	out := make(map[string]domain.Video, len(resp.Msg.Latest))
	for k, pb := range resp.Msg.Latest {
		out[k] = protoconv.ProtoToVideo(pb)
	}
	return out, nil
}

func (r *Remote) ChannelHideStats(ctx context.Context, channelID string) (int, int, error) {
	resp, err := r.ch.ChannelHideStats(ctx, connect.NewRequest(&v1.ChannelHideStatsRequest{ChannelId: channelID}))
	if err != nil {
		return 0, 0, err
	}
	return int(resp.Msg.Hidden), int(resp.Msg.Played), nil
}

func (r *Remote) GetSubscribedChannels(ctx context.Context) ([]domain.Channel, error) {
	resp, err := r.ch.GetSubscribedChannels(ctx, connect.NewRequest(&v1.GetSubscribedChannelsRequest{}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToChannels(resp.Msg.Channels), nil
}

func (r *Remote) AllChannels(ctx context.Context) ([]domain.Channel, error) {
	resp, err := r.ch.AllChannels(ctx, connect.NewRequest(&v1.AllChannelsRequest{}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToChannels(resp.Msg.Channels), nil
}

func (r *Remote) BlockedChannels(ctx context.Context) ([]domain.Channel, error) {
	resp, err := r.ch.BlockedChannels(ctx, connect.NewRequest(&v1.BlockedChannelsRequest{}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToChannels(resp.Msg.Channels), nil
}

func (r *Remote) BlockChannel(ctx context.Context, ch domain.Channel) error {
	_, err := r.ch.BlockChannel(ctx, connect.NewRequest(&v1.BlockChannelRequest{Channel: protoconv.ChannelToProto(ch)}))
	return err
}

func (r *Remote) UnblockChannel(ctx context.Context, channelID string) error {
	_, err := r.ch.UnblockChannel(ctx, connect.NewRequest(&v1.UnblockChannelRequest{ChannelId: channelID}))
	return err
}

func (r *Remote) SetChannelState(ctx context.Context, channelID string, state domain.SubscriptionState) error {
	_, err := r.ch.SetChannelState(ctx, connect.NewRequest(&v1.SetChannelStateRequest{ChannelId: channelID, State: string(state)}))
	return err
}

func (r *Remote) AddSubscribedChannel(ctx context.Context, ch domain.Channel) error {
	_, err := r.ch.AddSubscribedChannel(ctx, connect.NewRequest(&v1.AddSubscribedChannelRequest{Channel: protoconv.ChannelToProto(ch)}))
	return err
}

func (r *Remote) SaveSubscribedChannels(ctx context.Context, channels []domain.Channel) error {
	_, err := r.ch.SaveSubscribedChannels(ctx, connect.NewRequest(&v1.SaveSubscribedChannelsRequest{Channels: protoconv.ChannelsToProto(channels)}))
	return err
}

func (r *Remote) RemoveSubscribedChannel(ctx context.Context, channelID string) error {
	_, err := r.ch.RemoveSubscribedChannel(ctx, connect.NewRequest(&v1.RemoveSubscribedChannelRequest{ChannelId: channelID}))
	return err
}

func (r *Remote) DeleteChannelVideos(ctx context.Context, channelID string) error {
	_, err := r.ch.DeleteChannelVideos(ctx, connect.NewRequest(&v1.DeleteChannelVideosRequest{ChannelId: channelID}))
	return err
}

func (r *Remote) SetChannelAlias(ctx context.Context, channelID, alias string) error {
	_, err := r.ch.SetChannelAlias(ctx, connect.NewRequest(&v1.SetChannelAliasRequest{ChannelId: channelID, Alias: alias}))
	return err
}

func (r *Remote) SetChannelTags(ctx context.Context, channelID string, tags []string) error {
	_, err := r.ch.SetChannelTags(ctx, connect.NewRequest(&v1.SetChannelTagsRequest{ChannelId: channelID, Tags: tags}))
	return err
}

func (r *Remote) SaveChannelVideos(ctx context.Context, channelID string, videos []domain.Video) error {
	_, err := r.ch.SaveChannelVideos(ctx, connect.NewRequest(&v1.SaveChannelVideosRequest{ChannelId: channelID, Videos: protoconv.VideosToProto(videos)}))
	return err
}

func (r *Remote) Subscribe(ctx context.Context, ch domain.Channel) error {
	_, err := r.ch.Subscribe(ctx, connect.NewRequest(&v1.SubscribeRequest{Channel: protoconv.ChannelToProto(ch)}))
	return err
}

func (r *Remote) Unsubscribe(ctx context.Context, ch domain.Channel) error {
	_, err := r.ch.Unsubscribe(ctx, connect.NewRequest(&v1.UnsubscribeRequest{Channel: protoconv.ChannelToProto(ch)}))
	return err
}
