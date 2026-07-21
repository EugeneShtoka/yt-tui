//nolint:wrapcheck,gosec // Connect errors are already structured; pass through without re-wrapping. gosec G115: proto int32 fields are bounded in practice (durations, counts).
package api

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Remote implements Backend by dialing a yt-tuid daemon over Connect (HTTP/2 or HTTP/1.1).
type Remote struct {
	baseURL  string
	feed     backendv1connect.FeedServiceClient
	ch       backendv1connect.ChannelServiceClient
	vid      backendv1connect.VideoServiceClient
	lib      backendv1connect.LibraryServiceClient
	playlist backendv1connect.PlaylistServiceClient
	history  backendv1connect.HistoryServiceClient
	dl       backendv1connect.DownloadServiceClient
}

// authTransport injects a bearer token into every outbound request.
type authTransport struct {
	base  http.RoundTripper
	token string
}

func (a authTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if a.token != "" {
		r = r.Clone(r.Context())
		r.Header.Set("Authorization", "Bearer "+a.token)
	}
	return a.base.RoundTrip(r)
}

// NewRemote dials baseURL (e.g. "http://localhost:7373") with the given HTTP client and
// optional bearer token. Pass token="" for unauthenticated connections.
func NewRemote(baseURL, token string, httpClient *http.Client) *Remote {
	base := httpClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	if token != "" {
		httpClient = &http.Client{
			Transport: authTransport{base: base, token: token},
			Timeout:   httpClient.Timeout,
		}
	}
	return &Remote{
		baseURL:  baseURL,
		feed:     backendv1connect.NewFeedServiceClient(httpClient, baseURL),
		ch:       backendv1connect.NewChannelServiceClient(httpClient, baseURL),
		vid:      backendv1connect.NewVideoServiceClient(httpClient, baseURL),
		lib:      backendv1connect.NewLibraryServiceClient(httpClient, baseURL),
		playlist: backendv1connect.NewPlaylistServiceClient(httpClient, baseURL),
		history:  backendv1connect.NewHistoryServiceClient(httpClient, baseURL),
		dl:       backendv1connect.NewDownloadServiceClient(httpClient, baseURL),
	}
}

// ── YouTube fetch ─────────────────────────────────────────────────────────────

func (r *Remote) Recommended(ctx context.Context) ([]domain.Video, error) {
	resp, err := r.feed.Recommended(ctx, connect.NewRequest(&v1.RecommendedRequest{}))
	if err != nil {
		return nil, err
	}
	return rProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) SubscribedChannels(ctx context.Context) ([]domain.Channel, error) {
	resp, err := r.ch.SubscribedChannels(ctx, connect.NewRequest(&v1.SubscribedChannelsRequest{}))
	if err != nil {
		return nil, err
	}
	return rProtoToChannels(resp.Msg.Channels), nil
}

func (r *Remote) ChannelVideos(ctx context.Context, channelURL, channelID string) ([]domain.Video, error) {
	resp, err := r.ch.ChannelVideos(ctx, connect.NewRequest(&v1.ChannelVideosRequest{ChannelUrl: channelURL, ChannelId: channelID}))
	if err != nil {
		return nil, err
	}
	return rProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) ChannelLatestN(ctx context.Context, channelURL, channelID string, n int) ([]domain.Video, error) {
	resp, err := r.ch.ChannelLatestN(ctx, connect.NewRequest(&v1.ChannelLatestNRequest{ChannelUrl: channelURL, ChannelId: channelID, N: int32(n)}))
	if err != nil {
		return nil, err
	}
	return rProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) Search(ctx context.Context, query string) ([]domain.Channel, []domain.Video, error) {
	resp, err := r.ch.Search(ctx, connect.NewRequest(&v1.SearchRequest{Query: query}))
	if err != nil {
		return nil, nil, err
	}
	return rProtoToChannels(resp.Msg.Channels), rProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) YTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error) {
	resp, err := r.playlist.YTPlaylists(ctx, connect.NewRequest(&v1.YTPlaylistsRequest{}))
	if err != nil {
		return nil, err
	}
	return rProtoToYTPlaylists(resp.Msg.Playlists), nil
}

func (r *Remote) YTPlaylistVideos(ctx context.Context, playlistID string) ([]domain.Video, error) {
	resp, err := r.playlist.YTPlaylistVideos(ctx, connect.NewRequest(&v1.YTPlaylistVideosRequest{PlaylistId: playlistID}))
	if err != nil {
		return nil, err
	}
	return rProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) VideoDetails(ctx context.Context, videoURL string) (domain.VideoDetails, error) {
	resp, err := r.vid.VideoDetails(ctx, connect.NewRequest(&v1.VideoDetailsRequest{VideoUrl: videoURL}))
	if err != nil {
		return domain.VideoDetails{}, err
	}
	return rProtoToVideoDetails(resp.Msg.Details), nil
}

// ── Local DB reads ────────────────────────────────────────────────────────────

func (r *Remote) LocalVideos(ctx context.Context) ([]domain.LocalVideo, error) {
	resp, err := r.lib.LocalVideos(ctx, connect.NewRequest(&v1.LocalVideosRequest{}))
	if err != nil {
		return nil, err
	}
	out := make([]domain.LocalVideo, len(resp.Msg.Videos))
	for i, pb := range resp.Msg.Videos {
		out[i] = rProtoToLocalVideo(pb)
	}
	return out, nil
}

func (r *Remote) LocalPlaylists(ctx context.Context) ([]domain.Playlist, error) {
	resp, err := r.playlist.LocalPlaylists(ctx, connect.NewRequest(&v1.LocalPlaylistsRequest{}))
	if err != nil {
		return nil, err
	}
	out := make([]domain.Playlist, len(resp.Msg.Playlists))
	for i, pb := range resp.Msg.Playlists {
		out[i] = rProtoToPlaylist(pb)
	}
	return out, nil
}

func (r *Remote) LocalPlaylistVideos(ctx context.Context, playlistID int64) ([]domain.Video, error) {
	resp, err := r.playlist.LocalPlaylistVideos(ctx, connect.NewRequest(&v1.LocalPlaylistVideosRequest{PlaylistId: playlistID}))
	if err != nil {
		return nil, err
	}
	return rProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) History(ctx context.Context, limit int) ([]domain.HistoryEntry, error) {
	resp, err := r.history.History(ctx, connect.NewRequest(&v1.HistoryRequest{Limit: int32(limit)}))
	if err != nil {
		return nil, err
	}
	return rProtoToHistoryEntries(resp.Msg.Entries), nil
}

func (r *Remote) HistoryVideos(ctx context.Context, limit int) ([]domain.HistoryEntry, error) {
	resp, err := r.history.HistoryVideos(ctx, connect.NewRequest(&v1.HistoryVideosRequest{Limit: int32(limit)}))
	if err != nil {
		return nil, err
	}
	return rProtoToHistoryEntries(resp.Msg.Entries), nil
}

func (r *Remote) VideoHistory(ctx context.Context, videoID string) ([]domain.HistoryEntry, error) {
	resp, err := r.history.VideoHistory(ctx, connect.NewRequest(&v1.VideoHistoryRequest{VideoId: videoID}))
	if err != nil {
		return nil, err
	}
	return rProtoToHistoryEntries(resp.Msg.Entries), nil
}

func (r *Remote) ActivityLog(ctx context.Context, limit int) ([]domain.ActivityEntry, error) {
	resp, err := r.history.ActivityLog(ctx, connect.NewRequest(&v1.ActivityLogRequest{Limit: int32(limit)}))
	if err != nil {
		return nil, err
	}
	out := make([]domain.ActivityEntry, len(resp.Msg.Entries))
	for i, pb := range resp.Msg.Entries {
		out[i] = rProtoToActivityEntry(pb)
	}
	return out, nil
}

func (r *Remote) VideoPosition(ctx context.Context, videoID string) (int64, bool) {
	resp, err := r.vid.VideoPosition(ctx, connect.NewRequest(&v1.VideoPositionRequest{VideoId: videoID}))
	if err != nil {
		return 0, false
	}
	return resp.Msg.PositionMs, resp.Msg.Found
}

func (r *Remote) WatchedVideoIDs(ctx context.Context) (map[string]bool, error) {
	resp, err := r.feed.WatchedVideoIDs(ctx, connect.NewRequest(&v1.WatchedVideoIDsRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Ids, nil
}

func (r *Remote) HiddenRecVideoIDs(ctx context.Context) (map[string]bool, error) {
	resp, err := r.feed.HiddenVideoIDs(ctx, connect.NewRequest(&v1.HiddenVideoIDsRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Ids, nil
}

func (r *Remote) AllVideoPositions(ctx context.Context) (map[string]int64, error) {
	resp, err := r.vid.AllVideoPositions(ctx, connect.NewRequest(&v1.AllVideoPositionsRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Positions, nil
}

func (r *Remote) SearchQueries(ctx context.Context) ([]string, error) {
	resp, err := r.history.SearchQueries(ctx, connect.NewRequest(&v1.SearchQueriesRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Queries, nil
}

func (r *Remote) GetVideoDetailsCache(ctx context.Context, videoID string) (domain.CachedDetails, bool, error) {
	resp, err := r.vid.GetVideoDetailsCache(ctx, connect.NewRequest(&v1.GetVideoDetailsCacheRequest{VideoId: videoID}))
	if err != nil {
		return domain.CachedDetails{}, false, err
	}
	return rProtoCachedDetails(resp.Msg.Details), resp.Msg.Found, nil
}

func (r *Remote) GetChannelVideos(ctx context.Context, channelID string) ([]domain.Video, error) {
	resp, err := r.ch.GetChannelVideos(ctx, connect.NewRequest(&v1.GetChannelVideosRequest{ChannelId: channelID}))
	if err != nil {
		return nil, err
	}
	return rProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) GetAllChannelVideos(ctx context.Context, channelIDs []string) ([]domain.Video, error) {
	resp, err := r.ch.GetAllChannelVideos(ctx, connect.NewRequest(&v1.GetAllChannelVideosRequest{ChannelIds: channelIDs}))
	if err != nil {
		return nil, err
	}
	return rProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) GetChannelLatestAll(ctx context.Context) (map[string]domain.Video, error) {
	resp, err := r.ch.GetChannelLatestAll(ctx, connect.NewRequest(&v1.GetChannelLatestAllRequest{}))
	if err != nil {
		return nil, err
	}
	out := make(map[string]domain.Video, len(resp.Msg.Latest))
	for k, pb := range resp.Msg.Latest {
		out[k] = rProtoToVideo(pb)
	}
	return out, nil
}

func (r *Remote) ChannelHideStats(ctx context.Context, channelID string) (int, int, error) {
	resp, err := r.ch.ChannelHideStats(ctx, connect.NewRequest(&v1.ChannelHideStatsRequest{ChannelId: channelID}))
	if err != nil {
		return 0, 0, err
	}
	return int(resp.Msg.Hidden), int(resp.Msg.Played), nil
}

func (r *Remote) WatchLater(ctx context.Context) ([]domain.WatchLaterEntry, error) {
	resp, err := r.playlist.WatchLater(ctx, connect.NewRequest(&v1.WatchLaterRequest{}))
	if err != nil {
		return nil, err
	}
	out := make([]domain.WatchLaterEntry, len(resp.Msg.Entries))
	for i, pb := range resp.Msg.Entries {
		out[i] = rProtoToWatchLaterEntry(pb)
	}
	return out, nil
}

func (r *Remote) HasLocalVideo(ctx context.Context, videoID string) (domain.LocalVideo, bool) {
	resp, err := r.lib.HasLocalVideo(ctx, connect.NewRequest(&v1.HasLocalVideoRequest{VideoId: videoID}))
	if err != nil {
		return domain.LocalVideo{}, false
	}
	return rProtoToLocalVideo(resp.Msg.Video), resp.Msg.Found
}

func (r *Remote) GetSubscribedChannels(ctx context.Context) ([]domain.Channel, error) {
	resp, err := r.ch.GetSubscribedChannels(ctx, connect.NewRequest(&v1.GetSubscribedChannelsRequest{}))
	if err != nil {
		return nil, err
	}
	return rProtoToChannels(resp.Msg.Channels), nil
}

func (r *Remote) GetYTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error) {
	resp, err := r.playlist.GetYTPlaylists(ctx, connect.NewRequest(&v1.GetYTPlaylistsRequest{}))
	if err != nil {
		return nil, err
	}
	return rProtoToYTPlaylists(resp.Msg.Playlists), nil
}

func (r *Remote) GetYTPlaylistVideos(ctx context.Context, playlistID string) ([]domain.Video, error) {
	resp, err := r.playlist.GetYTPlaylistVideos(ctx, connect.NewRequest(&v1.GetYTPlaylistVideosRequest{PlaylistId: playlistID}))
	if err != nil {
		return nil, err
	}
	return rProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) GetFeedCache(ctx context.Context, feed string) ([]domain.Video, error) {
	resp, err := r.feed.GetFeedCache(ctx, connect.NewRequest(&v1.GetFeedCacheRequest{Feed: feed}))
	if err != nil {
		return nil, err
	}
	return rProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) PlaylistVideoIDs(ctx context.Context, playlistID int64) ([]string, error) {
	resp, err := r.playlist.PlaylistVideoIDs(ctx, connect.NewRequest(&v1.PlaylistVideoIDsRequest{PlaylistId: playlistID}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Ids, nil
}

// ── Mutations ─────────────────────────────────────────────────────────────────

func (r *Remote) UpsertVideo(ctx context.Context, id, title, channel, channelID string, duration int, viewCount int64, uploadDate, url string) error {
	_, err := r.vid.UpsertVideo(ctx, connect.NewRequest(&v1.UpsertVideoRequest{
		Id: id, Title: title, Channel: channel, ChannelId: channelID,
		Duration: int32(duration), ViewCount: viewCount, UploadDate: uploadDate, Url: url,
	}))
	return err
}

func (r *Remote) AddLocalVideo(ctx context.Context, v domain.LocalVideo) error {
	_, err := r.lib.AddLocalVideo(ctx, connect.NewRequest(&v1.AddLocalVideoRequest{Video: rLocalVideoToProto(v)}))
	return err
}

func (r *Remote) SetVideoStatus(ctx context.Context, id string, status domain.VideoStatus) error {
	_, err := r.vid.SetVideoStatus(ctx, connect.NewRequest(&v1.SetVideoStatusRequest{Id: id, Status: string(status)}))
	return err
}

func (r *Remote) DeleteLocalVideo(ctx context.Context, id string) error {
	_, err := r.lib.DeleteLocalVideo(ctx, connect.NewRequest(&v1.DeleteLocalVideoRequest{Id: id}))
	return err
}

func (r *Remote) SaveVideoPosition(ctx context.Context, videoID string, ms int64) error {
	_, err := r.vid.SaveVideoPosition(ctx, connect.NewRequest(&v1.SaveVideoPositionRequest{VideoId: videoID, PositionMs: ms}))
	return err
}

func (r *Remote) DeleteVideoPosition(ctx context.Context, videoID string) error {
	_, err := r.vid.DeleteVideoPosition(ctx, connect.NewRequest(&v1.DeleteVideoPositionRequest{VideoId: videoID}))
	return err
}

func (r *Remote) UpdateLastPosition(ctx context.Context, id string, ms int64) error {
	_, err := r.vid.UpdateLastPosition(ctx, connect.NewRequest(&v1.UpdateLastPositionRequest{Id: id, PositionMs: ms}))
	return err
}

func (r *Remote) AddHistory(ctx context.Context, videoID, eventType, details string) error {
	_, err := r.history.AddHistory(ctx, connect.NewRequest(&v1.AddHistoryRequest{VideoId: videoID, EventType: eventType, Details: details}))
	return err
}

func (r *Remote) DeleteVideoHistory(ctx context.Context, videoID string) error {
	_, err := r.history.DeleteVideoHistory(ctx, connect.NewRequest(&v1.DeleteVideoHistoryRequest{VideoId: videoID}))
	return err
}

func (r *Remote) DeleteVideoCompletely(ctx context.Context, videoID string) error {
	// File deletion is handled server-side; skip os.Remove.
	_ = r.DeleteLocalVideo(ctx, videoID) // no-op if not a local video
	if err := r.DeleteVideoHistory(ctx, videoID); err != nil {
		return err
	}
	_ = r.DeleteVideoPosition(ctx, videoID)
	return nil
}

func (r *Remote) DeleteSearchHistory(ctx context.Context, query string) error {
	_, err := r.history.DeleteSearchHistory(ctx, connect.NewRequest(&v1.DeleteSearchHistoryRequest{Query: query}))
	return err
}

func (r *Remote) ClearHistory(ctx context.Context) error {
	_, err := r.history.ClearHistory(ctx, connect.NewRequest(&v1.ClearHistoryRequest{}))
	return err
}

func (r *Remote) LogActivity(ctx context.Context, e domain.ActivityEntry) error {
	_, err := r.history.LogActivity(ctx, connect.NewRequest(&v1.LogActivityRequest{Entry: rActivityEntryToProto(e)}))
	return err
}

func (r *Remote) HideRecVideo(ctx context.Context, videoID string) error {
	_, err := r.feed.HideVideo(ctx, connect.NewRequest(&v1.HideVideoRequest{VideoId: videoID}))
	return err
}

func (r *Remote) AddSubscribedChannel(ctx context.Context, ch domain.Channel) error {
	_, err := r.ch.AddSubscribedChannel(ctx, connect.NewRequest(&v1.AddSubscribedChannelRequest{Channel: rChannelToProto(ch)}))
	return err
}

func (r *Remote) SaveSubscribedChannels(ctx context.Context, channels []domain.Channel) error {
	_, err := r.ch.SaveSubscribedChannels(ctx, connect.NewRequest(&v1.SaveSubscribedChannelsRequest{Channels: rChannelsToProto(channels)}))
	return err
}

func (r *Remote) RemoveSubscribedChannel(ctx context.Context, channelID string) error {
	_, err := r.ch.RemoveSubscribedChannel(ctx, connect.NewRequest(&v1.RemoveSubscribedChannelRequest{ChannelId: channelID}))
	return err
}

func (r *Remote) DeleteChannelVideos(ctx context.Context, channelID string) error {
	_, err := r.ch.DeleteChannelVideos(ctx, connect.NewRequest(&v1.DeleteChannelVideosRequest{ChannelId: channelID}))
	return err
}

func (r *Remote) SetChannelAlias(ctx context.Context, channelID, alias string) error {
	_, err := r.ch.SetChannelAlias(ctx, connect.NewRequest(&v1.SetChannelAliasRequest{ChannelId: channelID, Alias: alias}))
	return err
}

func (r *Remote) SetChannelTags(ctx context.Context, channelID string, tags []string) error {
	_, err := r.ch.SetChannelTags(ctx, connect.NewRequest(&v1.SetChannelTagsRequest{ChannelId: channelID, Tags: tags}))
	return err
}

func (r *Remote) SaveChannelVideos(ctx context.Context, channelID string, videos []domain.Video) error {
	_, err := r.ch.SaveChannelVideos(ctx, connect.NewRequest(&v1.SaveChannelVideosRequest{ChannelId: channelID, Videos: rVideosToProto(videos)}))
	return err
}

func (r *Remote) CreatePlaylist(ctx context.Context, name string) (int64, error) {
	resp, err := r.playlist.CreatePlaylist(ctx, connect.NewRequest(&v1.CreatePlaylistRequest{Name: name}))
	if err != nil {
		return 0, err
	}
	return resp.Msg.Id, nil
}

func (r *Remote) DeletePlaylist(ctx context.Context, id int64) error {
	_, err := r.playlist.DeletePlaylist(ctx, connect.NewRequest(&v1.DeletePlaylistRequest{Id: id}))
	return err
}

func (r *Remote) AddToPlaylist(ctx context.Context, playlistID int64, videoID string) error {
	_, err := r.playlist.AddToPlaylist(ctx, connect.NewRequest(&v1.AddToPlaylistRequest{PlaylistId: playlistID, VideoId: videoID}))
	return err
}

func (r *Remote) RemoveFromPlaylist(ctx context.Context, playlistID int64, videoID string) error {
	_, err := r.playlist.RemoveFromPlaylist(ctx, connect.NewRequest(&v1.RemoveFromPlaylistRequest{PlaylistId: playlistID, VideoId: videoID}))
	return err
}

func (r *Remote) AddWatchLater(ctx context.Context, id, title, channel, url string) error {
	_, err := r.playlist.AddWatchLater(ctx, connect.NewRequest(&v1.AddWatchLaterRequest{Id: id, Title: title, Channel: channel, Url: url}))
	return err
}

func (r *Remote) RemoveWatchLater(ctx context.Context, id string) error {
	_, err := r.playlist.RemoveWatchLater(ctx, connect.NewRequest(&v1.RemoveWatchLaterRequest{Id: id}))
	return err
}

func (r *Remote) SaveYTPlaylists(ctx context.Context, playlists []domain.YTPlaylist) error {
	pb := make([]*v1.YTPlaylist, len(playlists))
	for i, p := range playlists {
		pb[i] = &v1.YTPlaylist{Id: p.ID, Title: p.Title}
	}
	_, err := r.playlist.SaveYTPlaylists(ctx, connect.NewRequest(&v1.SaveYTPlaylistsRequest{Playlists: pb}))
	return err
}

func (r *Remote) SaveYTPlaylistVideos(ctx context.Context, playlistID string, videos []domain.Video) error {
	_, err := r.playlist.SaveYTPlaylistVideos(ctx, connect.NewRequest(&v1.SaveYTPlaylistVideosRequest{PlaylistId: playlistID, Videos: rVideosToProto(videos)}))
	return err
}

func (r *Remote) SaveVideoDetailsCache(ctx context.Context, videoID, description, thumbnailURL string, subscribers int64) error {
	_, err := r.vid.SaveVideoDetailsCache(ctx, connect.NewRequest(&v1.SaveVideoDetailsCacheRequest{
		VideoId: videoID, Description: description, ThumbnailUrl: thumbnailURL, Subscribers: subscribers,
	}))
	return err
}

func (r *Remote) SaveVideoChapters(ctx context.Context, videoID string, chapters []domain.Chapter) error {
	pb := make([]*v1.Chapter, len(chapters))
	for i, c := range chapters {
		pb[i] = &v1.Chapter{Title: c.Title, OriginalStart: c.OriginalStart, OriginalEnd: c.OriginalEnd, AdjustedStart: c.AdjustedStart, AdjustedEnd: c.AdjustedEnd}
	}
	_, err := r.vid.SaveVideoChapters(ctx, connect.NewRequest(&v1.SaveVideoChaptersRequest{VideoId: videoID, Chapters: pb}))
	return err
}

func (r *Remote) SaveVideoSBSegments(ctx context.Context, videoID string, segs []domain.SBSegment) error {
	pb := make([]*v1.SBSegment, len(segs))
	for i, s := range segs {
		pb[i] = &v1.SBSegment{Start: s.Start, End: s.End}
	}
	_, err := r.vid.SaveVideoSBSegments(ctx, connect.NewRequest(&v1.SaveVideoSBSegmentsRequest{VideoId: videoID, Segments: pb}))
	return err
}

func (r *Remote) SaveVideoLinks(ctx context.Context, videoID string, links []domain.Link) error {
	pb := make([]*v1.Link, len(links))
	for i, l := range links {
		pb[i] = &v1.Link{Label: l.Label, Url: l.URL}
	}
	_, err := r.vid.SaveVideoLinks(ctx, connect.NewRequest(&v1.SaveVideoLinksRequest{VideoId: videoID, Links: pb}))
	return err
}

func (r *Remote) ClearRecommended(ctx context.Context) error {
	_, err := r.feed.ClearRecommended(ctx, connect.NewRequest(&v1.ClearRecommendedRequest{}))
	return err
}

func (r *Remote) ClearVideoDetailsCache(ctx context.Context) error {
	_, err := r.vid.ClearVideoDetailsCache(ctx, connect.NewRequest(&v1.ClearVideoDetailsCacheRequest{}))
	return err
}

func (r *Remote) ClearDownloads(ctx context.Context) ([]string, error) {
	resp, err := r.dl.ClearDownloads(ctx, connect.NewRequest(&v1.ClearDownloadsRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.FilePaths, nil
}

func (r *Remote) PurgeFeedCacheMissingChannelID(ctx context.Context, feed string) error {
	_, err := r.feed.PurgeFeedCache(ctx, connect.NewRequest(&v1.PurgeFeedCacheRequest{Feed: feed}))
	return err
}

func (r *Remote) SaveFeedCache(ctx context.Context, feed string, videos []domain.Video) error {
	_, err := r.feed.SaveFeedCache(ctx, connect.NewRequest(&v1.SaveFeedCacheRequest{Feed: feed, Videos: rVideosToProto(videos)}))
	return err
}

// ── Channel subscription ──────────────────────────────────────────────────────

func (r *Remote) Subscribe(ctx context.Context, ch domain.Channel) error {
	_, err := r.ch.Subscribe(ctx, connect.NewRequest(&v1.SubscribeRequest{Channel: rChannelToProto(ch)}))
	return err
}

func (r *Remote) Unsubscribe(ctx context.Context, ch domain.Channel) error {
	_, err := r.ch.Unsubscribe(ctx, connect.NewRequest(&v1.UnsubscribeRequest{Channel: rChannelToProto(ch)}))
	return err
}

// ── Playback position ─────────────────────────────────────────────────────────

func (r *Remote) ReportPosition(ctx context.Context, videoID string, posMs int64) error {
	_, err := r.vid.ReportPosition(ctx, connect.NewRequest(&v1.ReportPositionRequest{VideoId: videoID, PositionMs: posMs}))
	return err
}

func (r *Remote) ResolveSource(ctx context.Context, videoID, fallbackURL string) (PlayableSource, error) {
	resp, err := r.vid.ResolveSource(ctx, connect.NewRequest(&v1.ResolveSourceRequest{VideoId: videoID, FallbackUrl: fallbackURL}))
	if err != nil {
		return PlayableSource{}, err
	}
	return PlayableSource{URI: resp.Msg.Uri}, nil
}

// ── Download queue ────────────────────────────────────────────────────────────

func (r *Remote) Enqueue(ctx context.Context, video domain.Video, audioOnly bool) error {
	_, err := r.dl.Enqueue(ctx, connect.NewRequest(&v1.EnqueueRequest{Video: rVideoToProto(video), AudioOnly: audioOnly}))
	return err
}

func (r *Remote) CancelDownload(ctx context.Context, videoID string) error {
	_, err := r.dl.CancelDownload(ctx, connect.NewRequest(&v1.CancelDownloadRequest{VideoId: videoID}))
	return err
}

func (r *Remote) DownloadItems(ctx context.Context) ([]DownloadItem, error) {
	resp, err := r.dl.DownloadItems(ctx, connect.NewRequest(&v1.DownloadItemsRequest{}))
	if err != nil {
		return nil, err
	}
	out := make([]DownloadItem, len(resp.Msg.Items))
	for i, pb := range resp.Msg.Items {
		out[i] = DownloadItem{
			VideoID:   pb.VideoId,
			Title:     pb.Title,
			Channel:   pb.Channel,
			Duration:  pb.Duration,
			URL:       pb.Url,
			AudioOnly: pb.AudioOnly,
			Status:    DownloadStatus(pb.Status),
			Progress:  pb.Progress,
			Speed:     pb.Speed,
			ETA:       pb.Eta,
			FilePath:  pb.FilePath,
		}
	}
	return out, nil
}

// Events opens a server-streaming connection to the daemon and forwards events
// onto the returned channel. The channel is closed when ctx is canceled or the
// stream ends.
func (r *Remote) Events(ctx context.Context) (<-chan Event, error) {
	stream, err := r.dl.Events(ctx, connect.NewRequest(&v1.EventsRequest{}))
	if err != nil {
		return nil, err
	}
	out := make(chan Event, 64)
	go func() {
		defer close(out)
		defer func() { _ = stream.Close() }()
		for stream.Receive() {
			ev := stream.Msg().Event
			if ev == nil {
				continue
			}
			select {
			case out <- Event{Kind: EventKind(ev.Kind), VideoID: ev.VideoId, Detail: ev.Detail}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// ── YouTube API mutations ─────────────────────────────────────────────────────

func (r *Remote) InitYTClient(ctx context.Context) error {
	_, err := r.playlist.InitYTClient(ctx, connect.NewRequest(&v1.InitYTClientRequest{}))
	return err
}

func (r *Remote) CreateYTPlaylist(ctx context.Context, name string) (string, error) {
	resp, err := r.playlist.CreateYTPlaylist(ctx, connect.NewRequest(&v1.CreateYTPlaylistRequest{Name: name}))
	if err != nil {
		return "", err
	}
	return resp.Msg.Id, nil
}

func (r *Remote) DeleteYTPlaylist(ctx context.Context, playlistID string) error {
	_, err := r.playlist.DeleteYTPlaylist(ctx, connect.NewRequest(&v1.DeleteYTPlaylistRequest{PlaylistId: playlistID}))
	return err
}

func (r *Remote) AddToYTPlaylist(ctx context.Context, playlistID, videoID string) error {
	_, err := r.playlist.AddToYTPlaylist(ctx, connect.NewRequest(&v1.AddToYTPlaylistRequest{PlaylistId: playlistID, VideoId: videoID}))
	return err
}

func (r *Remote) RemoveFromYTPlaylist(ctx context.Context, playlistID, videoID string) error {
	_, err := r.playlist.RemoveFromYTPlaylist(ctx, connect.NewRequest(&v1.RemoveFromYTPlaylistRequest{PlaylistId: playlistID, VideoId: videoID}))
	return err
}

// ── private conversion helpers ────────────────────────────────────────────────

func rVideoToProto(v domain.Video) *v1.Video {
	return &v1.Video{Id: v.ID, Title: v.Title, Channel: v.Channel, ChannelId: v.ChannelID, Duration: int32(v.Duration), ViewCount: v.ViewCount, UploadDate: v.UploadDate, Url: v.URL}
}

func rVideosToProto(vs []domain.Video) []*v1.Video {
	out := make([]*v1.Video, len(vs))
	for i := range vs {
		out[i] = rVideoToProto(vs[i])
	}
	return out
}

func rChannelToProto(ch domain.Channel) *v1.Channel {
	return &v1.Channel{Id: ch.ID, Name: ch.Name, Alias: ch.Alias, Tags: ch.Tags, Url: ch.URL, Subscribers: ch.Subscribers, IsLocal: ch.IsLocal}
}

func rChannelsToProto(chs []domain.Channel) []*v1.Channel {
	out := make([]*v1.Channel, len(chs))
	for i := range chs {
		out[i] = rChannelToProto(chs[i])
	}
	return out
}

func rLocalVideoToProto(v domain.LocalVideo) *v1.LocalVideo {
	pb := &v1.LocalVideo{
		Id: v.ID, Title: v.Title, Channel: v.Channel, Duration: int32(v.Duration),
		ViewCount: v.ViewCount, UploadDate: v.UploadDate, FilePath: v.FilePath,
		DownloadType: v.DownloadType, Status: string(v.Status), LastPositionMs: v.LastPositionMs,
	}
	if !v.DownloadedAt.IsZero() {
		pb.DownloadedAt = timestamppb.New(v.DownloadedAt)
	}
	if !v.LastPlayed.IsZero() {
		pb.LastPlayed = timestamppb.New(v.LastPlayed)
	}
	return pb
}

func rActivityEntryToProto(e domain.ActivityEntry) *v1.ActivityEntry {
	return &v1.ActivityEntry{
		Id: e.ID, Type: e.Type, IsLocal: e.IsLocal, ChannelId: e.ChannelID, ChannelName: e.ChannelName,
		PlaylistId: e.PlaylistID, PlaylistLocalId: e.PlaylistLocalID, PlaylistName: e.PlaylistName,
		VideoId: e.VideoID, VideoTitle: e.VideoTitle, Timestamp: timestamppb.New(e.Timestamp),
	}
}

func rProtoToVideo(pb *v1.Video) domain.Video {
	if pb == nil {
		return domain.Video{}
	}
	return domain.Video{ID: pb.Id, Title: pb.Title, Channel: pb.Channel, ChannelID: pb.ChannelId, Duration: int(pb.Duration), ViewCount: pb.ViewCount, UploadDate: pb.UploadDate, URL: pb.Url}
}

func rProtoToVideos(pbs []*v1.Video) []domain.Video {
	out := make([]domain.Video, len(pbs))
	for i, pb := range pbs {
		out[i] = rProtoToVideo(pb)
	}
	return out
}

func rProtoToChannel(pb *v1.Channel) domain.Channel {
	if pb == nil {
		return domain.Channel{}
	}
	return domain.Channel{ID: pb.Id, Name: pb.Name, Alias: pb.Alias, Tags: pb.Tags, URL: pb.Url, Subscribers: pb.Subscribers, IsLocal: pb.IsLocal}
}

func rProtoToChannels(pbs []*v1.Channel) []domain.Channel {
	out := make([]domain.Channel, len(pbs))
	for i, pb := range pbs {
		out[i] = rProtoToChannel(pb)
	}
	return out
}

func rProtoToLocalVideo(pb *v1.LocalVideo) domain.LocalVideo {
	if pb == nil {
		return domain.LocalVideo{}
	}
	v := domain.LocalVideo{
		ID: pb.Id, Title: pb.Title, Channel: pb.Channel, Duration: int(pb.Duration),
		ViewCount: pb.ViewCount, UploadDate: pb.UploadDate, FilePath: pb.FilePath,
		DownloadType: pb.DownloadType, Status: domain.VideoStatus(pb.Status), LastPositionMs: pb.LastPositionMs,
	}
	if pb.DownloadedAt != nil {
		v.DownloadedAt = pb.DownloadedAt.AsTime()
	}
	if pb.LastPlayed != nil {
		v.LastPlayed = pb.LastPlayed.AsTime()
	}
	return v
}

func rProtoToPlaylist(pb *v1.Playlist) domain.Playlist {
	if pb == nil {
		return domain.Playlist{}
	}
	p := domain.Playlist{ID: pb.Id, Name: pb.Name}
	if pb.CreatedAt != nil {
		p.CreatedAt = pb.CreatedAt.AsTime()
	}
	return p
}

func rProtoToYTPlaylists(pbs []*v1.YTPlaylist) []domain.YTPlaylist {
	out := make([]domain.YTPlaylist, len(pbs))
	for i, pb := range pbs {
		if pb != nil {
			out[i] = domain.YTPlaylist{ID: pb.Id, Title: pb.Title}
		}
	}
	return out
}

func rProtoToWatchLaterEntry(pb *v1.WatchLaterEntry) domain.WatchLaterEntry {
	if pb == nil {
		return domain.WatchLaterEntry{}
	}
	e := domain.WatchLaterEntry{VideoID: pb.VideoId, Title: pb.Title, Channel: pb.Channel, URL: pb.Url}
	if pb.AddedAt != nil {
		e.AddedAt = pb.AddedAt.AsTime()
	}
	return e
}

func rProtoToHistoryEntry(pb *v1.HistoryEntry) domain.HistoryEntry {
	if pb == nil {
		return domain.HistoryEntry{}
	}
	e := domain.HistoryEntry{
		ID: pb.Id, VideoID: pb.VideoId, Title: pb.Title, Channel: pb.Channel, ChannelID: pb.ChannelId,
		Duration: int(pb.Duration), ViewCount: pb.ViewCount, UploadDate: pb.UploadDate,
		EventType: pb.EventType, Details: pb.Details,
	}
	if pb.Timestamp != nil {
		e.Timestamp = pb.Timestamp.AsTime()
	}
	return e
}

func rProtoToHistoryEntries(pbs []*v1.HistoryEntry) []domain.HistoryEntry {
	out := make([]domain.HistoryEntry, len(pbs))
	for i, pb := range pbs {
		out[i] = rProtoToHistoryEntry(pb)
	}
	return out
}

func rProtoToActivityEntry(pb *v1.ActivityEntry) domain.ActivityEntry {
	if pb == nil {
		return domain.ActivityEntry{}
	}
	e := domain.ActivityEntry{
		ID: pb.Id, Type: pb.Type, IsLocal: pb.IsLocal, ChannelID: pb.ChannelId, ChannelName: pb.ChannelName,
		PlaylistID: pb.PlaylistId, PlaylistLocalID: pb.PlaylistLocalId, PlaylistName: pb.PlaylistName,
		VideoID: pb.VideoId, VideoTitle: pb.VideoTitle,
	}
	if pb.Timestamp != nil {
		e.Timestamp = pb.Timestamp.AsTime()
	}
	return e
}

func rProtoToVideoDetails(pb *v1.VideoDetails) domain.VideoDetails {
	if pb == nil {
		return domain.VideoDetails{}
	}
	vd := domain.VideoDetails{
		Video:        rProtoToVideo(pb.Video),
		Description:  pb.Description,
		ThumbnailURL: pb.ThumbnailUrl,
		Subscribers:  pb.Subscribers,
	}
	for _, rc := range pb.Chapters {
		if rc != nil {
			vd.Chapters = append(vd.Chapters, domain.RawChapter{Title: rc.Title, StartTime: rc.StartTime, EndTime: rc.EndTime})
		}
	}
	return vd
}

func rProtoCachedDetails(pb *v1.CachedDetails) domain.CachedDetails {
	if pb == nil {
		return domain.CachedDetails{}
	}
	cd := domain.CachedDetails{Description: pb.Description, ThumbnailURL: pb.ThumbnailUrl, Subscribers: pb.Subscribers}
	if pb.LinksParsed {
		links := make([]domain.Link, len(pb.Links))
		for i, l := range pb.Links {
			if l != nil {
				links[i] = domain.Link{Label: l.Label, URL: l.Url}
			}
		}
		cd.Links = &links
	}
	if pb.ChaptersParsed {
		chapters := make([]domain.Chapter, len(pb.Chapters))
		for i, c := range pb.Chapters {
			if c != nil {
				chapters[i] = domain.Chapter{Title: c.Title, OriginalStart: c.OriginalStart, OriginalEnd: c.OriginalEnd, AdjustedStart: c.AdjustedStart, AdjustedEnd: c.AdjustedEnd}
			}
		}
		cd.Chapters = &chapters
	}
	if pb.SbSegmentsParsed {
		segs := make([]domain.SBSegment, len(pb.SbSegments))
		for i, s := range pb.SbSegments {
			if s != nil {
				segs[i] = domain.SBSegment{Start: s.Start, End: s.End}
			}
		}
		cd.SBSegments = &segs
	}
	return cd
}
