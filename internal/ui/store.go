package ui

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// Store is the persistence interface used by Model views and commands.
// It is a context-free subset of api.Backend, satisfied by backendStore.
type Store interface {
	// channels / subscriptions
	GetSubscribedChannels() ([]domain.Channel, error)
	AddSubscribedChannel(ch domain.Channel) error
	RemoveSubscribedChannel(channelID string) error
	SaveSubscribedChannels(channels []domain.Channel) error
	SetChannelAlias(channelID, alias string) error
	SetChannelTags(channelID string, tags []string) error
	GetChannelVideos(channelID string) ([]domain.Video, error)
	SaveChannelVideos(channelID string, videos []domain.Video) error
	DeleteChannelVideos(channelID string) error
	GetAllChannelVideos(channelIDs []string) ([]domain.Video, error)
	GetChannelLatestAll() (map[string]domain.Video, error)
	ChannelHideStats(channelID string) (hidden, played int, err error)

	// local videos
	UpsertVideo(id, title, channel, channelID string, duration int, viewCount int64, uploadDate, url string) error
	SetVideoStatus(id string, status domain.VideoStatus) error
	DeleteLocalVideo(id string) error
	LocalVideos() ([]domain.LocalVideo, error)
	SaveVideoPosition(videoID string, ms int64) error
	DeleteVideoPosition(videoID string) error
	VideoPosition(videoID string) (int64, bool)
	AllVideoPositions() (map[string]int64, error)
	WatchedVideoIDs() (map[string]bool, error)

	// history / activity
	AddHistory(videoID, eventType, details string) error
	SearchQueries(limit int) ([]string, error)
	HistoryVideos(limit int) ([]domain.HistoryEntry, error)
	DeleteVideoHistory(videoID string) error
	DeleteSearchHistory(query string) error
	VideoHistory(videoID string) ([]domain.HistoryEntry, error)
	LogActivity(e domain.ActivityEntry) error
	GetActivityLog(limit int) ([]domain.ActivityEntry, error)

	// playlists
	SaveYTPlaylists(playlists []domain.YTPlaylist) error
	GetYTPlaylists() ([]domain.YTPlaylist, error)
	SaveYTPlaylistVideos(playlistID string, videos []domain.Video) error
	GetYTPlaylistVideos(playlistID string) ([]domain.Video, error)
	Playlists() ([]domain.Playlist, error)
	CreatePlaylist(name string) (int64, error)
	DeletePlaylist(id int64) error
	AddToPlaylist(playlistID int64, videoID string) error
	RemoveFromPlaylist(playlistID int64, videoID string) error
	PlaylistVideos(playlistID int64) ([]domain.Video, error)

	// recommended feed / detail cache
	SaveFeedCache(feed string, videos []domain.Video) error
	GetFeedCache(feed string) ([]domain.Video, error)
	PurgeFeedCacheMissingChannelID(feed string) error
	HideRecVideo(videoID string) error
	HiddenRecVideoIDs() (map[string]bool, error)
	ClearRecommended() error

	// video detail cache
	SaveVideoDetailsCache(videoID, description, thumbnailURL string, subscribers int64) error
	GetVideoDetailsCache(videoID string) (domain.CachedDetails, bool, error)
	ClearVideoDetailsCache() error
	SaveVideoChapters(videoID string, chapters []domain.Chapter) error
	SaveVideoSBSegments(videoID string, segs []domain.SBSegment) error
	SaveVideoLinks(videoID string, links []domain.Link) error

	// bulk clear
	ClearHistory() error
	ClearDownloads() ([]string, error)
}

// backendStore wraps api.Backend and strips the context.Context parameter,
// satisfying the Store interface used by views and commands.
type backendStore struct {
	b api.Backend
}

var bg = context.Background

func (s backendStore) GetSubscribedChannels() ([]domain.Channel, error) {
	return s.b.GetSubscribedChannels(bg())
}
func (s backendStore) AddSubscribedChannel(ch domain.Channel) error {
	return s.b.AddSubscribedChannel(bg(), ch)
}
func (s backendStore) RemoveSubscribedChannel(channelID string) error {
	return s.b.RemoveSubscribedChannel(bg(), channelID)
}
func (s backendStore) SaveSubscribedChannels(channels []domain.Channel) error {
	return s.b.SaveSubscribedChannels(bg(), channels)
}
func (s backendStore) SetChannelAlias(channelID, alias string) error {
	return s.b.SetChannelAlias(bg(), channelID, alias)
}
func (s backendStore) SetChannelTags(channelID string, tags []string) error {
	return s.b.SetChannelTags(bg(), channelID, tags)
}
func (s backendStore) GetChannelVideos(channelID string) ([]domain.Video, error) {
	return s.b.GetChannelVideos(bg(), channelID)
}
func (s backendStore) SaveChannelVideos(channelID string, videos []domain.Video) error {
	return s.b.SaveChannelVideos(bg(), channelID, videos)
}
func (s backendStore) DeleteChannelVideos(channelID string) error {
	return s.b.DeleteChannelVideos(bg(), channelID)
}
func (s backendStore) GetAllChannelVideos(channelIDs []string) ([]domain.Video, error) {
	return s.b.GetAllChannelVideos(bg(), channelIDs)
}
func (s backendStore) GetChannelLatestAll() (map[string]domain.Video, error) {
	return s.b.GetChannelLatestAll(bg())
}
func (s backendStore) ChannelHideStats(channelID string) (int, int, error) {
	return s.b.ChannelHideStats(bg(), channelID)
}
func (s backendStore) UpsertVideo(id, title, channel, channelID string, duration int, viewCount int64, uploadDate, url string) error {
	return s.b.UpsertVideo(bg(), id, title, channel, channelID, duration, viewCount, uploadDate, url)
}
func (s backendStore) SetVideoStatus(id string, status domain.VideoStatus) error {
	return s.b.SetVideoStatus(bg(), id, status)
}
func (s backendStore) DeleteLocalVideo(id string) error {
	return s.b.DeleteLocalVideo(bg(), id)
}
func (s backendStore) LocalVideos() ([]domain.LocalVideo, error) {
	return s.b.LocalVideos(bg())
}
func (s backendStore) SaveVideoPosition(videoID string, ms int64) error {
	return s.b.SaveVideoPosition(bg(), videoID, ms)
}
func (s backendStore) DeleteVideoPosition(videoID string) error {
	return s.b.DeleteVideoPosition(bg(), videoID)
}
func (s backendStore) VideoPosition(videoID string) (int64, bool) {
	return s.b.VideoPosition(bg(), videoID)
}
func (s backendStore) AllVideoPositions() (map[string]int64, error) {
	return s.b.AllVideoPositions(bg())
}
func (s backendStore) WatchedVideoIDs() (map[string]bool, error) {
	return s.b.WatchedVideoIDs(bg())
}
func (s backendStore) AddHistory(videoID, eventType, details string) error {
	return s.b.AddHistory(bg(), videoID, eventType, details)
}
func (s backendStore) SearchQueries(limit int) ([]string, error) {
	return s.b.SearchQueries(bg(), limit)
}
func (s backendStore) HistoryVideos(limit int) ([]domain.HistoryEntry, error) {
	return s.b.HistoryVideos(bg(), limit)
}
func (s backendStore) DeleteVideoHistory(videoID string) error {
	return s.b.DeleteVideoHistory(bg(), videoID)
}
func (s backendStore) DeleteSearchHistory(query string) error {
	return s.b.DeleteSearchHistory(bg(), query)
}
func (s backendStore) VideoHistory(videoID string) ([]domain.HistoryEntry, error) {
	return s.b.VideoHistory(bg(), videoID)
}
func (s backendStore) LogActivity(e domain.ActivityEntry) error {
	return s.b.LogActivity(bg(), e)
}
func (s backendStore) GetActivityLog(limit int) ([]domain.ActivityEntry, error) {
	return s.b.ActivityLog(bg(), limit)
}
func (s backendStore) SaveYTPlaylists(playlists []domain.YTPlaylist) error {
	return s.b.SaveYTPlaylists(bg(), playlists)
}
func (s backendStore) GetYTPlaylists() ([]domain.YTPlaylist, error) {
	return s.b.GetYTPlaylists(bg())
}
func (s backendStore) SaveYTPlaylistVideos(playlistID string, videos []domain.Video) error {
	return s.b.SaveYTPlaylistVideos(bg(), playlistID, videos)
}
func (s backendStore) GetYTPlaylistVideos(playlistID string) ([]domain.Video, error) {
	return s.b.GetYTPlaylistVideos(bg(), playlistID)
}
func (s backendStore) Playlists() ([]domain.Playlist, error) {
	return s.b.LocalPlaylists(bg())
}
func (s backendStore) CreatePlaylist(name string) (int64, error) {
	return s.b.CreatePlaylist(bg(), name)
}
func (s backendStore) DeletePlaylist(id int64) error {
	return s.b.DeletePlaylist(bg(), id)
}
func (s backendStore) AddToPlaylist(playlistID int64, videoID string) error {
	return s.b.AddToPlaylist(bg(), playlistID, videoID)
}
func (s backendStore) RemoveFromPlaylist(playlistID int64, videoID string) error {
	return s.b.RemoveFromPlaylist(bg(), playlistID, videoID)
}
func (s backendStore) PlaylistVideos(playlistID int64) ([]domain.Video, error) {
	return s.b.LocalPlaylistVideos(bg(), playlistID)
}
func (s backendStore) SaveFeedCache(feed string, videos []domain.Video) error {
	return s.b.SaveFeedCache(bg(), feed, videos)
}
func (s backendStore) GetFeedCache(feed string) ([]domain.Video, error) {
	return s.b.GetFeedCache(bg(), feed)
}
func (s backendStore) PurgeFeedCacheMissingChannelID(feed string) error {
	return s.b.PurgeFeedCacheMissingChannelID(bg(), feed)
}
func (s backendStore) HideRecVideo(videoID string) error {
	return s.b.HideRecVideo(bg(), videoID)
}
func (s backendStore) HiddenRecVideoIDs() (map[string]bool, error) {
	return s.b.HiddenRecVideoIDs(bg())
}
func (s backendStore) ClearRecommended() error {
	return s.b.ClearRecommended(bg())
}
func (s backendStore) SaveVideoDetailsCache(videoID, description, thumbnailURL string, subscribers int64) error {
	return s.b.SaveVideoDetailsCache(bg(), videoID, description, thumbnailURL, subscribers)
}
func (s backendStore) GetVideoDetailsCache(videoID string) (domain.CachedDetails, bool, error) {
	return s.b.GetVideoDetailsCache(bg(), videoID)
}
func (s backendStore) ClearVideoDetailsCache() error {
	return s.b.ClearVideoDetailsCache(bg())
}
func (s backendStore) SaveVideoChapters(videoID string, chapters []domain.Chapter) error {
	return s.b.SaveVideoChapters(bg(), videoID, chapters)
}
func (s backendStore) SaveVideoSBSegments(videoID string, segs []domain.SBSegment) error {
	return s.b.SaveVideoSBSegments(bg(), videoID, segs)
}
func (s backendStore) SaveVideoLinks(videoID string, links []domain.Link) error {
	return s.b.SaveVideoLinks(bg(), videoID, links)
}
func (s backendStore) ClearHistory() error {
	return s.b.ClearHistory(bg())
}
func (s backendStore) ClearDownloads() ([]string, error) {
	return s.b.ClearDownloads(bg())
}
