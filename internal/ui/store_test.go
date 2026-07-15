package ui

import (
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// Compile-time assertion: *db.DB must satisfy Store.
var _ Store = (*db.DB)(nil)

// fakeStore is a no-op Store for use in tests.
type fakeStore struct{}

func (f *fakeStore) GetSubscribedChannels() ([]domain.Channel, error) { return nil, nil }
func (f *fakeStore) AddSubscribedChannel(domain.Channel) error        { return nil }
func (f *fakeStore) RemoveSubscribedChannel(string) error             { return nil }
func (f *fakeStore) SaveSubscribedChannels([]domain.Channel) error    { return nil }
func (f *fakeStore) SetChannelAlias(string, string) error             { return nil }
func (f *fakeStore) SetChannelTags(string, []string) error            { return nil }
func (f *fakeStore) GetChannelVideos(string) ([]domain.Video, error)  { return nil, nil }
func (f *fakeStore) SaveChannelVideos(string, []domain.Video) error   { return nil }
func (f *fakeStore) DeleteChannelVideos(string) error                 { return nil }
func (f *fakeStore) GetAllChannelVideos([]string) ([]domain.Video, error) {
	return nil, nil
}
func (f *fakeStore) GetChannelLatestAll() (map[string]domain.Video, error) {
	return nil, nil
}
func (f *fakeStore) ChannelHideStats(string) (int, int, error) { return 0, 0, nil }
func (f *fakeStore) UpsertVideo(string, string, string, string, int, int64, string, string) error {
	return nil
}
func (f *fakeStore) SetVideoStatus(string, domain.VideoStatus) error           { return nil }
func (f *fakeStore) DeleteLocalVideo(string) error                             { return nil }
func (f *fakeStore) LocalVideos() ([]domain.LocalVideo, error)                 { return nil, nil }
func (f *fakeStore) SaveVideoPosition(string, int64) error                     { return nil }
func (f *fakeStore) DeleteVideoPosition(string) error                          { return nil }
func (f *fakeStore) VideoPosition(string) (int64, bool)                        { return 0, false }
func (f *fakeStore) AllVideoPositions() (map[string]int64, error)              { return nil, nil }
func (f *fakeStore) WatchedVideoIDs() (map[string]bool, error)                 { return nil, nil }
func (f *fakeStore) AddHistory(string, string, string) error                   { return nil }
func (f *fakeStore) SearchQueries(int) ([]string, error)                       { return nil, nil }
func (f *fakeStore) HistoryVideos(int) ([]domain.HistoryEntry, error)          { return nil, nil }
func (f *fakeStore) DeleteVideoHistory(string) error                           { return nil }
func (f *fakeStore) DeleteSearchHistory(string) error                          { return nil }
func (f *fakeStore) VideoHistory(string) ([]domain.HistoryEntry, error)        { return nil, nil }
func (f *fakeStore) LogActivity(domain.ActivityEntry) error                    { return nil }
func (f *fakeStore) GetActivityLog(int) ([]domain.ActivityEntry, error)        { return nil, nil }
func (f *fakeStore) SaveYTPlaylists([]domain.YTPlaylist) error                 { return nil }
func (f *fakeStore) GetYTPlaylists() ([]domain.YTPlaylist, error)              { return nil, nil }
func (f *fakeStore) SaveYTPlaylistVideos(string, []domain.Video) error         { return nil }
func (f *fakeStore) GetYTPlaylistVideos(string) ([]domain.Video, error)        { return nil, nil }
func (f *fakeStore) Playlists() ([]domain.Playlist, error)                     { return nil, nil }
func (f *fakeStore) CreatePlaylist(string) (int64, error)                      { return 0, nil }
func (f *fakeStore) DeletePlaylist(int64) error                                { return nil }
func (f *fakeStore) AddToPlaylist(int64, string) error                         { return nil }
func (f *fakeStore) RemoveFromPlaylist(int64, string) error                    { return nil }
func (f *fakeStore) PlaylistVideos(int64) ([]domain.Video, error)              { return nil, nil }
func (f *fakeStore) SaveFeedCache(string, []domain.Video) error                { return nil }
func (f *fakeStore) GetFeedCache(string) ([]domain.Video, error)               { return nil, nil }
func (f *fakeStore) PurgeFeedCacheMissingChannelID(string) error               { return nil }
func (f *fakeStore) HideRecVideo(string) error                                 { return nil }
func (f *fakeStore) HiddenRecVideoIDs() (map[string]bool, error)               { return nil, nil }
func (f *fakeStore) ClearRecommended() error                                   { return nil }
func (f *fakeStore) SaveVideoDetailsCache(string, string, string, int64) error { return nil }
func (f *fakeStore) GetVideoDetailsCache(string) (domain.CachedDetails, bool, error) {
	return domain.CachedDetails{}, false, nil
}
func (f *fakeStore) ClearVideoDetailsCache() error                        { return nil }
func (f *fakeStore) SaveVideoChapters(string, []domain.Chapter) error     { return nil }
func (f *fakeStore) SaveVideoSBSegments(string, []domain.SBSegment) error { return nil }
func (f *fakeStore) SaveVideoLinks(string, []domain.Link) error           { return nil }
func (f *fakeStore) ClearHistory() error                                  { return nil }
func (f *fakeStore) ClearDownloads() ([]string, error)                    { return nil, nil }

// Compile-time assertion: fakeStore satisfies Store.
var _ Store = (*fakeStore)(nil)
