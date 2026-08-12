//nolint:wrapcheck // pass-through adapter; errors from backend/db/yt are already contextual
package api

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ── ChannelBackend ───────────────────────────────────────────────────────────

func (p *InProc) SubscribedChannels(ctx context.Context) ([]domain.Channel, error) {
	return p.ch.SubscribedChannels(ctx)
}

func (p *InProc) ChannelVideos(ctx context.Context, channelURL, channelID string) ([]domain.Video, error) {
	return p.ch.ChannelVideos(ctx, channelURL, channelID)
}

func (p *InProc) ChannelLatestN(ctx context.Context, channelURL, channelID string, n int) ([]domain.Video, error) {
	return p.ch.ChannelLatestN(ctx, channelURL, channelID, n)
}

func (p *InProc) Search(ctx context.Context, query string) ([]domain.Channel, []domain.Video, error) {
	return p.yt.Search(ctx, query)
}

func (p *InProc) GetChannelVideos(ctx context.Context, channelID string) ([]domain.Video, error) {
	return p.db.GetChannelVideos(ctx, channelID)
}

func (p *InProc) GetAllChannelVideos(ctx context.Context, channelIDs []string) ([]domain.Video, error) {
	return p.db.GetAllChannelVideos(ctx, channelIDs)
}

func (p *InProc) GetChannelLatestAll(ctx context.Context) (map[string]domain.Video, error) {
	return p.db.GetChannelLatestAll(ctx)
}

func (p *InProc) ChannelHideStats(ctx context.Context, channelID string) (int, int, error) {
	return p.db.ChannelHideStats(ctx, channelID)
}

func (p *InProc) GetSubscribedChannels(ctx context.Context) ([]domain.Channel, error) {
	return p.db.GetSubscribedChannels(ctx)
}

func (p *InProc) AllChannels(ctx context.Context) ([]domain.Channel, error) {
	return p.ch.AllChannels(ctx)
}

func (p *InProc) BlockedChannels(ctx context.Context) ([]domain.Channel, error) {
	return p.ch.BlockedChannels(ctx)
}

func (p *InProc) AddSubscribedChannel(ctx context.Context, ch domain.Channel) error {
	return p.db.AddSubscribedChannel(ctx, ch)
}

func (p *InProc) SaveSubscribedChannels(ctx context.Context, channels []domain.Channel) error {
	return p.db.SaveSubscribedChannels(ctx, channels)
}

func (p *InProc) RemoveSubscribedChannel(ctx context.Context, channelID string) error {
	return p.db.RemoveSubscribedChannel(ctx, channelID)
}

func (p *InProc) DeleteChannelVideos(ctx context.Context, channelID string) error {
	return p.db.DeleteChannelVideos(ctx, channelID)
}

func (p *InProc) SetChannelAlias(ctx context.Context, channelID, alias string) error {
	return p.db.SetChannelAlias(ctx, channelID, alias)
}

func (p *InProc) SetChannelTags(ctx context.Context, channelID string, tags []string) error {
	return p.db.SetChannelTags(ctx, channelID, tags)
}

func (p *InProc) SaveChannelVideos(ctx context.Context, channelID string, videos []domain.Video) error {
	return p.db.SaveChannelVideos(ctx, channelID, videos)
}

// Subscribe and Unsubscribe delegate to ChannelService, which routes
// local/remote channels based on ch.IsLocal.
func (p *InProc) Subscribe(ctx context.Context, ch domain.Channel) error {
	return p.ch.Subscribe(ctx, ch)
}

func (p *InProc) Unsubscribe(ctx context.Context, ch domain.Channel) error {
	return p.ch.Unsubscribe(ctx, ch)
}

func (p *InProc) BlockChannel(ctx context.Context, ch domain.Channel) error {
	return p.ch.Block(ctx, ch)
}

func (p *InProc) UnblockChannel(ctx context.Context, channelID string) error {
	return p.ch.Unblock(ctx, channelID)
}

func (p *InProc) SetChannelState(ctx context.Context, channelID string, state domain.SubscriptionState) error {
	return p.ch.SetChannelState(ctx, channelID, state)
}
