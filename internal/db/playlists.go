package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// Playlists (local + YouTube) share one storage shape since Target 1: rows live
// in the collections table (kind='local'|'yt') and memberships in
// collection_videos. Local ids are synthetic "local:N" strings; YT ids are the
// YouTube playlist id (incl. "WL"). Membership order is insertion order (rowid).

// SaveYTPlaylists persists the YouTube playlist list, replacing the cached 'yt'
// collections wholesale. It deliberately does not touch collection_videos: the
// per-playlist video caches are managed by SaveYTPlaylistVideos and must survive
// a list-only refresh (there is no FK cascade for exactly this reason).
func (d *DB) SaveYTPlaylists(ctx context.Context, playlists []domain.YTPlaylist) error {
	return d.withTx(ctx, "SaveYTPlaylists", func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM collections WHERE kind='yt'`); err != nil {
			return fmt.Errorf("SaveYTPlaylists delete: %w", err)
		}
		for _, pl := range playlists {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO collections (id, kind, name, synced) VALUES (?, 'yt', ?, 1)`,
				pl.ID, pl.Title); err != nil {
				return fmt.Errorf("SaveYTPlaylists insert: %w", err)
			}
		}
		return nil
	})
}

// YTPlaylistsFresh reports whether the cached YouTube playlist list was synced
// within the last withinMinutes — i.e. any 'yt' collection was updated that
// recently (SaveYTPlaylists rewrites the rows, resetting updated_at, on every
// sync). Used to throttle the background sync. withinMinutes <= 0 disables the
// throttle (always stale). The comparison runs entirely in SQLite: both
// updated_at and datetime('now') are UTC, so it's timezone-consistent.
func (d *DB) YTPlaylistsFresh(ctx context.Context, withinMinutes int) (bool, error) {
	if withinMinutes <= 0 {
		return false, nil
	}
	var n int
	if err := d.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM collections WHERE kind='yt' AND updated_at > datetime('now', ?)`,
		fmt.Sprintf("-%d minutes", withinMinutes),
	).Scan(&n); err != nil {
		return false, fmt.Errorf("YTPlaylistsFresh: %w", err)
	}
	return n > 0, nil
}

// GetYTPlaylists returns the cached YouTube playlist list.
func (d *DB) GetYTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error) {
	out, err := queryList(ctx, d.sql, `SELECT id, name FROM collections WHERE kind='yt' ORDER BY rowid`,
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

// SaveYTPlaylistVideos replaces the cached video list for a YT playlist. Videos
// are written in order; reads recover that order via rowid.
func (d *DB) SaveYTPlaylistVideos(ctx context.Context, playlistID string, videos []domain.Video) error {
	return d.withTx(ctx, "SaveYTPlaylistVideos", func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM collection_videos WHERE collection_id=?`, playlistID); err != nil {
			return fmt.Errorf("SaveYTPlaylistVideos delete: %w", err)
		}
		for _, v := range videos {
			if err := upsertVideoTx(ctx, tx, v); err != nil {
				return fmt.Errorf("SaveYTPlaylistVideos upsert video: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT OR REPLACE INTO collection_videos (collection_id, video_id)
				VALUES (?, ?)
			`, playlistID, v.ID); err != nil {
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
// removal for a video that isn't queued. Both stores are now collection_videos
// rows, distinguished by the parent collection's kind.
func (d *DB) InWatchLater(ctx context.Context, videoID, ytWLID, localWLName string) (bool, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM collection_videos WHERE collection_id=? AND video_id=?)
			+ (SELECT COUNT(*) FROM collection_videos cv
			   JOIN collections c ON c.id = cv.collection_id
			   WHERE c.kind='local' AND c.name=? AND cv.video_id=?)
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
		`DELETE FROM collection_videos WHERE collection_id=? AND video_id=?`, playlistID, videoID); err != nil {
		return fmt.Errorf("RemoveYTPlaylistVideo: %w", err)
	}
	return nil
}

// GetYTPlaylistVideos returns cached videos for a YT playlist in insertion order.
func (d *DB) GetYTPlaylistVideos(ctx context.Context, playlistID string) ([]domain.Video, error) {
	out, err := queryList(ctx, d.sql, `
		SELECT v.id, v.title, COALESCE(v.channel,''), COALESCE(v.channel_id,''),
		       COALESCE(v.duration,0), COALESCE(v.view_count,0),
		       COALESCE(v.upload_date,''), COALESCE(v.url,'')
		FROM collection_videos cv
		JOIN videos v ON v.id = cv.video_id
		WHERE cv.collection_id = ?
		ORDER BY cv.rowid
	`, scanVideoRow, playlistID)
	if err != nil {
		return nil, fmt.Errorf("GetYTPlaylistVideos: %w", err)
	}
	return out, nil
}

// Playlists returns all local (user-created) playlists.
func (d *DB) Playlists(ctx context.Context) ([]domain.Playlist, error) {
	result, err := queryList(ctx, d.sql, `SELECT id, name, created_at FROM collections WHERE kind='local' ORDER BY name`,
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

// CreatePlaylist creates a new local playlist, or returns the existing
// playlist's id if the name is already taken (create-or-get by name, preserving
// the pre-unification INSERT-OR-IGNORE-then-return-existing semantics). Local
// ids are "local:N" where N is one past the current max, so they stay unique and
// never collide with a YouTube playlist id.
func (d *DB) CreatePlaylist(ctx context.Context, name string) (string, error) {
	var id string
	err := d.withTx(ctx, "CreatePlaylist", func(tx *sql.Tx) error {
		e := tx.QueryRowContext(ctx, `SELECT id FROM collections WHERE kind='local' AND name=?`, name).Scan(&id)
		if e == nil {
			return nil // already exists
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		return tx.QueryRowContext(ctx, `
			INSERT INTO collections (id, kind, name, synced)
			VALUES (
				'local:' || (SELECT COALESCE(MAX(CAST(SUBSTR(id, 7) AS INTEGER)), 0) + 1
				             FROM collections WHERE kind='local'),
				'local', ?, 0)
			RETURNING id`, name).Scan(&id)
	})
	if err != nil {
		return "", fmt.Errorf("CreatePlaylist: %w", err)
	}
	return id, nil
}

// DeletePlaylist removes a local playlist and its memberships. There is no FK
// cascade on collection_videos (see collections DDL), so the membership rows are
// deleted explicitly; the kind='local' guard keeps a YT collection id from ever
// being deleted through the local verb.
func (d *DB) DeletePlaylist(ctx context.Context, id string) error {
	return d.withTx(ctx, "DeletePlaylist", func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM collection_videos WHERE collection_id=?`, id); err != nil {
			return fmt.Errorf("DeletePlaylist videos: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM collections WHERE id=? AND kind='local'`, id); err != nil {
			return fmt.Errorf("DeletePlaylist: %w", err)
		}
		return nil
	})
}

// AddToPlaylist adds a video to a playlist (appends; rowid preserves add order).
func (d *DB) AddToPlaylist(ctx context.Context, playlistID string, videoID string) error {
	if _, err := d.sql.ExecContext(ctx, `
		INSERT OR IGNORE INTO collection_videos (collection_id, video_id) VALUES (?, ?)
	`, playlistID, videoID); err != nil {
		return fmt.Errorf("AddToPlaylist: %w", err)
	}
	return nil
}

// RemoveFromPlaylist removes a video from a playlist.
func (d *DB) RemoveFromPlaylist(ctx context.Context, playlistID string, videoID string) error {
	if _, err := d.sql.ExecContext(ctx, `
		DELETE FROM collection_videos WHERE collection_id=? AND video_id=?
	`, playlistID, videoID); err != nil {
		return fmt.Errorf("RemoveFromPlaylist: %w", err)
	}
	return nil
}

// PlaylistVideoIDs returns video IDs in a playlist (needs cross-reference with a video cache).
func (d *DB) PlaylistVideoIDs(ctx context.Context, playlistID string) ([]string, error) {
	ids, err := queryList(ctx, d.sql, `
		SELECT video_id FROM collection_videos
		WHERE collection_id=? ORDER BY rowid
	`, scanString, playlistID)
	if err != nil {
		return nil, fmt.Errorf("PlaylistVideoIDs: %w", err)
	}
	return ids, nil
}

// PlaylistVideos returns full video details for all videos in a playlist.
func (d *DB) PlaylistVideos(ctx context.Context, playlistID string) ([]domain.Video, error) {
	result, err := queryList(ctx, d.sql, `
		SELECT v.id, v.title, COALESCE(v.channel,''), COALESCE(v.channel_id,''),
		       COALESCE(v.duration,0), COALESCE(v.view_count,0),
		       COALESCE(v.upload_date,''), COALESCE(v.url,'')
		FROM collection_videos cv
		JOIN videos v ON v.id = cv.video_id
		WHERE cv.collection_id = ?
		ORDER BY cv.rowid
	`, scanVideoRow, playlistID)
	if err != nil {
		return nil, fmt.Errorf("PlaylistVideos: %w", err)
	}
	return result, nil
}
