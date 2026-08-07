package service

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/debug"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/channels"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
)

// ChannelRepo is the persistence port required by ChannelService.
type ChannelRepo interface {
	GetSubscribedChannels(ctx context.Context) ([]domain.Channel, error)
	AllChannels(ctx context.Context) ([]domain.Channel, error)
	BlockedChannels(ctx context.Context) ([]domain.Channel, error)
	SaveSubscribedChannels(ctx context.Context, channels []domain.Channel) error
	AddSubscribedChannel(ctx context.Context, ch domain.Channel) error
	RemoveSubscribedChannel(ctx context.Context, channelID string) error
	SetChannelState(ctx context.Context, channelID string, state domain.SubscriptionState) error
	BlockChannel(ctx context.Context, channelID string) error
	UnblockChannel(ctx context.Context, channelID string) error
	DeleteChannelVideos(ctx context.Context, channelID string) error
	GetChannelVideos(ctx context.Context, channelID string) ([]domain.Video, error)
	SaveChannelVideos(ctx context.Context, channelID string, videos []domain.Video) error
	TouchChannelVideosRefreshed(ctx context.Context, channelID string) error
	SetChannelFetchOffset(ctx context.Context, channelID string, offset int64) error
	StampChannelActivity(ctx context.Context, channelIDs ...string) error
}

// ChannelSource fetches channel data from YouTube via yt-dlp.
type ChannelSource interface {
	SubscribedChannels(ctx context.Context) ([]domain.Channel, error)
	// ChannelVideosStream fetches a channel's full video list, paginated,
	// invoking onPage after each page so the caller can persist incrementally.
	ChannelVideosStream(ctx context.Context, channelURL, channelID string, onPage func([]domain.Video) error) ([]domain.Video, error)
	// ChannelVideosPage fetches one page of a channel's videos starting at the
	// 1-based list offset `start`, returning the offset to resume from next and
	// whether more pages remain. Lets backfill crawl many channels breadth-first
	// (round-robin) and persist a resume cursor rather than draining each in turn.
	ChannelVideosPage(ctx context.Context, channelURL, channelID string, start int) ([]domain.Video, int, bool, error)
	ChannelLatestN(ctx context.Context, channelURL, channelID string, n int) ([]domain.Video, error)
}

// YTAPIClient performs mutations against the YouTube internal API
// (subscribe, unsubscribe, playlist operations). May be nil until
// browser-cookie auth is initialized.
type YTAPIClient interface {
	Subscribe(ctx context.Context, channelID string) error
	Unsubscribe(ctx context.Context, channelID string) error
}

// ChannelService owns channel subscription and video-cache operations, plus
// single-channel drill-in fetches. The startup backfill crawl is delegated to
// an owned backfillScheduler (M-2) rather than living here.
type ChannelService struct {
	repo     ChannelRepo
	source   ChannelSource
	ytAPI    atomic.Pointer[YTAPIClient] // nil until InitYTClient is called
	backfill *backfillScheduler
}

func NewChannelService(repo ChannelRepo, source ChannelSource) *ChannelService {
	s := &ChannelService{repo: repo, source: source}
	// The scheduler reuses this service's ChannelLatestN for the stale-refresh
	// path so a backfill refresh and a drill-in refresh behave identically.
	s.backfill = newBackfillScheduler(repo, source, s.ChannelLatestN)
	return s
}

func (s *ChannelService) SetYTAPI(client YTAPIClient) { s.ytAPI.Store(&client) }

// SubscribedChannels fetches the YT channel list, merges with locally-added
// channels, persists the result, and returns the merged list.
func (s *ChannelService) SubscribedChannels(ctx context.Context) ([]domain.Channel, error) {
	ytChannels, err := s.source.SubscribedChannels(ctx)
	if err != nil {
		return nil, fmt.Errorf("SubscribedChannels: %w", err)
	}
	existing, err := s.repo.GetSubscribedChannels(ctx)
	if err != nil {
		return nil, fmt.Errorf("SubscribedChannels: %w", err)
	}
	merged := channels.Sync(existing, ytChannels)
	if err := s.repo.SaveSubscribedChannels(ctx, merged); err != nil {
		debug.Log("ChannelService.SubscribedChannels: save: %v", err)
	}
	return merged, nil
}

// Subscribe adds a channel subscription. Local channels are stored in DB only;
// remote channels call the YouTube API and then DB.
func (s *ChannelService) Subscribe(ctx context.Context, ch domain.Channel) error {
	if !ch.IsLocal {
		ytAPI := s.ytAPI.Load()
		if ytAPI == nil {
			return domain.ErrYTNotInitialized
		}
		if err := (*ytAPI).Subscribe(ctx, ch.ID); err != nil {
			return fmt.Errorf("Subscribe: %w", err)
		}
	}
	if err := s.repo.AddSubscribedChannel(ctx, ch); err != nil {
		return fmt.Errorf("Subscribe: %w", err)
	}
	return nil
}

// Unsubscribe removes a channel subscription. Routes local/remote based on
// ch.IsLocal — the caller already has the full channel object.
func (s *ChannelService) Unsubscribe(ctx context.Context, ch domain.Channel) error {
	if !ch.IsLocal {
		ytAPI := s.ytAPI.Load()
		if ytAPI == nil {
			return domain.ErrYTNotInitialized
		}
		if err := (*ytAPI).Unsubscribe(ctx, ch.ID); err != nil {
			return fmt.Errorf("Unsubscribe: %w", err)
		}
	} else {
		if err := s.repo.RemoveSubscribedChannel(ctx, ch.ID); err != nil {
			return fmt.Errorf("Unsubscribe: %w", err)
		}
	}
	if err := s.repo.DeleteChannelVideos(ctx, ch.ID); err != nil {
		return fmt.Errorf("Unsubscribe: %w", err)
	}
	return nil
}

// AllChannels returns every known channel row (subscribed, annotated-but-none,
// and blocked) — the universe backing the all-channels views.
func (s *ChannelService) AllChannels(ctx context.Context) ([]domain.Channel, error) {
	chs, err := s.repo.AllChannels(ctx)
	if err != nil {
		return nil, fmt.Errorf("AllChannels: %w", err)
	}
	return chs, nil
}

// BlockedChannels returns the channels currently on the blocklist.
func (s *ChannelService) BlockedChannels(ctx context.Context) ([]domain.Channel, error) {
	chs, err := s.repo.BlockedChannels(ctx)
	if err != nil {
		return nil, fmt.Errorf("BlockedChannels: %w", err)
	}
	return chs, nil
}

// SetChannelState transitions a channel's subscription state. The repo enforces
// the block invariant (rejecting a subscribe on a blocked channel with
// domain.ErrChannelBlocked).
func (s *ChannelService) SetChannelState(ctx context.Context, channelID string, state domain.SubscriptionState) error {
	if err := s.repo.SetChannelState(ctx, channelID, state); err != nil {
		return fmt.Errorf("SetChannelState: %w", err)
	}
	return nil
}

// Block performs the guarded block transition: it best-effort unsubscribes on
// YouTube when the channel is YT-subscribed and the API is available (blocking
// must still work offline / without auth — the DB block is authoritative for
// local filtering, so a missing YT client only leaves the upstream subscription
// in place, which the next sync keeps at 'none' via the block invariant), then
// flags the channel blocked and clears its cached videos (mirroring Unsubscribe).
func (s *ChannelService) Block(ctx context.Context, ch domain.Channel) error {
	if ch.SubState() == domain.SubYT {
		if ytAPI := s.ytAPI.Load(); ytAPI != nil {
			if err := (*ytAPI).Unsubscribe(ctx, ch.ID); err != nil {
				return fmt.Errorf("Block: yt unsubscribe: %w", err)
			}
		} else {
			debug.Log("ChannelService.Block: yt api unavailable; blocking %q locally only", ch.ID)
		}
	}
	if err := s.repo.BlockChannel(ctx, ch.ID); err != nil {
		return fmt.Errorf("Block: %w", err)
	}
	if err := s.repo.DeleteChannelVideos(ctx, ch.ID); err != nil {
		return fmt.Errorf("Block: %w", err)
	}
	return nil
}

// Unblock clears the blocked flag, leaving the channel unsubscribed (state
// 'none'); the user re-subscribes deliberately.
func (s *ChannelService) Unblock(ctx context.Context, channelID string) error {
	if err := s.repo.UnblockChannel(ctx, channelID); err != nil {
		return fmt.Errorf("Unblock: %w", err)
	}
	return nil
}

// ChannelVideos fetches a channel's full video list, merges with the DB cache,
// persists, and returns the merged result.
func (s *ChannelService) ChannelVideos(ctx context.Context, channelURL, channelID string) ([]domain.Video, error) {
	// Persist each page as it arrives so a long back-catalog crawl becomes
	// visible (a drilled-in TUI polls GetChannelVideos) before the full pull
	// finishes. SaveChannelVideos upserts additively, so per-page saves
	// accumulate. A per-page save error is logged, not fatal: a transient write
	// failure must not abort a multi-minute crawl, and the authoritative save
	// below re-persists the merged result.
	onPage := func(page []domain.Video) error {
		if err := s.repo.SaveChannelVideos(ctx, channelID, page); err != nil {
			debug.Log("ChannelService.ChannelVideos: incremental save: %v", err)
		}
		return nil
	}
	fresh, err := s.source.ChannelVideosStream(ctx, channelURL, channelID, onPage)
	if err != nil {
		return nil, fmt.Errorf("ChannelVideos: %w", err)
	}
	// markComplete=true: a drill-in streams the whole catalog to completion, so
	// record it as fully crawled (backfill then only latest-N's it afterward).
	merged, err := s.mergeAndPersist(ctx, channelID, fresh, true)
	if err != nil {
		return nil, fmt.Errorf("ChannelVideos: %w", err)
	}
	return merged, nil
}

// ChannelLatestN fetches the N most recent videos for a channel, merges them
// into the cached full list, persists, and returns the merged result. Merging
// (rather than replacing) keeps a previously-loaded full list intact when the
// background refresh only pulls the newest N videos.
func (s *ChannelService) ChannelLatestN(ctx context.Context, channelURL, channelID string, n int) ([]domain.Video, error) {
	fresh, err := s.source.ChannelLatestN(ctx, channelURL, channelID, n)
	if err != nil {
		return nil, fmt.Errorf("ChannelLatestN: %w", err)
	}
	// markComplete=false: latest-N is a partial refresh, so it must not clear the
	// fetch offset that tracks how far the back-catalog crawl has progressed.
	merged, err := s.mergeAndPersist(ctx, channelID, fresh, false)
	if err != nil {
		return nil, fmt.Errorf("ChannelLatestN: %w", err)
	}
	return merged, nil
}

// mergeAndPersist folds freshly-fetched videos into the channel's cached list,
// sorts newest-first, persists the merged result, and stamps the refresh and
// activity times. When markComplete is set it also records that the whole
// back-catalog was crawled. Persist/stamp failures are logged, not fatal — the
// merged slice is already the return value and a transient write must not fail a
// fetch. Shared by ChannelVideos and ChannelLatestN (M-7).
func (s *ChannelService) mergeAndPersist(ctx context.Context, channelID string, fresh []domain.Video, markComplete bool) ([]domain.Video, error) {
	cached, err := s.repo.GetChannelVideos(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("cached: %w", err)
	}
	merged := feed.MergeVideos(cached, fresh)
	feed.SortVideos(merged, feed.SortDate)
	if err := s.repo.SaveChannelVideos(ctx, channelID, merged); err != nil {
		debug.Log("ChannelService.mergeAndPersist: save %s: %v", channelID, err)
	}
	if err := s.repo.TouchChannelVideosRefreshed(ctx, channelID); err != nil {
		debug.Log("ChannelService.mergeAndPersist: touch %s: %v", channelID, err)
	}
	if markComplete {
		if err := s.repo.SetChannelFetchOffset(ctx, channelID, domain.FetchOffsetComplete); err != nil {
			debug.Log("ChannelService.mergeAndPersist: fetch-offset %s: %v", channelID, err)
		}
	}
	// Fetching a channel's videos is a deliberate engagement (drill-in / refresh),
	// so keep the channel fresh for the stale filter.
	if err := s.repo.StampChannelActivity(ctx, channelID); err != nil {
		debug.Log("ChannelService.mergeAndPersist: stamp %s: %v", channelID, err)
	}
	return merged, nil
}

// BackfillSubscribed delegates to the owned backfillScheduler (M-2). It is kept
// on ChannelService so callers (the InProc backend's background pass) keep a
// single channel entry point; the scheduling algorithm itself lives in
// backfill.go. See backfillScheduler.Run for the full contract.
func (s *ChannelService) BackfillSubscribed(ctx context.Context, latestN int, staleAfter, delay time.Duration) (int, error) {
	return s.backfill.Run(ctx, latestN, staleAfter, delay)
}
