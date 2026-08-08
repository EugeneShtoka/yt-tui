package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// SaveYTPlaylists persists the YouTube playlist list.
func (d *DB) SaveYTPlaylists(ctx context.Context, playlists []domain.YTPlaylist) error {
	return d.withTx(ctx, "SaveYTPlaylists", func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM yt_playlists`); err != nil {
			return fmt.Errorf("SaveYTPlaylists delete: %w", err)
		}
		for _, pl := range playlists {
			if _, err := tx.ExecContext(ctx, `INSERT INTO yt_playlists (id, title) VALUES (?, ?)`, pl.ID, pl.Title); err != nil {
				return fmt.Errorf("SaveYTPlaylists insert: %w", err)
			}
		}
		return nil
	})
}

// GetYTPlaylists returns the cached YouTube playlist list.
func (d *DB) GetYTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error) {
	out, err := queryList(ctx, d.sql, `SELECT id, title FROM yt_playlists ORDER BY rowid`,
		func(rows *sql.Rows) (domain.YTPlaylist, error) {
			var pl domain.YTPlaylist
			err := rows.Scan(&pl.ID, &pl.Title)
			return pl, err
		})
	if err != nil {
		return nil, fmt.Errorf("GetYTPlaylists: %w", err)
	}
	return out, nil
}

// SaveYTPlaylistVideos replaces the cached video list for a YT playlist.
func (d *DB) SaveYTPlaylistVideos(ctx context.Context, playlistID string, videos []domain.Video) error {
	return d.withTx(ctx, "SaveYTPlaylistVideos", func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM yt_playlist_videos WHERE playlist_id=?`, playlistID); err != nil {
			return fmt.Errorf("SaveYTPlaylistVideos delete: %w", err)
		}
		for i, v := range videos {
			if err := upsertVideoTx(ctx, tx, v); err != nil {
				return fmt.Errorf("SaveYTPlaylistVideos upsert video: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT OR REPLACE INTO yt_playlist_videos (playlist_id, video_id, position)
				VALUES (?, ?, ?)
			`, playlistID, v.ID, i); err != nil {
				return fmt.Errorf("SaveYTPlaylistVideos insert: %w", err)
			}
		}
		return nil
	})
}

// InWatchLater reports whether a video is currently in Watch Later, checking
// both possible stores: the cached YouTube "WL" playlist and the reserved local
// "Watch Later" playlist. ytWLID / localWLName are domain.WatchLaterYTID and
// domain.WatchLaterPlaylistName (passed in to keep db free of that dependency
// direction). Used by the watched-percent auto-remove to avoid firing a YouTube
// removal for a video that isn't queued.
func (d *DB) InWatchLater(ctx context.Context, videoID, ytWLID, localWLName string) (bool, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM yt_playlist_videos WHERE playlist_id=? AND video_id=?)
			+ (SELECT COUNT(*) FROM playlist_videos pv
			   JOIN playlists p ON p.id = pv.playlist_id
			   WHERE p.name=? AND pv.video_id=?)
	`, ytWLID, videoID, localWLName, videoID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("InWatchLater: %w", err)
	}
	return n > 0, nil
}

// RemoveYTPlaylistVideo deletes a single video from a cached YT playlist. Used
// for optimistic local removal (e.g. Watch Later) so the UI reflects a YouTube
// mutation immediately, before the next backfill sync rewrites the cache.
func (d *DB) RemoveYTPlaylistVideo(ctx context.Context, playlistID, videoID string) error {
	if _, err := d.sql.ExecContext(ctx,
		`DELETE FROM yt_playlist_videos WHERE playlist_id=? AND video_id=?`, playlistID, videoID); err != nil {
		return fmt.Errorf("RemoveYTPlaylistVideo: %w", err)
	}
	return nil
}

// GetYTPlaylistVideos returns cached videos for a YT playlist in position order.
func (d *DB) GetYTPlaylistVideos(ctx context.Context, playlistID string) ([]domain.Video, error) {
	out, err := queryList(ctx, d.sql, `
		SELECT v.id, v.title, COALESCE(v.channel,''), COALESCE(v.channel_id,''),
		       COALESCE(v.duration,0), COALESCE(v.view_count,0),
		       COALESCE(v.upload_date,''), COALESCE(v.url,'')
		FROM yt_playlist_videos pv
		JOIN videos v ON v.id = pv.video_id
		WHERE pv.playlist_id = ?
		ORDER BY pv.position
	`, scanVideoRow, playlistID)
	if err != nil {
		return nil, fmt.Errorf("GetYTPlaylistVideos: %w", err)
	}
	return out, nil
}

// Playlists returns all custom playlists.
func (d *DB) Playlists(ctx context.Context) ([]domain.Playlist, error) {
	result, err := queryList(ctx, d.sql, `SELECT id, name, created_at FROM playlists ORDER BY name`,
		func(rows *sql.Rows) (domain.Playlist, error) {
			var p domain.Playlist
			err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt)
			return p, err
		})
	if err != nil {
		return nil, fmt.Errorf("Playlists: %w", err)
	}
	return result, nil
}

// CreatePlaylist creates a new playlist, or returns the existing playlist's id
// if name is already taken. INSERT OR IGNORE + LastInsertId is unsafe here:
// SQLite's last-insert-rowid is connection-global and unchanged by an ignored
// insert, so it can return an unrelated playlist's id on a duplicate name.
func (d *DB) CreatePlaylist(ctx context.Context, name string) (int64, error) {
	var id int64
	err := d.sql.QueryRowContext(ctx,
		`INSERT INTO playlists (name) VALUES (?)
		 ON CONFLICT(name) DO UPDATE SET name=name
		 RETURNING id`, name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("CreatePlaylist insert: %w", err)
	}
	return id, nil
}

// DeletePlaylist removes a playlist.
func (d *DB) DeletePlaylist(ctx context.Context, id int64) error {
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM playlists WHERE id=?`, id); err != nil {
		return fmt.Errorf("DeletePlaylist: %w", err)
	}
	return nil
}

// AddToPlaylist adds a video to a playlist.
func (d *DB) AddToPlaylist(ctx context.Context, playlistID int64, videoID string) error {
	if _, err := d.sql.ExecContext(ctx, `
		INSERT OR IGNORE INTO playlist_videos (playlist_id, video_id) VALUES (?, ?)
	`, playlistID, videoID); err != nil {
		return fmt.Errorf("AddToPlaylist: %w", err)
	}
	return nil
}

// RemoveFromPlaylist removes a video from a playlist.
func (d *DB) RemoveFromPlaylist(ctx context.Context, playlistID int64, videoID string) error {
	if _, err := d.sql.ExecContext(ctx, `
		DELETE FROM playlist_videos WHERE playlist_id=? AND video_id=?
	`, playlistID, videoID); err != nil {
		return fmt.Errorf("RemoveFromPlaylist: %w", err)
	}
	return nil
}

// PlaylistVideoIDs returns video IDs in a playlist (needs cross-reference with a video cache).
func (d *DB) PlaylistVideoIDs(ctx context.Context, playlistID int64) ([]string, error) {
	ids, err := queryList(ctx, d.sql, `
		SELECT video_id FROM playlist_videos
		WHERE playlist_id=? ORDER BY position, added_at
	`, scanString, playlistID)
	if err != nil {
		return nil, fmt.Errorf("PlaylistVideoIDs: %w", err)
	}
	return ids, nil
}

// PlaylistVideos returns full video details for all videos in a playlist.
func (d *DB) PlaylistVideos(ctx context.Context, playlistID int64) ([]domain.Video, error) {
	result, err := queryList(ctx, d.sql, `
		SELECT v.id, v.title, COALESCE(v.channel,''), COALESCE(v.channel_id,''),
		       COALESCE(v.duration,0), COALESCE(v.view_count,0),
		       COALESCE(v.upload_date,''), COALESCE(v.url,'')
		FROM playlist_videos pv
		JOIN videos v ON v.id = pv.video_id
		WHERE pv.playlist_id = ?
		ORDER BY pv.position, pv.added_at
	`, scanVideoRow, playlistID)
	if err != nil {
		return nil, fmt.Errorf("PlaylistVideos: %w", err)
	}
	return result, nil
}
