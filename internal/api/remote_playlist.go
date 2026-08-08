//nolint:wrapcheck // Connect errors are already structured; pass through without re-wrapping.
package api

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/protoconv"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ── PlaylistBackend ──────────────────────────────────────────────────────────

func (r *Remote) LocalPlaylists(ctx context.Context) ([]domain.Playlist, error) {
	resp, err := r.playlist.LocalPlaylists(ctx, connect.NewRequest(&v1.LocalPlaylistsRequest{}))
	if err != nil {
		return nil, err
	}
	out := make([]domain.Playlist, len(resp.Msg.Playlists))
	for i, pb := range resp.Msg.Playlists {
		out[i] = protoconv.ProtoToPlaylist(pb)
	}
	return out, nil
}

func (r *Remote) LocalPlaylistVideos(ctx context.Context, playlistID int64) ([]domain.Video, error) {
	resp, err := r.playlist.LocalPlaylistVideos(ctx, connect.NewRequest(&v1.LocalPlaylistVideosRequest{PlaylistId: playlistID}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) PlaylistVideoIDs(ctx context.Context, playlistID int64) ([]string, error) {
	resp, err := r.playlist.PlaylistVideoIDs(ctx, connect.NewRequest(&v1.PlaylistVideoIDsRequest{PlaylistId: playlistID}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Ids, nil
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

func (r *Remote) YTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error) {
	resp, err := r.playlist.YTPlaylists(ctx, connect.NewRequest(&v1.YTPlaylistsRequest{}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToYTPlaylists(resp.Msg.Playlists), nil
}

func (r *Remote) YTPlaylistVideos(ctx context.Context, playlistID string) ([]domain.Video, error) {
	resp, err := r.playlist.YTPlaylistVideos(ctx, connect.NewRequest(&v1.YTPlaylistVideosRequest{PlaylistId: playlistID}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) GetYTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error) {
	resp, err := r.playlist.GetYTPlaylists(ctx, connect.NewRequest(&v1.GetYTPlaylistsRequest{}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToYTPlaylists(resp.Msg.Playlists), nil
}

func (r *Remote) GetYTPlaylistVideos(ctx context.Context, playlistID string) ([]domain.Video, error) {
	resp, err := r.playlist.GetYTPlaylistVideos(ctx, connect.NewRequest(&v1.GetYTPlaylistVideosRequest{PlaylistId: playlistID}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToVideos(resp.Msg.Videos), nil
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
	_, err := r.playlist.SaveYTPlaylistVideos(ctx, connect.NewRequest(&v1.SaveYTPlaylistVideosRequest{PlaylistId: playlistID, Videos: protoconv.VideosToProto(videos)}))
	return err
}

func (r *Remote) AddToWatchLater(ctx context.Context, v domain.Video) error {
	_, err := r.playlist.AddToWatchLater(ctx, connect.NewRequest(&v1.AddToWatchLaterRequest{Video: protoconv.VideoToProto(v)}))
	return err
}

func (r *Remote) RemoveFromWatchLater(ctx context.Context, videoID string) error {
	_, err := r.playlist.RemoveFromWatchLater(ctx, connect.NewRequest(&v1.RemoveFromWatchLaterRequest{VideoId: videoID}))
	return err
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
