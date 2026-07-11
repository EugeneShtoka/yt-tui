package ui

import (
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

// Store is the persistence interface used by Model. *db.DB satisfies it.
// Keeping it as a single interface here matches the current single-consumer
// (Model); P4 sub-views can accept narrower slices of it when they arrive.
type Store interface {
	// channels / subscriptions
	GetSubscribedChannels() ([]youtube.Channel, error)
	AddSubscribedChannel(ch youtube.Channel) error
	RemoveSubscribedChannel(channelID string) error
	SaveSubscribedChannels(channels []youtube.Channel) error
	SetChannelAlias(channelID, alias string) error
	SetChannelTags(channelID string, tags []string) error
	GetChannelVideos(channelID string) ([]youtube.Video, error)
	SaveChannelVideos(channelID string, videos []youtube.Video) error
	DeleteChannelVideos(channelID string) error
	GetAllChannelVideos(channelIDs []string) ([]youtube.Video, error)
	GetChannelLatestAll() (map[string]youtube.Video, error)
	ChannelHideStats(channelID string) (hidden, played int, err error)

	// local videos
	UpsertVideo(id, title, channel, channelID string, duration int, viewCount int64, uploadDate, url string) error
	SetVideoStatus(id string, status db.VideoStatus) error
	DeleteLocalVideo(id string) error
	LocalVideos() ([]db.LocalVideo, error)
	SaveVideoPosition(videoID string, ms int64) error
	DeleteVideoPosition(videoID string) error
	VideoPosition(videoID string) (int64, bool)
	AllVideoPositions() (map[string]int64, error)
	WatchedVideoIDs() (map[string]bool, error)

	// history / activity
	AddHistory(videoID, eventType, details string) error
	SearchQueries(limit int) ([]string, error)
	HistoryVideos(limit int) ([]db.HistoryEntry, error)
	DeleteVideoHistory(videoID string) error
	DeleteSearchHistory(query string) error
	VideoHistory(videoID string) ([]db.HistoryEntry, error)
	LogActivity(e db.ActivityEntry) error
	GetActivityLog(limit int) ([]db.ActivityEntry, error)

	// playlists
	SaveYTPlaylists(playlists []youtube.YTPlaylist) error
	GetYTPlaylists() ([]youtube.YTPlaylist, error)
	SaveYTPlaylistVideos(playlistID string, videos []youtube.Video) error
	GetYTPlaylistVideos(playlistID string) ([]youtube.Video, error)
	Playlists() ([]db.Playlist, error)
	CreatePlaylist(name string) (int64, error)
	DeletePlaylist(id int64) error
	AddToPlaylist(playlistID int64, videoID string) error
	RemoveFromPlaylist(playlistID int64, videoID string) error
	PlaylistVideos(playlistID int64) ([]youtube.Video, error)

	// recommended feed / detail cache
	SaveFeedCache(feed string, videos []youtube.Video) error
	GetFeedCache(feed string) ([]youtube.Video, error)
	PurgeFeedCacheMissingChannelID(feed string) error
	HideRecVideo(videoID string) error
	HiddenRecVideoIDs() (map[string]bool, error)
	ClearRecommended() error

	// video detail cache
	SaveVideoDetailsCache(videoID, description, thumbnailURL string, subscribers int64) error
	GetVideoDetailsCache(videoID string) (db.CachedDetails, bool, error)
	ClearVideoDetailsCache() error
	SaveVideoChapters(videoID string, chapters []db.Chapter) error
	SaveVideoSBSegments(videoID string, segs []db.SBSegment) error
	SaveVideoLinks(videoID string, links []db.Link) error

	// bulk clear
	ClearHistory() error
	ClearDownloads() ([]string, error)
}
