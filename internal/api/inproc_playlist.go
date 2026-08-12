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

func (p *InProc) LocalPlaylistVideos(ctx context.Context, playlistID string) ([]domain.Video, error) {
	return p.db.PlaylistVideos(ctx, playlistID)
}

func (p *InProc) PlaylistVideoIDs(ctx context.Context, playlistID string) ([]string, error) {
	return p.db.PlaylistVideoIDs(ctx, playlistID)
}

func (p *InProc) CreatePlaylist(ctx context.Context, name string) (string, error) {
	return p.db.CreatePlaylist(ctx, name)
}

func (p *InProc) DeletePlaylist(ctx context.Context, id string) error {
	return p.db.DeletePlaylist(ctx, id)
}

func (p *InProc) AddToPlaylist(ctx context.Context, playlistID string, videoID string) error {
	return p.db.AddToPlaylist(ctx, playlistID, videoID)
}

func (p *InProc) RemoveFromPlaylist(ctx context.Context, playlistID string, videoID string) error {
	return p.db.RemoveFromPlaylist(ctx, playlistID, videoID)
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

// SyncYTPlaylists is the throttled background refresh. When the cache was synced
// within refresh_minutes it serves the cache untouched (a single cheap DB read,
// no network). Otherwise it fetches live and persists, so the cache — and the
// next fresh window — advance. A live-fetch error falls back to the cache so a
// transient failure doesn't blank the caller's list.
func (p *InProc) SyncYTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error) {
	if fresh, err := p.db.YTPlaylistsFresh(ctx, p.cfg.RefreshMinutes); err == nil && fresh {
		return p.db.GetYTPlaylists(ctx)
	}
	pls, err := p.yt.YTPlaylists(ctx)
	if err != nil {
		cached, _ := p.db.GetYTPlaylists(ctx)
		return cached, err
	}
	if err := p.db.SaveYTPlaylists(ctx, pls); err != nil {
		return pls, err
	}
	return pls, nil
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

// AddToWatchLater adds a video to Watch Later. With YT auth it hits YouTube's
// "WL" playlist (the existing playlist-cache refresh then persists it locally);
// without auth it falls back to the reserved local "Watch Later" playlist,
// upserting the video row first so it rehydrates in list views.
func (p *InProc) AddToWatchLater(ctx context.Context, v domain.Video) error {
	if client := p.ytAPI.Load(); client != nil {
		return client.AddToWatchLater(ctx, v.ID)
	}
	id, err := p.db.CreatePlaylist(ctx, domain.WatchLaterPlaylistName)
	if err != nil {
		return err
	}
	if err := p.db.UpsertVideo(ctx, v.ID, v.Title, v.Channel, v.ChannelID, v.Duration, v.ViewCount, v.UploadDate, v.URL); err != nil {
		return err
	}
	return p.db.AddToPlaylist(ctx, id, v.ID)
}

// RemoveFromWatchLater removes a video from Watch Later, mirroring
// AddToWatchLater's store choice: YouTube's "WL" playlist with auth, else the
// reserved local "Watch Later" playlist. CreatePlaylist is idempotent, so the
// offline path resolves the reserved playlist's id without creating duplicates.
func (p *InProc) RemoveFromWatchLater(ctx context.Context, videoID string) error {
	if client := p.ytAPI.Load(); client != nil {
		if err := client.RemoveFromWatchLater(ctx, videoID); err != nil {
			return err
		}
		// Optimistic local removal: drop from the cached YT "WL" so the UI reflects
		// the change immediately, before the next backfill sync rewrites the cache.
		// Best-effort — a cache miss here is harmless.
		_ = p.db.RemoveYTPlaylistVideo(ctx, domain.WatchLaterYTID, videoID)
		return nil
	}
	id, err := p.db.CreatePlaylist(ctx, domain.WatchLaterPlaylistName)
	if err != nil {
		return err
	}
	return p.db.RemoveFromPlaylist(ctx, id, videoID)
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
