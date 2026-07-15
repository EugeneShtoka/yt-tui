package api

import (
	"context"
	"fmt"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

// InProc implements Backend by calling db, youtube.Client, and downloader directly.
type InProc struct {
	db    *db.DB
	yt    *youtube.Client
	ytAPI *youtube.YTClient // nil until InitYTClient is called
	dl    *downloader.Downloader
	cfg   *config.Config
}

// NewInProc creates an InProc Backend.
func NewInProc(database *db.DB, yt *youtube.Client, dl *downloader.Downloader, cfg *config.Config) *InProc {
	return &InProc{db: database, yt: yt, dl: dl, cfg: cfg}
}

// ── YouTube fetch ─────────────────────────────────────────────────────────────

func (p *InProc) Recommended(_ context.Context) ([]domain.Video, error) {
	return p.yt.Recommended()
}

func (p *InProc) SubscribedChannels(_ context.Context) ([]domain.Channel, error) {
	return p.yt.SubscribedChannels()
}

func (p *InProc) ChannelVideos(_ context.Context, channelURL, channelID string) ([]domain.Video, error) {
	return p.yt.ChannelVideos(channelURL, channelID)
}

func (p *InProc) ChannelLatestN(_ context.Context, channelURL, channelID string, n int) ([]domain.Video, error) {
	return p.yt.ChannelLatestN(channelURL, channelID, n)
}

func (p *InProc) Search(_ context.Context, query string) ([]domain.Channel, []domain.Video, error) {
	return p.yt.Search(query)
}

func (p *InProc) YTPlaylists(_ context.Context) ([]domain.YTPlaylist, error) {
	return p.yt.YTPlaylists()
}

func (p *InProc) YTPlaylistVideos(_ context.Context, playlistID string) ([]domain.Video, error) {
	return p.yt.PlaylistVideos(playlistID)
}

func (p *InProc) VideoDetails(_ context.Context, videoURL string) (domain.VideoDetails, error) {
	return p.yt.VideoDetails(videoURL)
}

// ── Local DB reads ────────────────────────────────────────────────────────────

func (p *InProc) LocalVideos(_ context.Context) ([]domain.LocalVideo, error) {
	return p.db.LocalVideos()
}

func (p *InProc) LocalPlaylists(_ context.Context) ([]domain.Playlist, error) {
	return p.db.Playlists()
}

func (p *InProc) LocalPlaylistVideos(_ context.Context, playlistID int64) ([]domain.Video, error) {
	return p.db.PlaylistVideos(playlistID)
}

func (p *InProc) History(_ context.Context, limit int) ([]domain.HistoryEntry, error) {
	return p.db.History(limit)
}

func (p *InProc) HistoryVideos(_ context.Context, limit int) ([]domain.HistoryEntry, error) {
	return p.db.HistoryVideos(limit)
}

func (p *InProc) VideoHistory(_ context.Context, videoID string) ([]domain.HistoryEntry, error) {
	return p.db.VideoHistory(videoID)
}

func (p *InProc) ActivityLog(_ context.Context, limit int) ([]domain.ActivityEntry, error) {
	return p.db.GetActivityLog(limit)
}

func (p *InProc) VideoPosition(_ context.Context, videoID string) (int64, bool) {
	return p.db.VideoPosition(videoID)
}

func (p *InProc) WatchedVideoIDs(_ context.Context) (map[string]bool, error) {
	return p.db.WatchedVideoIDs()
}

func (p *InProc) HiddenRecVideoIDs(_ context.Context) (map[string]bool, error) {
	return p.db.HiddenRecVideoIDs()
}

func (p *InProc) AllVideoPositions(_ context.Context) (map[string]int64, error) {
	return p.db.AllVideoPositions()
}

func (p *InProc) SearchQueries(_ context.Context, limit int) ([]string, error) {
	return p.db.SearchQueries(limit)
}

func (p *InProc) GetVideoDetailsCache(_ context.Context, videoID string) (domain.CachedDetails, bool, error) {
	return p.db.GetVideoDetailsCache(videoID)
}

func (p *InProc) GetChannelVideos(_ context.Context, channelID string) ([]domain.Video, error) {
	return p.db.GetChannelVideos(channelID)
}

func (p *InProc) GetAllChannelVideos(_ context.Context, channelIDs []string) ([]domain.Video, error) {
	return p.db.GetAllChannelVideos(channelIDs)
}

func (p *InProc) GetChannelLatestAll(_ context.Context) (map[string]domain.Video, error) {
	return p.db.GetChannelLatestAll()
}

func (p *InProc) ChannelHideStats(_ context.Context, channelID string) (int, int, error) {
	return p.db.ChannelHideStats(channelID)
}

func (p *InProc) WatchLater(_ context.Context) ([]domain.WatchLaterEntry, error) {
	return p.db.WatchLater()
}

func (p *InProc) HasLocalVideo(_ context.Context, videoID string) (domain.LocalVideo, bool) {
	return p.db.HasLocalVideo(videoID)
}

func (p *InProc) GetSubscribedChannels(_ context.Context) ([]domain.Channel, error) {
	return p.db.GetSubscribedChannels()
}

func (p *InProc) GetYTPlaylists(_ context.Context) ([]domain.YTPlaylist, error) {
	return p.db.GetYTPlaylists()
}

func (p *InProc) GetYTPlaylistVideos(_ context.Context, playlistID string) ([]domain.Video, error) {
	return p.db.GetYTPlaylistVideos(playlistID)
}

func (p *InProc) GetFeedCache(_ context.Context, feed string) ([]domain.Video, error) {
	return p.db.GetFeedCache(feed)
}

func (p *InProc) PlaylistVideoIDs(_ context.Context, playlistID int64) ([]string, error) {
	return p.db.PlaylistVideoIDs(playlistID)
}

// ── Mutations ─────────────────────────────────────────────────────────────────

func (p *InProc) UpsertVideo(_ context.Context, id, title, channel, channelID string, duration int, viewCount int64, uploadDate, url string) error {
	return p.db.UpsertVideo(id, title, channel, channelID, duration, viewCount, uploadDate, url)
}

func (p *InProc) AddLocalVideo(_ context.Context, v domain.LocalVideo) error {
	return p.db.AddLocalVideo(v)
}

func (p *InProc) SetVideoStatus(_ context.Context, id string, status domain.VideoStatus) error {
	return p.db.SetVideoStatus(id, status)
}

func (p *InProc) DeleteLocalVideo(_ context.Context, id string) error {
	return p.db.DeleteLocalVideo(id)
}

func (p *InProc) SaveVideoPosition(_ context.Context, videoID string, ms int64) error {
	return p.db.SaveVideoPosition(videoID, ms)
}

func (p *InProc) DeleteVideoPosition(_ context.Context, videoID string) error {
	return p.db.DeleteVideoPosition(videoID)
}

func (p *InProc) UpdateLastPosition(_ context.Context, id string, ms int64) error {
	return p.db.UpdateLastPosition(id, ms)
}

func (p *InProc) AddHistory(_ context.Context, videoID, eventType, details string) error {
	return p.db.AddHistory(videoID, eventType, details)
}

func (p *InProc) DeleteVideoHistory(_ context.Context, videoID string) error {
	return p.db.DeleteVideoHistory(videoID)
}

func (p *InProc) DeleteSearchHistory(_ context.Context, query string) error {
	return p.db.DeleteSearchHistory(query)
}

func (p *InProc) ClearHistory(_ context.Context) error {
	return p.db.ClearHistory()
}

func (p *InProc) LogActivity(_ context.Context, e domain.ActivityEntry) error {
	return p.db.LogActivity(e)
}

func (p *InProc) HideRecVideo(_ context.Context, videoID string) error {
	return p.db.HideRecVideo(videoID)
}

func (p *InProc) AddSubscribedChannel(_ context.Context, ch domain.Channel) error {
	return p.db.AddSubscribedChannel(ch)
}

func (p *InProc) SaveSubscribedChannels(_ context.Context, channels []domain.Channel) error {
	return p.db.SaveSubscribedChannels(channels)
}

func (p *InProc) RemoveSubscribedChannel(_ context.Context, channelID string) error {
	return p.db.RemoveSubscribedChannel(channelID)
}

func (p *InProc) DeleteChannelVideos(_ context.Context, channelID string) error {
	return p.db.DeleteChannelVideos(channelID)
}

func (p *InProc) SetChannelAlias(_ context.Context, channelID, alias string) error {
	return p.db.SetChannelAlias(channelID, alias)
}

func (p *InProc) SetChannelTags(_ context.Context, channelID string, tags []string) error {
	return p.db.SetChannelTags(channelID, tags)
}

func (p *InProc) SaveChannelVideos(_ context.Context, channelID string, videos []domain.Video) error {
	return p.db.SaveChannelVideos(channelID, videos)
}

func (p *InProc) CreatePlaylist(_ context.Context, name string) (int64, error) {
	return p.db.CreatePlaylist(name)
}

func (p *InProc) DeletePlaylist(_ context.Context, id int64) error {
	return p.db.DeletePlaylist(id)
}

func (p *InProc) AddToPlaylist(_ context.Context, playlistID int64, videoID string) error {
	return p.db.AddToPlaylist(playlistID, videoID)
}

func (p *InProc) RemoveFromPlaylist(_ context.Context, playlistID int64, videoID string) error {
	return p.db.RemoveFromPlaylist(playlistID, videoID)
}

func (p *InProc) AddWatchLater(_ context.Context, id, title, channel, url string) error {
	return p.db.AddWatchLater(id, title, channel, url)
}

func (p *InProc) RemoveWatchLater(_ context.Context, id string) error {
	return p.db.RemoveWatchLater(id)
}

func (p *InProc) SaveYTPlaylists(_ context.Context, playlists []domain.YTPlaylist) error {
	return p.db.SaveYTPlaylists(playlists)
}

func (p *InProc) SaveYTPlaylistVideos(_ context.Context, playlistID string, videos []domain.Video) error {
	return p.db.SaveYTPlaylistVideos(playlistID, videos)
}

func (p *InProc) SaveVideoDetailsCache(_ context.Context, videoID, description, thumbnailURL string, subscribers int64) error {
	return p.db.SaveVideoDetailsCache(videoID, description, thumbnailURL, subscribers)
}

func (p *InProc) SaveVideoChapters(_ context.Context, videoID string, chapters []domain.Chapter) error {
	return p.db.SaveVideoChapters(videoID, chapters)
}

func (p *InProc) SaveVideoSBSegments(_ context.Context, videoID string, segs []domain.SBSegment) error {
	return p.db.SaveVideoSBSegments(videoID, segs)
}

func (p *InProc) SaveVideoLinks(_ context.Context, videoID string, links []domain.Link) error {
	return p.db.SaveVideoLinks(videoID, links)
}

func (p *InProc) ClearRecommended(_ context.Context) error {
	return p.db.ClearRecommended()
}

func (p *InProc) ClearVideoDetailsCache(_ context.Context) error {
	return p.db.ClearVideoDetailsCache()
}

func (p *InProc) ClearDownloads(_ context.Context) ([]string, error) {
	return p.db.ClearDownloads()
}

func (p *InProc) PurgeFeedCacheMissingChannelID(_ context.Context, feed string) error {
	return p.db.PurgeFeedCacheMissingChannelID(feed)
}

func (p *InProc) SaveFeedCache(_ context.Context, feed string, videos []domain.Video) error {
	return p.db.SaveFeedCache(feed, videos)
}

// ── Download queue ────────────────────────────────────────────────────────────

func (p *InProc) Enqueue(_ context.Context, video domain.Video, audioOnly bool) error {
	dlType := downloader.TypeVideo
	if audioOnly {
		dlType = downloader.TypeAudio
	}
	p.dl.Start(video, dlType)
	return nil
}

func (p *InProc) CancelDownload(_ context.Context, videoID string) error {
	p.dl.Remove(videoID)
	return nil
}

// ── YouTube API mutations ─────────────────────────────────────────────────────

func (p *InProc) InitYTClient(_ context.Context) error {
	client, err := youtube.NewYTClient(p.cfg)
	if err != nil {
		return err
	}
	p.ytAPI = client
	return nil
}

func (p *InProc) Subscribe(_ context.Context, ch domain.Channel) error {
	if !ch.IsLocal {
		if p.ytAPI == nil {
			return fmt.Errorf("YouTube API not initialised")
		}
		if err := p.ytAPI.Subscribe(ch.ID); err != nil {
			return err
		}
	}
	return p.db.AddSubscribedChannel(ch)
}

func (p *InProc) Unsubscribe(_ context.Context, ch domain.Channel) error {
	if !ch.IsLocal {
		if p.ytAPI == nil {
			return fmt.Errorf("YouTube API not initialised")
		}
		if err := p.ytAPI.Unsubscribe(ch.ID); err != nil {
			return err
		}
	} else {
		if err := p.db.RemoveSubscribedChannel(ch.ID); err != nil {
			return err
		}
	}
	return p.db.DeleteChannelVideos(ch.ID)
}

func (p *InProc) CreateYTPlaylist(_ context.Context, name string) (string, error) {
	if p.ytAPI == nil {
		return "", fmt.Errorf("YouTube API not initialised")
	}
	return p.ytAPI.CreatePlaylist(name)
}

func (p *InProc) DeleteYTPlaylist(_ context.Context, playlistID string) error {
	if p.ytAPI == nil {
		return fmt.Errorf("YouTube API not initialised")
	}
	return p.ytAPI.DeletePlaylist(playlistID)
}

func (p *InProc) AddToYTPlaylist(_ context.Context, playlistID, videoID string) error {
	if p.ytAPI == nil {
		return fmt.Errorf("YouTube API not initialised")
	}
	return p.ytAPI.AddToPlaylist(playlistID, videoID)
}

func (p *InProc) RemoveFromYTPlaylist(_ context.Context, playlistID, videoID string) error {
	if p.ytAPI == nil {
		return fmt.Errorf("YouTube API not initialised")
	}
	return p.ytAPI.RemoveFromPlaylist(playlistID, videoID)
}

// Events bridges the downloader's event channel into api.Event values.
// The returned channel stays open until ctx is cancelled.
func (p *InProc) Events(ctx context.Context) (<-chan Event, error) {
	out := make(chan Event, 64)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-p.dl.EventChan():
				if !ok {
					return
				}
				var kind EventKind
				var detail string
				switch ev.Kind {
				case downloader.EventProgress:
					kind = EventDownloadProgress
					detail = fmt.Sprintf("%.0f%% %s ETA %s", ev.Progress, ev.Speed, ev.ETA)
				case downloader.EventComplete:
					kind = EventDownloadDone
					detail = ev.FilePath
				case downloader.EventError:
					kind = EventDownloadError
					if ev.Err != nil {
						detail = ev.Err.Error()
					}
				default:
					continue
				}
				select {
				case out <- Event{Kind: kind, VideoID: ev.VideoID, Detail: detail}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}
