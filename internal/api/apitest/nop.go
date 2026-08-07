// Package apitest provides shared test doubles for the api.Backend contract so
// consumers (the api transport tests and the TUI tab tests) can embed one
// zero-value fake and override only the handful of methods under test, instead
// of each re-declaring the full interface. Each role gets its own Nop struct
// mirroring api's FeedBackend/ChannelBackend/.../DownloadBackend split, so a
// test that only needs e.g. HistoryBackend can embed NopHistoryBackend
// directly instead of pulling in the full ~80-method NopBackend.
package apitest

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
)

// ── FeedBackend ──────────────────────────────────────────────────────────────

// NopFeedBackend implements api.FeedBackend with every method returning zero values.
type NopFeedBackend struct{}

var _ api.FeedBackend = (*NopFeedBackend)(nil)

func (NopFeedBackend) Recommended(context.Context) ([]domain.Video, error) { return nil, nil }
func (NopFeedBackend) GetFeedCache(_ context.Context, _ string) ([]domain.Video, error) {
	return nil, nil
}
func (NopFeedBackend) SaveFeedCache(_ context.Context, _ string, _ []domain.Video) error { return nil }
func (NopFeedBackend) PurgeFeedCacheMissingChannelID(_ context.Context, _ string) error  { return nil }
func (NopFeedBackend) HideRecVideo(_ context.Context, _ string) error                    { return nil }
func (NopFeedBackend) HiddenRecVideoIDs(context.Context) (map[string]bool, error)        { return nil, nil }
func (NopFeedBackend) WatchedVideoIDs(context.Context) (map[string]bool, error)          { return nil, nil }
func (NopFeedBackend) ClearRecommended(context.Context) error                            { return nil }

// ── ChannelBackend ───────────────────────────────────────────────────────────

// NopChannelBackend implements api.ChannelBackend with every method returning zero values.
type NopChannelBackend struct{}

var _ api.ChannelBackend = (*NopChannelBackend)(nil)

func (NopChannelBackend) Search(_ context.Context, _ string) ([]domain.Channel, []domain.Video, error) {
	return nil, nil, nil
}
func (NopChannelBackend) ChannelVideos(_ context.Context, _, _ string) ([]domain.Video, error) {
	return nil, nil
}
func (NopChannelBackend) ChannelLatestN(_ context.Context, _, _ string, _ int) ([]domain.Video, error) {
	return nil, nil
}
func (NopChannelBackend) SubscribedChannels(context.Context) ([]domain.Channel, error) {
	return nil, nil
}
func (NopChannelBackend) GetSubscribedChannels(context.Context) ([]domain.Channel, error) {
	return nil, nil
}
func (NopChannelBackend) AllChannels(context.Context) ([]domain.Channel, error) {
	return nil, nil
}
func (NopChannelBackend) BlockedChannels(context.Context) ([]domain.Channel, error) {
	return nil, nil
}
func (NopChannelBackend) GetChannelVideos(_ context.Context, _ string) ([]domain.Video, error) {
	return nil, nil
}
func (NopChannelBackend) GetAllChannelVideos(_ context.Context, _ []string) ([]domain.Video, error) {
	return nil, nil
}
func (NopChannelBackend) GetChannelLatestAll(context.Context) (map[string]domain.Video, error) {
	return nil, nil
}
func (NopChannelBackend) ChannelHideStats(_ context.Context, _ string) (int, int, error) {
	return 0, 0, nil
}
func (NopChannelBackend) Subscribe(_ context.Context, _ domain.Channel) error   { return nil }
func (NopChannelBackend) Unsubscribe(_ context.Context, _ domain.Channel) error { return nil }
func (NopChannelBackend) BlockChannel(_ context.Context, _ domain.Channel) error {
	return nil
}
func (NopChannelBackend) UnblockChannel(_ context.Context, _ string) error { return nil }
func (NopChannelBackend) SetChannelState(_ context.Context, _ string, _ domain.SubscriptionState) error {
	return nil
}
func (NopChannelBackend) AddSubscribedChannel(_ context.Context, _ domain.Channel) error {
	return nil
}
func (NopChannelBackend) SaveSubscribedChannels(_ context.Context, _ []domain.Channel) error {
	return nil
}
func (NopChannelBackend) RemoveSubscribedChannel(_ context.Context, _ string) error { return nil }
func (NopChannelBackend) DeleteChannelVideos(_ context.Context, _ string) error     { return nil }
func (NopChannelBackend) SetChannelAlias(_ context.Context, _, _ string) error      { return nil }
func (NopChannelBackend) SetChannelTags(_ context.Context, _ string, _ []string) error {
	return nil
}
func (NopChannelBackend) SaveChannelVideos(_ context.Context, _ string, _ []domain.Video) error {
	return nil
}

// ── VideoBackend ─────────────────────────────────────────────────────────────

// NopVideoBackend implements api.VideoBackend with every method returning zero values.
type NopVideoBackend struct{}

var _ api.VideoBackend = (*NopVideoBackend)(nil)

func (NopVideoBackend) VideoDetails(_ context.Context, _ string) (domain.VideoDetails, error) {
	return domain.VideoDetails{}, nil
}
func (NopVideoBackend) GetVideoDetailsCache(_ context.Context, _ string) (domain.CachedDetails, bool, error) {
	return domain.CachedDetails{}, false, nil
}
func (NopVideoBackend) HasLocalVideo(_ context.Context, _ string) (domain.LocalVideo, bool, error) {
	return domain.LocalVideo{}, false, nil
}
func (NopVideoBackend) VideoPosition(_ context.Context, _ string) (int64, bool, error) {
	return 0, false, nil
}
func (NopVideoBackend) AllVideoPositions(context.Context) (map[string]int64, error) {
	return nil, nil
}
func (NopVideoBackend) UpsertVideo(_ context.Context, _, _, _, _ string, _ int, _ int64, _, _ string) error {
	return nil
}
func (NopVideoBackend) SetVideoStatus(_ context.Context, _ string, _ domain.VideoStatus) error {
	return nil
}
func (NopVideoBackend) SaveVideoPosition(_ context.Context, _ string, _ int64) error  { return nil }
func (NopVideoBackend) DeleteVideoPosition(_ context.Context, _ string) error         { return nil }
func (NopVideoBackend) UpdateLastPosition(_ context.Context, _ string, _ int64) error { return nil }
func (NopVideoBackend) SaveVideoDetailsCache(_ context.Context, _, _, _ string, _ int64) error {
	return nil
}
func (NopVideoBackend) SaveVideoChapters(_ context.Context, _ string, _ []domain.Chapter) error {
	return nil
}
func (NopVideoBackend) SaveVideoSBSegments(_ context.Context, _ string, _ []domain.SBSegment) error {
	return nil
}
func (NopVideoBackend) SaveVideoLinks(_ context.Context, _ string, _ []domain.Link) error {
	return nil
}
func (NopVideoBackend) ClearVideoDetailsCache(context.Context) error            { return nil }
func (NopVideoBackend) DeleteVideoCompletely(_ context.Context, _ string) error { return nil }
func (NopVideoBackend) ResolveSource(_ context.Context, _, fallback string) (api.PlayableSource, error) {
	return api.PlayableSource{URI: fallback}, nil
}
func (NopVideoBackend) GetThumbnail(_ context.Context, _, _ string) ([]byte, bool, error) {
	return nil, false, nil
}
func (NopVideoBackend) GetTranscript(_ context.Context, _, _ string) (string, bool, error) {
	return "", false, nil
}
func (NopVideoBackend) EligibleThumbnailIDs(context.Context) (map[string]bool, error) {
	return nil, nil
}

// ── LibraryBackend ───────────────────────────────────────────────────────────

// NopLibraryBackend implements api.LibraryBackend with every method returning zero values.
type NopLibraryBackend struct{}

var _ api.LibraryBackend = (*NopLibraryBackend)(nil)

func (NopLibraryBackend) LocalVideos(context.Context) ([]domain.LocalVideo, error) { return nil, nil }
func (NopLibraryBackend) HasLocalVideo(_ context.Context, _ string) (domain.LocalVideo, bool, error) {
	return domain.LocalVideo{}, false, nil
}
func (NopLibraryBackend) AddLocalVideo(_ context.Context, _ domain.LocalVideo) error { return nil }
func (NopLibraryBackend) DeleteLocalVideo(_ context.Context, _ string) error         { return nil }
func (NopLibraryBackend) DeleteAllLocalFiles(context.Context) (int, error)           { return 0, nil }

// ── PlaylistBackend ──────────────────────────────────────────────────────────

// NopPlaylistBackend implements api.PlaylistBackend with every method returning zero values.
type NopPlaylistBackend struct{}

var _ api.PlaylistBackend = (*NopPlaylistBackend)(nil)

func (NopPlaylistBackend) LocalPlaylists(context.Context) ([]domain.Playlist, error) {
	return nil, nil
}
func (NopPlaylistBackend) LocalPlaylistVideos(_ context.Context, _ int64) ([]domain.Video, error) {
	return nil, nil
}
func (NopPlaylistBackend) PlaylistVideoIDs(_ context.Context, _ int64) ([]string, error) {
	return nil, nil
}
func (NopPlaylistBackend) CreatePlaylist(_ context.Context, _ string) (int64, error) { return 0, nil }
func (NopPlaylistBackend) DeletePlaylist(_ context.Context, _ int64) error           { return nil }
func (NopPlaylistBackend) AddToPlaylist(_ context.Context, _ int64, _ string) error  { return nil }
func (NopPlaylistBackend) RemoveFromPlaylist(_ context.Context, _ int64, _ string) error {
	return nil
}
func (NopPlaylistBackend) WatchLater(context.Context) ([]domain.WatchLaterEntry, error) {
	return nil, nil
}
func (NopPlaylistBackend) AddWatchLater(_ context.Context, _, _, _, _ string) error { return nil }
func (NopPlaylistBackend) RemoveWatchLater(_ context.Context, _ string) error       { return nil }
func (NopPlaylistBackend) YTPlaylists(context.Context) ([]domain.YTPlaylist, error) { return nil, nil }
func (NopPlaylistBackend) YTPlaylistVideos(_ context.Context, _ string) ([]domain.Video, error) {
	return nil, nil
}
func (NopPlaylistBackend) GetYTPlaylists(context.Context) ([]domain.YTPlaylist, error) {
	return nil, nil
}
func (NopPlaylistBackend) GetYTPlaylistVideos(_ context.Context, _ string) ([]domain.Video, error) {
	return nil, nil
}
func (NopPlaylistBackend) SaveYTPlaylists(_ context.Context, _ []domain.YTPlaylist) error {
	return nil
}
func (NopPlaylistBackend) SaveYTPlaylistVideos(_ context.Context, _ string, _ []domain.Video) error {
	return nil
}
func (NopPlaylistBackend) InitYTClient(context.Context) error { return nil }
func (NopPlaylistBackend) CreateYTPlaylist(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (NopPlaylistBackend) DeleteYTPlaylist(_ context.Context, _ string) error        { return nil }
func (NopPlaylistBackend) AddToYTPlaylist(_ context.Context, _, _ string) error      { return nil }
func (NopPlaylistBackend) RemoveFromYTPlaylist(_ context.Context, _, _ string) error { return nil }

// ── HistoryBackend ───────────────────────────────────────────────────────────

// NopHistoryBackend implements api.HistoryBackend with every method returning zero values.
type NopHistoryBackend struct{}

var _ api.HistoryBackend = (*NopHistoryBackend)(nil)

func (NopHistoryBackend) History(_ context.Context, _ int) ([]domain.HistoryEntry, error) {
	return nil, nil
}
func (NopHistoryBackend) HistoryVideos(_ context.Context, _ int) ([]domain.HistoryEntry, error) {
	return nil, nil
}
func (NopHistoryBackend) VideoHistory(_ context.Context, _ string) ([]domain.HistoryEntry, error) {
	return nil, nil
}
func (NopHistoryBackend) ActivityLog(_ context.Context, _ int) ([]domain.ActivityEntry, error) {
	return nil, nil
}
func (NopHistoryBackend) SearchQueries(context.Context) ([]string, error)    { return nil, nil }
func (NopHistoryBackend) AddHistory(_ context.Context, _, _, _ string) error { return nil }
func (NopHistoryBackend) LogActivity(_ context.Context, _ domain.ActivityEntry) error {
	return nil
}
func (NopHistoryBackend) DeleteVideoHistory(_ context.Context, _ string) error  { return nil }
func (NopHistoryBackend) DeleteSearchHistory(_ context.Context, _ string) error { return nil }
func (NopHistoryBackend) ClearHistory(context.Context) error                    { return nil }

// ── DownloadBackend ──────────────────────────────────────────────────────────

// NopDownloadBackend implements api.DownloadBackend with every method returning zero values.
type NopDownloadBackend struct{}

var _ api.DownloadBackend = (*NopDownloadBackend)(nil)

func (NopDownloadBackend) Enqueue(_ context.Context, _ domain.Video, _ bool) error { return nil }
func (NopDownloadBackend) CancelDownload(_ context.Context, _ string) error        { return nil }
func (NopDownloadBackend) DownloadItems(context.Context) ([]api.DownloadItem, error) {
	return nil, nil
}
func (NopDownloadBackend) ClearDownloads(context.Context) error { return nil }
func (NopDownloadBackend) Events(context.Context) (<-chan api.Event, error) {
	ch := make(chan api.Event)
	close(ch)
	return ch, nil
}

// ── PortabilityBackend ─────────────────────────────────────────────────────────

// NopPortabilityBackend implements api.PortabilityBackend with every method returning zero values.
type NopPortabilityBackend struct{}

var _ api.PortabilityBackend = (*NopPortabilityBackend)(nil)

func (NopPortabilityBackend) Export(_ context.Context, _ portability.ExportOptions) (portability.Bundle, error) {
	return portability.Bundle{}, nil
}

func (NopPortabilityBackend) ImportPreview(_ context.Context, _ portability.Bundle, _ portability.ImportOptions) (portability.ImportPlan, error) {
	return portability.ImportPlan{}, nil
}

func (NopPortabilityBackend) ImportApply(_ context.Context, _ portability.Bundle, _ portability.ImportOptions) (portability.ImportResult, error) {
	return portability.ImportResult{}, nil
}

// ── ProfileBackend ─────────────────────────────────────────────────────────────

// NopProfileBackend implements api.ProfileBackend with every method returning zero values.
type NopProfileBackend struct{}

var _ api.ProfileBackend = (*NopProfileBackend)(nil)

func (NopProfileBackend) ListProfiles(context.Context) ([]string, error) { return nil, nil }
func (NopProfileBackend) GetProfile(_ context.Context, _ string) ([]byte, bool, error) {
	return nil, false, nil
}
func (NopProfileBackend) SaveProfile(_ context.Context, _ string, _ []byte) error { return nil }

// ── StatusBackend ──────────────────────────────────────────────────────────────

// NopStatusBackend implements api.StatusBackend, reporting a healthy environment.
type NopStatusBackend struct{}

var _ api.StatusBackend = (*NopStatusBackend)(nil)

func (NopStatusBackend) CheckAvailability(context.Context) ([]config.ConfigIssue, error) {
	return nil, nil
}
func (NopStatusBackend) Capabilities(context.Context) (api.Capabilities, error) {
	return api.Capabilities{}, nil
}

// ── NopBackend ───────────────────────────────────────────────────────────────

// NopBackend implements api.Backend with every method returning zero values,
// composed from the per-role Nop structs above. Embed it in a test struct and
// override only the methods under test.
type NopBackend struct {
	NopFeedBackend
	NopChannelBackend
	NopVideoBackend
	NopLibraryBackend
	NopPlaylistBackend
	NopHistoryBackend
	NopPortabilityBackend
	NopProfileBackend
	NopStatusBackend
	NopDownloadBackend
}

var _ api.Backend = (*NopBackend)(nil)

// HasLocalVideo is declared on both NopVideoBackend and NopLibraryBackend
// (mirroring api.VideoBackend/api.LibraryBackend, which both need it); Go
// treats that as an ambiguous promoted selector on NopBackend, so it must be
// resolved explicitly here for NopBackend to satisfy api.Backend.
func (n NopBackend) HasLocalVideo(ctx context.Context, videoID string) (domain.LocalVideo, bool, error) {
	return n.NopVideoBackend.HasLocalVideo(ctx, videoID)
}
