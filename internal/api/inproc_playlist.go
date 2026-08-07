//nolint:wrapcheck // pass-through adapter; errors from backend/db/yt are already contextual
package api

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

// ── PlaylistBackend ──────────────────────────────────────────────────────────

func (p *InProc) LocalPlaylists(ctx context.Context) ([]domain.Playlist, error) {
	return p.db.Playlists(ctx)
}

func (p *InProc) LocalPlaylistVideos(ctx context.Context, playlistID int64) ([]domain.Video, error) {
	return p.db.PlaylistVideos(ctx, playlistID)
}

func (p *InProc) PlaylistVideoIDs(ctx context.Context, playlistID int64) ([]string, error) {
	return p.db.PlaylistVideoIDs(ctx, playlistID)
}

func (p *InProc) CreatePlaylist(ctx context.Context, name string) (int64, error) {
	return p.db.CreatePlaylist(ctx, name)
}

func (p *InProc) DeletePlaylist(ctx context.Context, id int64) error {
	return p.db.DeletePlaylist(ctx, id)
}

func (p *InProc) AddToPlaylist(ctx context.Context, playlistID int64, videoID string) error {
	return p.db.AddToPlaylist(ctx, playlistID, videoID)
}

func (p *InProc) RemoveFromPlaylist(ctx context.Context, playlistID int64, videoID string) error {
	return p.db.RemoveFromPlaylist(ctx, playlistID, videoID)
}

func (p *InProc) WatchLater(ctx context.Context) ([]domain.WatchLaterEntry, error) {
	return p.db.WatchLater(ctx)
}

func (p *InProc) AddWatchLater(ctx context.Context, id, title, channel, url string) error {
	return p.db.AddWatchLater(ctx, id, title, channel, url)
}

func (p *InProc) RemoveWatchLater(ctx context.Context, id string) error {
	return p.db.RemoveWatchLater(ctx, id)
}

func (p *InProc) YTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error) {
	return p.yt.YTPlaylists(ctx)
}

func (p *InProc) YTPlaylistVideos(ctx context.Context, playlistID string) ([]domain.Video, error) {
	return p.yt.PlaylistVideos(ctx, playlistID)
}

func (p *InProc) GetYTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error) {
	return p.db.GetYTPlaylists(ctx)
}

func (p *InProc) GetYTPlaylistVideos(ctx context.Context, playlistID string) ([]domain.Video, error) {
	return p.db.GetYTPlaylistVideos(ctx, playlistID)
}

func (p *InProc) SaveYTPlaylists(ctx context.Context, playlists []domain.YTPlaylist) error {
	return p.db.SaveYTPlaylists(ctx, playlists)
}

func (p *InProc) SaveYTPlaylistVideos(ctx context.Context, playlistID string, videos []domain.Video) error {
	return p.db.SaveYTPlaylistVideos(ctx, playlistID, videos)
}

// ── YouTube API mutations (require browser-cookie auth) ─────────────────────

func (p *InProc) InitYTClient(ctx context.Context) error {
	client, err := youtube.NewYTClient(ctx, p.cfg)
	if err != nil {
		return err
	}
	p.ytAPI.Store(client)
	p.ch.SetYTAPI(client)
	return nil
}

func (p *InProc) CreateYTPlaylist(ctx context.Context, name string) (string, error) {
	client := p.ytAPI.Load()
	if client == nil {
		return "", domain.ErrYTNotInitialized
	}
	return client.CreatePlaylist(ctx, name)
}

func (p *InProc) DeleteYTPlaylist(ctx context.Context, playlistID string) error {
	client := p.ytAPI.Load()
	if client == nil {
		return domain.ErrYTNotInitialized
	}
	return client.DeletePlaylist(ctx, playlistID)
}

func (p *InProc) AddToYTPlaylist(ctx context.Context, playlistID, videoID string) error {
	client := p.ytAPI.Load()
	if client == nil {
		return domain.ErrYTNotInitialized
	}
	return client.AddToPlaylist(ctx, playlistID, videoID)
}

func (p *InProc) RemoveFromYTPlaylist(ctx context.Context, playlistID, videoID string) error {
	client := p.ytAPI.Load()
	if client == nil {
		return domain.ErrYTNotInitialized
	}
	return client.RemoveFromPlaylist(ctx, playlistID, videoID)
}
