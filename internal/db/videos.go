package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// execer is satisfied by both *sql.DB and *sql.Tx, letting upsertVideoTx run
// either standalone or as part of a larger transaction.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// querier is satisfied by both *sql.DB and *sql.Tx, letting queryList read either
// standalone or inside a transaction.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// queryList runs query and collects every row through scan, centralizing the
// QueryContext → rows.Close → for rows.Next → rows.Err envelope that each list
// query used to hand-roll (a copy-paste seam that once dropped a rows.Err check).
// Errors are returned unwrapped; each call site adds its own context so messages
// stay specific.
func queryList[T any](ctx context.Context, qr querier, query string, scan func(*sql.Rows) (T, error), args ...any) ([]T, error) {
	rows, err := qr.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err //nolint:wrapcheck // call sites wrap with their own context
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err //nolint:wrapcheck // call sites wrap with their own context
		}
		out = append(out, v)
	}
	return out, rows.Err() //nolint:wrapcheck // call sites wrap with their own context
}

// queryMap is the map-valued analog of queryList: it runs query and collects
// every row through scan (which returns a key and value) into a map, centralizing
// the same QueryContext → rows.Close → for rows.Next → rows.Err envelope. Errors
// are returned unwrapped; each call site adds its own context.
func queryMap[K comparable, V any](ctx context.Context, qr querier, query string, scan func(*sql.Rows) (K, V, error), args ...any) (map[K]V, error) {
	rows, err := qr.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err //nolint:wrapcheck // call sites wrap with their own context
	}
	defer rows.Close()
	out := make(map[K]V)
	for rows.Next() {
		k, v, err := scan(rows)
		if err != nil {
			return nil, err //nolint:wrapcheck // call sites wrap with their own context
		}
		out[k] = v
	}
	return out, rows.Err() //nolint:wrapcheck // call sites wrap with their own context
}

// scanVideoRow adapts scanVideo (variadic) to the queryList scan signature for
// the plain 8-column video projection with no leading columns.
func scanVideoRow(rows *sql.Rows) (domain.Video, error) { return scanVideo(rows) }

// scanString is the queryList scan for a single string column (id/name/query lists).
func scanString(rows *sql.Rows) (string, error) {
	var s string
	err := rows.Scan(&s)
	return s, err
}

// upsertVideoTx is the single video upsert used by every write path (UpsertVideo,
// SaveChannelVideos, SaveFeedCache, SaveYTPlaylistVideos). Centralizing it means
// every caller updates every column — a previous copy in SaveFeedCache omitted
// url=excluded.url, so a video's URL silently never refreshed via that path.
func upsertVideoTx(ctx context.Context, ex execer, v domain.Video) error {
	if _, err := ex.ExecContext(ctx, `
		INSERT INTO videos (id, title, channel, channel_id, duration, view_count, upload_date, url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title=excluded.title, channel=excluded.channel,
			channel_id=COALESCE(NULLIF(excluded.channel_id,''), channel_id),
			duration=excluded.duration, view_count=excluded.view_count,
			upload_date=excluded.upload_date, url=excluded.url
	`, v.ID, v.Title, v.Channel, v.ChannelID, v.Duration, v.ViewCount, v.UploadDate, v.URL); err != nil {
		return fmt.Errorf("upsertVideoTx: %w", err)
	}
	return nil
}

// scanVideo scans the standard 8-column video projection
// (id, title, channel, channel_id, duration, view_count, upload_date, url) that
// every video-listing query shares. leading receives any columns selected
// before those eight (e.g. a grouping key).
func scanVideo(rows *sql.Rows, leading ...any) (domain.Video, error) {
	var v domain.Video
	dest := append(append([]any{}, leading...),
		&v.ID, &v.Title, &v.Channel, &v.ChannelID,
		&v.Duration, &v.ViewCount, &v.UploadDate, &v.URL)
	if err := rows.Scan(dest...); err != nil {
		return domain.Video{}, err //nolint:wrapcheck // every call site wraps with its own context
	}
	return v, nil
}

// UpsertVideo inserts or updates a video record.
func (d *DB) UpsertVideo(ctx context.Context, id, title, channel, channelID string, duration int, viewCount int64, uploadDate, url string) error {
	if err := upsertVideoTx(ctx, d.sql, domain.Video{
		ID: id, Title: title, Channel: channel, ChannelID: channelID,
		Duration: duration, ViewCount: viewCount, UploadDate: uploadDate, URL: url,
	}); err != nil {
		return fmt.Errorf("UpsertVideo: %w", err)
	}
	return nil
}

// UpdateVideoUploadDate overwrites just the upload_date of an existing video,
// leaving every other column untouched. Used by the lazy/background enrichment
// path to replace yt-dlp's approximate flat-listing date with the exact date
// from a full metadata fetch. Updating a non-existent id is a silent no-op.
func (d *DB) UpdateVideoUploadDate(ctx context.Context, videoID, uploadDate string) error {
	if _, err := d.sql.ExecContext(ctx,
		`UPDATE videos SET upload_date=? WHERE id=?`, uploadDate, videoID); err != nil {
		return fmt.Errorf("UpdateVideoUploadDate: %w", err)
	}
	return nil
}

// VideoDuration returns a video's duration in seconds, or 0 if the row is absent
// or has no duration. Used by the watched-percent auto-remove check.
func (d *DB) VideoDuration(ctx context.Context, id string) (int, error) {
	var dur int
	err := d.sql.QueryRowContext(ctx, `SELECT COALESCE(duration,0) FROM videos WHERE id=?`, id).Scan(&dur)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("VideoDuration: %w", err)
	}
	return dur, nil
}

// AddLocalVideo records a downloaded video.
func (d *DB) AddLocalVideo(ctx context.Context, v domain.LocalVideo) error {
	if _, err := d.sql.ExecContext(ctx, `
		INSERT INTO local_videos (id, file_path, file_size, download_type, downloaded_at, status)
		VALUES (?, ?, ?, ?, ?, 'new')
		ON CONFLICT(id) DO UPDATE SET file_path=excluded.file_path, file_size=excluded.file_size, download_type=excluded.download_type
	`, v.ID, v.FilePath, v.FileSize, v.DownloadType, v.DownloadedAt); err != nil {
		return fmt.Errorf("AddLocalVideo: %w", err)
	}
	return nil
}

// SetVideoStatus updates playback status.
func (d *DB) SetVideoStatus(ctx context.Context, id string, status domain.VideoStatus) error {
	now := time.Now()
	if _, err := d.sql.ExecContext(ctx, `
		UPDATE local_videos SET status=?, last_played=? WHERE id=?
	`, string(status), now, id); err != nil {
		return fmt.Errorf("SetVideoStatus: %w", err)
	}
	return nil
}

// DeleteLocalVideo removes a local video record.
func (d *DB) DeleteLocalVideo(ctx context.Context, id string) error {
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM local_videos WHERE id=?`, id); err != nil {
		return fmt.Errorf("DeleteLocalVideo: %w", err)
	}
	return nil
}

// LocalVideos returns all downloaded videos ordered by download date.
func (d *DB) LocalVideos(ctx context.Context) ([]domain.LocalVideo, error) {
	result, err := queryList(ctx, d.sql, `
		SELECT lv.id, v.title, v.channel, v.duration,
		       COALESCE(v.view_count, 0), COALESCE(v.upload_date, ''),
		       lv.file_path, COALESCE(lv.file_size, 0), lv.download_type, lv.downloaded_at, lv.status,
		       lv.last_played, COALESCE(lv.last_position_ms, 0)
		FROM local_videos lv
		JOIN videos v ON v.id = lv.id
		ORDER BY lv.downloaded_at DESC
	`, func(rows *sql.Rows) (domain.LocalVideo, error) {
		var lv domain.LocalVideo
		var lastPlayed sql.NullTime
		if err := rows.Scan(
			&lv.ID, &lv.Title, &lv.Channel, &lv.Duration,
			&lv.ViewCount, &lv.UploadDate,
			&lv.FilePath, &lv.FileSize, &lv.DownloadType, &lv.DownloadedAt,
			&lv.Status, &lastPlayed, &lv.LastPositionMs,
		); err != nil {
			return lv, err
		}
		if lastPlayed.Valid {
			lv.LastPlayed = lastPlayed.Time
		}
		return lv, nil
	})
	if err != nil {
		return nil, fmt.Errorf("LocalVideos: %w", err)
	}
	return result, nil
}

// AllVideoPositions returns all saved positions as a map of videoID → position_ms.
func (d *DB) AllVideoPositions(ctx context.Context) (map[string]int64, error) {
	m, err := queryMap(ctx, d.sql, `SELECT video_id, position_ms FROM video_positions WHERE position_ms > 0`,
		func(rows *sql.Rows) (string, int64, error) {
			var id string
			var ms int64
			err := rows.Scan(&id, &ms)
			return id, ms, err
		})
	if err != nil {
		return nil, fmt.Errorf("AllVideoPositions: %w", err)
	}
	return m, nil
}

// SaveVideoPosition upserts the last known playback position for any video (local or streamed).
func (d *DB) SaveVideoPosition(ctx context.Context, videoID string, ms int64) error {
	if _, err := d.sql.ExecContext(ctx, `
		INSERT INTO video_positions (video_id, position_ms, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(video_id) DO UPDATE SET position_ms=excluded.position_ms, updated_at=excluded.updated_at
	`, videoID, ms); err != nil {
		return fmt.Errorf("SaveVideoPosition: %w", err)
	}
	return nil
}

// DeleteVideoPosition removes the saved playback position for a video.
func (d *DB) DeleteVideoPosition(ctx context.Context, videoID string) error {
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM video_positions WHERE video_id = ?`, videoID); err != nil {
		return fmt.Errorf("DeleteVideoPosition: %w", err)
	}
	return nil
}

// VideoPosition returns the last saved position for any video, or (0, false,
// nil) if none is saved. A non-nil error means the lookup itself failed and
// must not be treated the same as "no position saved". (H-8)
func (d *DB) VideoPosition(ctx context.Context, videoID string) (int64, bool, error) {
	var ms int64
	err := d.sql.QueryRowContext(ctx, `SELECT position_ms FROM video_positions WHERE video_id=?`, videoID).Scan(&ms)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("VideoPosition: %w", err)
	}
	if ms == 0 {
		return 0, false, nil
	}
	return ms, true, nil
}

// WatchedVideoIDs returns the set of video IDs that have any play or stream history event.
func (d *DB) WatchedVideoIDs(ctx context.Context) (map[string]bool, error) {
	ids, err := queryMap(ctx, d.sql, `
		SELECT DISTINCT video_id FROM history
		WHERE event_type IN ('playVideo','playAudio','streamVideo','streamAudio')
		AND video_id != ''
	`, func(rows *sql.Rows) (string, bool, error) {
		var id string
		err := rows.Scan(&id)
		return id, true, err
	})
	if err != nil {
		return nil, fmt.Errorf("WatchedVideoIDs: %w", err)
	}
	return ids, nil
}

// UpdateLastPosition saves the last known playback position for a local video.
func (d *DB) UpdateLastPosition(ctx context.Context, id string, ms int64) error {
	return d.SaveVideoPosition(ctx, id, ms)
}

// HasLocalVideo returns the local video record if it exists. A non-nil error
// means the lookup itself failed and must not be treated the same as "not a
// local video". (H-8)
func (d *DB) HasLocalVideo(ctx context.Context, id string) (domain.LocalVideo, bool, error) {
	var lv domain.LocalVideo
	var lastPlayed sql.NullTime
	err := d.sql.QueryRowContext(ctx, `
		SELECT lv.id, v.title, v.channel, v.duration,
		       COALESCE(v.view_count, 0), COALESCE(v.upload_date, ''),
		       lv.file_path, COALESCE(lv.file_size, 0), lv.download_type, lv.downloaded_at, lv.status,
		       lv.last_played, COALESCE(lv.last_position_ms, 0)
		FROM local_videos lv JOIN videos v ON v.id=lv.id
		WHERE lv.id=?
	`, id).Scan(
		&lv.ID, &lv.Title, &lv.Channel, &lv.Duration,
		&lv.ViewCount, &lv.UploadDate,
		&lv.FilePath, &lv.FileSize, &lv.DownloadType, &lv.DownloadedAt,
		&lv.Status, &lastPlayed, &lv.LastPositionMs,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.LocalVideo{}, false, nil
	}
	if err != nil {
		return domain.LocalVideo{}, false, fmt.Errorf("HasLocalVideo: %w", err)
	}
	if lastPlayed.Valid {
		lv.LastPlayed = lastPlayed.Time
	}
	return lv, true, nil
}
