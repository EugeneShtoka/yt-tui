//nolint:wrapcheck // pass-through adapter; errors from backend/db/yt are already contextual
package api

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ── FeedBackend ──────────────────────────────────────────────────────────────

func (p *InProc) Recommended(ctx context.Context) ([]domain.Video, error) {
	return p.feed.Recommended(ctx)
}

func (p *InProc) GetFeedCache(ctx context.Context, feed string) ([]domain.Video, error) {
	return p.db.GetFeedCache(ctx, feed)
}

func (p *InProc) SaveFeedCache(ctx context.Context, feed string, videos []domain.Video) error {
	return p.db.SaveFeedCache(ctx, feed, videos)
}

func (p *InProc) PurgeFeedCacheMissingChannelID(ctx context.Context, feed string) error {
	return p.db.PurgeFeedCacheMissingChannelID(ctx, feed)
}

func (p *InProc) HideRecVideo(ctx context.Context, videoID string) error {
	return p.db.HideRecVideo(ctx, videoID)
}

func (p *InProc) HiddenRecVideoIDs(ctx context.Context) (map[string]bool, error) {
	return p.db.HiddenRecVideoIDs(ctx)
}

func (p *InProc) WatchedVideoIDs(ctx context.Context) (map[string]bool, error) {
	return p.db.WatchedVideoIDs(ctx)
}

func (p *InProc) ClearRecommended(ctx context.Context) error {
	return p.db.ClearRecommended(ctx)
}
