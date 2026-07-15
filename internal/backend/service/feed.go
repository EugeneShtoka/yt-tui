package service

import (
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/channels"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
)

// FeedRepo is the persistence port required by FeedService.
type FeedRepo interface {
	GetSubscribedChannels() ([]domain.Channel, error)
	LocalVideos() ([]domain.LocalVideo, error)
	HiddenRecVideoIDs() (map[string]bool, error)
	SaveFeedCache(name string, videos []domain.Video) error
}

// RecommendSource is the fetch port for raw recommended videos.
type RecommendSource interface {
	Recommended() ([]domain.Video, error)
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
func (s *FeedService) Recommended() ([]domain.Video, error) {
	raw, err := s.source.Recommended()
	if err != nil {
		return nil, err
	}
	hidden, _ := s.repo.HiddenRecVideoIDs()
	localSlice, _ := s.repo.LocalVideos()
	localMap := make(map[string]domain.LocalVideo, len(localSlice))
	for _, lv := range localSlice {
		localMap[lv.ID] = lv
	}
	existing, _ := s.repo.GetSubscribedChannels()
	subs := channels.New(existing)
	filtered := feed.FilterByAge(raw, s.cfg.RecommendedMaxAgeDays)
	filtered = feed.FilterByMinDuration(filtered, s.cfg.RecommendedMinDurationSecs)
	filtered = feed.FilterByMinViews(filtered, s.cfg.RecommendedMinViews)
	filtered = feed.FilterDownloaded(filtered, localMap)
	filtered = feed.FilterHidden(filtered, hidden)
	filtered = feed.FilterBlacklisted(filtered, s.cfg.BlacklistedChannels, s.cfg)
	filtered = feed.FilterSubscribed(filtered, subs.Index())
	_ = s.repo.SaveFeedCache("recommended", filtered)
	return filtered, nil
}
