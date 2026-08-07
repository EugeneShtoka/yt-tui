package db

import (
	"context"
	"fmt"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// SaveYTPlaylists persists the YouTube playlist list.
func (d *DB) SaveYTPlaylists(ctx context.Context, playlists []domain.YTPlaylist) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("SaveYTPlaylists begin: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM yt_playlists`); err != nil {
		return fmt.Errorf("SaveYTPlaylists delete: %w", err)
	}
	for _, pl := range playlists {
		if _, err := tx.ExecContext(ctx, `INSERT INTO yt_playlists (id, title) VALUES (?, ?)`, pl.ID, pl.Title); err != nil {
			return fmt.Errorf("SaveYTPlaylists insert: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("SaveYTPlaylists commit: %w", err)
	}
	return nil
}

// GetYTPlaylists returns the cached YouTube playlist list.
func (d *DB) GetYTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT id, title FROM yt_playlists ORDER BY rowid`)
	if err != nil {
		return nil, fmt.Errorf("GetYTPlaylists query: %w", err)
	}
	defer rows.Close()
	var out []domain.YTPlaylist
	for rows.Next() {
		var pl domain.YTPlaylist
		if err := rows.Scan(&pl.ID, &pl.Title); err != nil {
			return nil, fmt.Errorf("GetYTPlaylists scan: %w", err)
		}
		out = append(out, pl)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("GetYTPlaylists rows: %w", err)
	}
	return out, nil
}

// SaveYTPlaylistVideos replaces the cached video list for a YT playlist.
func (d *DB) SaveYTPlaylistVideos(ctx context.Context, playlistID string, videos []domain.Video) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("SaveYTPlaylistVideos begin: %w", err)
	}
	defer tx.Rollback()
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
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("SaveYTPlaylistVideos commit: %w", err)
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
	rows, err := d.sql.QueryContext(ctx, `SELECT id, name, created_at FROM playlists ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("Playlists query: %w", err)
	}
	defer rows.Close()
	var result []domain.Playlist
	for rows.Next() {
		var p domain.Playlist
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("Playlists scan: %w", err)
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("Playlists rows: %w", err)
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
	rows, err := d.sql.QueryContext(ctx, `
		SELECT video_id FROM playlist_videos
		WHERE playlist_id=? ORDER BY position, added_at
	`, playlistID)
	if err != nil {
		return nil, fmt.Errorf("PlaylistVideoIDs query: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("PlaylistVideoIDs scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return ids, fmt.Errorf("PlaylistVideoIDs rows: %w", err)
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

// AddWatchLater adds a video to watch later.
func (d *DB) AddWatchLater(ctx context.Context, id, title, channel, url string) error {
	if _, err := d.sql.ExecContext(ctx, `
		INSERT OR REPLACE INTO watch_later (video_id, title, channel, url) VALUES (?, ?, ?, ?)
	`, id, title, channel, url); err != nil {
		return fmt.Errorf("AddWatchLater: %w", err)
	}
	return nil
}

// RemoveWatchLater removes a video from watch later.
func (d *DB) RemoveWatchLater(ctx context.Context, id string) error {
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM watch_later WHERE video_id=?`, id); err != nil {
		return fmt.Errorf("RemoveWatchLater: %w", err)
	}
	return nil
}

// WatchLater returns all watch-later entries.
func (d *DB) WatchLater(ctx context.Context) ([]domain.WatchLaterEntry, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT video_id, title, channel, url, added_at
		FROM watch_later ORDER BY added_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("WatchLater query: %w", err)
	}
	defer rows.Close()
	var result []domain.WatchLaterEntry
	for rows.Next() {
		var e domain.WatchLaterEntry
		if err := rows.Scan(&e.VideoID, &e.Title, &e.Channel, &e.URL, &e.AddedAt); err != nil {
			return nil, fmt.Errorf("WatchLater scan: %w", err)
		}
		result = append(result, e)
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("WatchLater rows: %w", err)
	}
	return result, nil
}
