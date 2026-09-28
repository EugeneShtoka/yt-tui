package service

import (
	"context"
	"fmt"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/channels"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
)

// FeedRepo is the persistence port required by FeedService.
type FeedRepo interface {
	GetSubscribedChannels(ctx context.Context) ([]domain.Channel, error)
	LocalVideos(ctx context.Context) ([]domain.LocalVideo, error)
	HiddenRecVideoIDs(ctx context.Context) (map[string]bool, error)
	SaveFeedCache(ctx context.Context, name string, videos []domain.Video) error
	GetFeedCache(ctx context.Context, name string) ([]domain.Video, error)
	// Blocklist returns the IDs of channels flagged blocked=1, used to filter
	// the feed.
	Blocklist(ctx context.Context) (ids []string, err error)
}

// RecommendSource is the fetch port for raw recommended videos.
type RecommendSource interface {
	Recommended(ctx context.Context) ([]domain.Video, error)
}

// FeedService owns the recommended-feed pipeline: fetch → filter → persist.
type FeedService struct {
	repo   FeedRepo
	source RecommendSource
	cfg    *config.Config
}

func NewFeedService(repo FeedRepo, source RecommendSource, cfg *config.Config) *FeedService {
	return &FeedService{repo: repo, source: source, cfg: cfg}
}

// Recommended fetches raw videos, runs the full filter pipeline, persists the
// result, and returns the filtered list ready for the UI to sort and display.
func (s *FeedService) Recommended(ctx context.Context) ([]domain.Video, error) {
	raw, err := s.source.Recommended(ctx)
	if err != nil {
		return nil, fmt.Errorf("Recommended: %w", err)
	}
	// These reads feed the filter pipeline; an error here silently corrupts the
	// result (hidden videos reappear, downloaded/subscribed not filtered), so
	// fail loudly rather than return a wrong feed.
	hidden, err := s.repo.HiddenRecVideoIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("Recommended: hidden ids: %w", err)
	}
	localSlice, err := s.repo.LocalVideos(ctx)
	if err != nil {
		return nil, fmt.Errorf("Recommended: local videos: %w", err)
	}
	localMap := make(map[string]domain.LocalVideo, len(localSlice))
	for i := range localSlice {
		localMap[localSlice[i].ID] = localSlice[i]
	}
	existing, err := s.repo.GetSubscribedChannels(ctx)
	if err != nil {
		return nil, fmt.Errorf("Recommended: subscribed channels: %w", err)
	}
	subs := channels.New(existing)
	filtered := feed.FilterRecommended(raw, s.cfg.RecommendedMaxAgeDays, s.cfg.RecommendedMinDurationSecs, s.cfg.RecommendedMinViews)
	filtered = feed.FilterDownloaded(filtered, localMap)
	filtered = feed.FilterHidden(filtered, hidden)
	blIDs, err := s.repo.Blocklist(ctx)
	if err != nil {
		return nil, fmt.Errorf("Recommended: blocklist: %w", err)
	}
	filtered = feed.FilterBlacklisted(filtered, feed.NewBlocklist(blIDs))
	filtered = feed.FilterSubscribed(filtered, subs.Index())
	// Cache write is best-effort: the filtered list is already valid for display,
	// a failed save only means the next cold load is stale, not wrong.
	if err := s.repo.SaveFeedCache(ctx, "recommended", filtered); err != nil {
		debug.Log("FeedService.Recommended: SaveFeedCache: %v", err)
	}
	return filtered, nil
}

// FeedCache returns the persisted feed, re-applying the user's two suppression
// filters — the channel blocklist and the hidden-video set — on the way out.
//
// The cache is a snapshot of a past fetch, so it goes stale the moment the user
// blocks a channel or hides a video: those rows stay in feed_cache until some
// later fetch overwrites them. Since the cache read is the cold-start path (the
// Feed tab seeds from it before the network fetch lands), filtering only inside
// Recommended left blocked channels and hidden videos visible after every
// restart. Applying the filters here makes the stored cache advisory and the DB
// flags authoritative, so a suppression survives a restart even if feed_cache
// was never rewritten.
//
// Only the suppression filters run here. The property filters (age, duration,
// views) belong to the fetch path and the Feed tab's cumulative display pass,
// which already re-applies them to the whole accumulated list.
func (s *FeedService) FeedCache(ctx context.Context, name string) ([]domain.Video, error) {
	videos, err := s.repo.GetFeedCache(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("FeedCache: %w", err)
	}
	if len(videos) == 0 {
		return videos, nil
	}
	blIDs, err := s.repo.Blocklist(ctx)
	if err != nil {
		return nil, fmt.Errorf("FeedCache: blocklist: %w", err)
	}
	hidden, err := s.repo.HiddenRecVideoIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("FeedCache: hidden ids: %w", err)
	}
	videos = feed.FilterBlacklisted(videos, feed.NewBlocklist(blIDs))
	videos = feed.FilterHidden(videos, hidden)
	return videos, nil
}
