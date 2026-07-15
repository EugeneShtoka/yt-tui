package api

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// EventKind identifies the type of a daemon→TUI notification.
type EventKind string

const (
	EventDownloadProgress EventKind = "download_progress"
	EventDownloadDone     EventKind = "download_done"
	EventDownloadError    EventKind = "download_error"
)

// Event is a daemon→TUI notification (download progress, feed refresh, new videos).
type Event struct {
	Kind    EventKind
	VideoID string
	Detail  string // human-readable status / error message
}

// Backend is the contract between the TUI and the data layer.
// InProc implements it by calling db/youtube/downloader directly.
type Backend interface {
	// ── YouTube fetch ──────────────────────────────────────────────────────────
	Recommended(ctx context.Context) ([]domain.Video, error)
	SubscribedChannels(ctx context.Context) ([]domain.Channel, error)
	ChannelVideos(ctx context.Context, channelURL, channelID string) ([]domain.Video, error)
	ChannelLatestN(ctx context.Context, channelURL, channelID string, n int) ([]domain.Video, error)
	Search(ctx context.Context, query string) (channels []domain.Channel, videos []domain.Video, err error)
	YTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error)
	YTPlaylistVideos(ctx context.Context, playlistID string) ([]domain.Video, error)
	VideoDetails(ctx context.Context, videoURL string) (domain.VideoDetails, error)

	// ── Local DB reads ─────────────────────────────────────────────────────────
	LocalVideos(ctx context.Context) ([]domain.LocalVideo, error)
	LocalPlaylists(ctx context.Context) ([]domain.Playlist, error)
	LocalPlaylistVideos(ctx context.Context, playlistID int64) ([]domain.Video, error)
	History(ctx context.Context, limit int) ([]domain.HistoryEntry, error)
	HistoryVideos(ctx context.Context, limit int) ([]domain.HistoryEntry, error)
	VideoHistory(ctx context.Context, videoID string) ([]domain.HistoryEntry, error)
	ActivityLog(ctx context.Context, limit int) ([]domain.ActivityEntry, error)
	VideoPosition(ctx context.Context, videoID string) (int64, bool)
	WatchedVideoIDs(ctx context.Context) (map[string]bool, error)
	HiddenRecVideoIDs(ctx context.Context) (map[string]bool, error)
	AllVideoPositions(ctx context.Context) (map[string]int64, error)
	SearchQueries(ctx context.Context, limit int) ([]string, error)
	GetVideoDetailsCache(ctx context.Context, videoID string) (domain.CachedDetails, bool, error)
	GetChannelVideos(ctx context.Context, channelID string) ([]domain.Video, error)
	GetAllChannelVideos(ctx context.Context, channelIDs []string) ([]domain.Video, error)
	GetChannelLatestAll(ctx context.Context) (map[string]domain.Video, error)
	ChannelHideStats(ctx context.Context, channelID string) (hidden, played int, err error)
	WatchLater(ctx context.Context) ([]domain.WatchLaterEntry, error)
	HasLocalVideo(ctx context.Context, videoID string) (domain.LocalVideo, bool)
	GetSubscribedChannels(ctx context.Context) ([]domain.Channel, error)
	GetYTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error)
	GetYTPlaylistVideos(ctx context.Context, playlistID string) ([]domain.Video, error)
	GetFeedCache(ctx context.Context, feed string) ([]domain.Video, error)
	PlaylistVideoIDs(ctx context.Context, playlistID int64) ([]string, error)

	// ── Mutations ──────────────────────────────────────────────────────────────
	UpsertVideo(ctx context.Context, id, title, channel, channelID string, duration int, viewCount int64, uploadDate, url string) error
	AddLocalVideo(ctx context.Context, v domain.LocalVideo) error
	SetVideoStatus(ctx context.Context, id string, status domain.VideoStatus) error
	DeleteLocalVideo(ctx context.Context, id string) error
	SaveVideoPosition(ctx context.Context, videoID string, ms int64) error
	DeleteVideoPosition(ctx context.Context, videoID string) error
	UpdateLastPosition(ctx context.Context, id string, ms int64) error
	AddHistory(ctx context.Context, videoID, eventType, details string) error
	DeleteVideoHistory(ctx context.Context, videoID string) error
	DeleteSearchHistory(ctx context.Context, query string) error
	ClearHistory(ctx context.Context) error
	LogActivity(ctx context.Context, e domain.ActivityEntry) error
	HideRecVideo(ctx context.Context, videoID string) error
	AddSubscribedChannel(ctx context.Context, ch domain.Channel) error
	SaveSubscribedChannels(ctx context.Context, channels []domain.Channel) error
	RemoveSubscribedChannel(ctx context.Context, channelID string) error
	DeleteChannelVideos(ctx context.Context, channelID string) error
	SetChannelAlias(ctx context.Context, channelID, alias string) error
	SetChannelTags(ctx context.Context, channelID string, tags []string) error
	SaveChannelVideos(ctx context.Context, channelID string, videos []domain.Video) error
	CreatePlaylist(ctx context.Context, name string) (int64, error)
	DeletePlaylist(ctx context.Context, id int64) error
	AddToPlaylist(ctx context.Context, playlistID int64, videoID string) error
	RemoveFromPlaylist(ctx context.Context, playlistID int64, videoID string) error
	AddWatchLater(ctx context.Context, id, title, channel, url string) error
	RemoveWatchLater(ctx context.Context, id string) error
	SaveYTPlaylists(ctx context.Context, playlists []domain.YTPlaylist) error
	SaveYTPlaylistVideos(ctx context.Context, playlistID string, videos []domain.Video) error
	SaveVideoDetailsCache(ctx context.Context, videoID, description, thumbnailURL string, subscribers int64) error
	SaveVideoChapters(ctx context.Context, videoID string, chapters []domain.Chapter) error
	SaveVideoSBSegments(ctx context.Context, videoID string, segs []domain.SBSegment) error
	SaveVideoLinks(ctx context.Context, videoID string, links []domain.Link) error
	ClearRecommended(ctx context.Context) error
	ClearVideoDetailsCache(ctx context.Context) error
	ClearDownloads(ctx context.Context) ([]string, error)
	PurgeFeedCacheMissingChannelID(ctx context.Context, feed string) error
	SaveFeedCache(ctx context.Context, feed string, videos []domain.Video) error

	// ── Download queue ─────────────────────────────────────────────────────────
	Enqueue(ctx context.Context, video domain.Video, audioOnly bool) error
	CancelDownload(ctx context.Context, videoID string) error
	Events(ctx context.Context) (<-chan Event, error)

	// ── Channel subscription (local or remote, decided by ch.IsLocal / channelID lookup) ──
	Subscribe(ctx context.Context, ch domain.Channel) error
	Unsubscribe(ctx context.Context, ch domain.Channel) error

	// ── YouTube API mutations (require browser-cookie auth) ────────────────────
	InitYTClient(ctx context.Context) error
	CreateYTPlaylist(ctx context.Context, name string) (id string, err error)
	DeleteYTPlaylist(ctx context.Context, playlistID string) error
	AddToYTPlaylist(ctx context.Context, playlistID, videoID string) error
	RemoveFromYTPlaylist(ctx context.Context, playlistID, videoID string) error
}
