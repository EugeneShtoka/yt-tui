package service

import (
	"fmt"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/channels"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
)

// ChannelRepo is the persistence port required by ChannelService.
type ChannelRepo interface {
	GetSubscribedChannels() ([]domain.Channel, error)
	SaveSubscribedChannels(channels []domain.Channel) error
	AddSubscribedChannel(ch domain.Channel) error
	RemoveSubscribedChannel(channelID string) error
	DeleteChannelVideos(channelID string) error
	GetChannelVideos(channelID string) ([]domain.Video, error)
	SaveChannelVideos(channelID string, videos []domain.Video) error
}

// ChannelSource fetches channel data from YouTube via yt-dlp.
type ChannelSource interface {
	SubscribedChannels() ([]domain.Channel, error)
	ChannelVideos(channelURL, channelID string) ([]domain.Video, error)
	ChannelLatestN(channelURL, channelID string, n int) ([]domain.Video, error)
}

// YTAPIClient performs mutations against the YouTube internal API
// (subscribe, unsubscribe, playlist operations). May be nil until
// browser-cookie auth is initialized.
type YTAPIClient interface {
	Subscribe(channelID string) error
	Unsubscribe(channelID string) error
}

// ChannelService owns channel subscription and video-cache operations.
type ChannelService struct {
	repo   ChannelRepo
	source ChannelSource
	ytAPI  YTAPIClient // nil until InitYTClient is called
}

func NewChannelService(repo ChannelRepo, source ChannelSource) *ChannelService {
	return &ChannelService{repo: repo, source: source}
}

func (s *ChannelService) SetYTAPI(client YTAPIClient) { s.ytAPI = client }

// SubscribedChannels fetches the YT channel list, merges with locally-added
// channels, persists the result, and returns the merged list.
func (s *ChannelService) SubscribedChannels() ([]domain.Channel, error) {
	ytChannels, err := s.source.SubscribedChannels()
	if err != nil {
		return nil, fmt.Errorf("SubscribedChannels: %w", err)
	}
	existing, _ := s.repo.GetSubscribedChannels()
	merged := channels.Sync(existing, ytChannels)
	_ = s.repo.SaveSubscribedChannels(merged)
	return merged, nil
}

// Subscribe adds a channel subscription. Local channels are stored in DB only;
// remote channels call the YouTube API and then DB.
func (s *ChannelService) Subscribe(ch domain.Channel) error {
	if !ch.IsLocal {
		if s.ytAPI == nil {
			return fmt.Errorf("YouTube API not initialized")
		}
		if err := s.ytAPI.Subscribe(ch.ID); err != nil {
			return fmt.Errorf("Subscribe: %w", err)
		}
	}
	if err := s.repo.AddSubscribedChannel(ch); err != nil {
		return fmt.Errorf("Subscribe: %w", err)
	}
	return nil
}

// Unsubscribe removes a channel subscription. Routes local/remote based on
// ch.IsLocal — the caller already has the full channel object.
func (s *ChannelService) Unsubscribe(ch domain.Channel) error {
	if !ch.IsLocal {
		if s.ytAPI == nil {
			return fmt.Errorf("YouTube API not initialized")
		}
		if err := s.ytAPI.Unsubscribe(ch.ID); err != nil {
			return fmt.Errorf("Unsubscribe: %w", err)
		}
	} else {
		if err := s.repo.RemoveSubscribedChannel(ch.ID); err != nil {
			return fmt.Errorf("Unsubscribe: %w", err)
		}
	}
	if err := s.repo.DeleteChannelVideos(ch.ID); err != nil {
		return fmt.Errorf("Unsubscribe: %w", err)
	}
	return nil
}

// ChannelVideos fetches a channel's full video list, merges with the DB cache,
// persists, and returns the merged result.
func (s *ChannelService) ChannelVideos(channelURL, channelID string) ([]domain.Video, error) {
	fresh, err := s.source.ChannelVideos(channelURL, channelID)
	if err != nil {
		return nil, fmt.Errorf("ChannelVideos: %w", err)
	}
	cached, _ := s.repo.GetChannelVideos(channelID)
	merged := feed.MergeVideos(cached, fresh)
	_ = s.repo.SaveChannelVideos(channelID, merged)
	return merged, nil
}

// ChannelLatestN fetches the N most recent videos for a channel and persists them.
func (s *ChannelService) ChannelLatestN(channelURL, channelID string, n int) ([]domain.Video, error) {
	fresh, err := s.source.ChannelLatestN(channelURL, channelID, n)
	if err != nil {
		return nil, fmt.Errorf("ChannelLatestN: %w", err)
	}
	if len(fresh) > 0 {
		_ = s.repo.SaveChannelVideos(channelID, fresh)
	}
	return fresh, nil
}
