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
	// Blocklist returns the blocked-channel projection (IDs + unresolved
	// name-only blocks) used to filter the feed.
	Blocklist(ctx context.Context) (ids, names []string, err error)
	// ResolveBlockedName upgrades a name-only block to an ID-keyed one once a
	// feed video reveals the channel's ID.
	ResolveBlockedName(ctx context.Context, name, channelID string) error
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
	filtered := feed.FilterByAge(raw, s.cfg.RecommendedMaxAgeDays)
	filtered = feed.FilterByMinDuration(filtered, s.cfg.RecommendedMinDurationSecs)
	filtered = feed.FilterByMinViews(filtered, s.cfg.RecommendedMinViews)
	filtered = feed.FilterDownloaded(filtered, localMap)
	filtered = feed.FilterHidden(filtered, hidden)
	blIDs, blNames, err := s.repo.Blocklist(ctx)
	if err != nil {
		return nil, fmt.Errorf("Recommended: blocklist: %w", err)
	}
	filtered, blEnrich := feed.FilterBlacklisted(filtered, feed.NewBlocklist(blIDs, blNames))
	for _, e := range blEnrich {
		// Best-effort: a failed resolve only means we re-resolve next refresh; the
		// video is already filtered out of this result either way.
		if err := s.repo.ResolveBlockedName(ctx, e.Name, e.ChannelID); err != nil {
			debug.Log("FeedService.Recommended: ResolveBlockedName(%q): %v", e.Name, err)
		}
	}
	filtered = feed.FilterSubscribed(filtered, subs.Index())
	// Cache write is best-effort: the filtered list is already valid for display,
	// a failed save only means the next cold load is stale, not wrong.
	if err := s.repo.SaveFeedCache(ctx, "recommended", filtered); err != nil {
		debug.Log("FeedService.Recommended: SaveFeedCache: %v", err)
	}
	return filtered, nil
}
