package db

import (
	"context"
	"fmt"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// nullInt64 stores 0 as SQL NULL (used for optional foreign-key IDs).
func nullInt64(v int64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

// AddHistory records an event. An empty videoID is stored as NULL (search events).
func (d *DB) AddHistory(videoID, eventType, details string) error {
	var vid interface{}
	if videoID != "" {
		vid = videoID
	}
	ctx := context.Background()
	if _, err := d.sql.ExecContext(ctx, `
		INSERT INTO history (video_id, event_type, details) VALUES (?, ?, ?)
	`, vid, eventType, details); err != nil {
		return fmt.Errorf("AddHistory: %w", err)
	}
	return nil
}

// SearchQueries returns recent unique search queries, newest first.
func (d *DB) SearchQueries(limit int) ([]string, error) {
	ctx := context.Background()
	rows, err := d.sql.QueryContext(ctx, `
		SELECT details FROM history
		WHERE event_type = 'search' AND details != ''
		GROUP BY details
		ORDER BY MAX(timestamp) DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("SearchQueries query: %w", err)
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var q string
		if err := rows.Scan(&q); err != nil {
			return nil, fmt.Errorf("SearchQueries scan: %w", err)
		}
		result = append(result, q)
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("SearchQueries rows: %w", err)
	}
	return result, nil
}

// HistoryVideos returns one entry per video (most recent event) plus one entry per
// unique search query, all ordered by recency.
func (d *DB) HistoryVideos(limit int) ([]domain.HistoryEntry, error) {
	ctx := context.Background()
	rows, err := d.sql.QueryContext(ctx, `
		SELECT * FROM (
			SELECT h.id, h.video_id, COALESCE(v.title, h.video_id) AS title,
			       COALESCE(v.channel, '') AS channel, COALESCE(v.channel_id, '') AS channel_id,
			       COALESCE(v.duration, 0) AS duration,
			       COALESCE(v.view_count, 0) AS view_count, COALESCE(v.upload_date, '') AS upload_date,
			       h.event_type, COALESCE(h.details,'') AS details, h.timestamp
			FROM history h
			LEFT JOIN videos v ON v.id = h.video_id
			WHERE h.video_id IS NOT NULL
			AND h.id = (
			    SELECT h2.id FROM history h2
			    WHERE h2.video_id = h.video_id
			    ORDER BY h2.timestamp DESC, h2.id DESC
			    LIMIT 1
			)
			GROUP BY h.video_id

			UNION ALL

			SELECT MAX(h.id), '', h.details, '', '', 0, 0, '', h.event_type, h.details, MAX(h.timestamp)
			FROM history h
			WHERE h.event_type = 'search' AND h.details != ''
			GROUP BY h.details
		) ORDER BY timestamp DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("HistoryVideos query: %w", err)
	}
	defer rows.Close()
	var result []domain.HistoryEntry
	for rows.Next() {
		var e domain.HistoryEntry
		var tsStr string
		if err := rows.Scan(
			&e.ID, &e.VideoID, &e.Title,
			&e.Channel, &e.ChannelID, &e.Duration, &e.ViewCount, &e.UploadDate,
			&e.EventType, &e.Details, &tsStr,
		); err != nil {
			return nil, fmt.Errorf("HistoryVideos scan: %w", err)
		}
		// UNION ALL causes the driver to return timestamps as strings.
		for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05Z"} {
			if t, err := time.Parse(layout, tsStr); err == nil {
				e.Timestamp = t
				break
			}
		}
		result = append(result, e)
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("HistoryVideos rows: %w", err)
	}
	return result, nil
}

// DeleteVideoHistory removes all history events for a video.
func (d *DB) DeleteVideoHistory(videoID string) error {
	ctx := context.Background()
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM history WHERE video_id = ?`, videoID); err != nil {
		return fmt.Errorf("DeleteVideoHistory: %w", err)
	}
	return nil
}

// DeleteSearchHistory removes all history events for a search query.
func (d *DB) DeleteSearchHistory(query string) error {
	ctx := context.Background()
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM history WHERE event_type = 'search' AND details = ?`, query); err != nil {
		return fmt.Errorf("DeleteSearchHistory: %w", err)
	}
	return nil
}

// VideoHistory returns all events for a single video, newest first.
func (d *DB) VideoHistory(videoID string) ([]domain.HistoryEntry, error) {
	ctx := context.Background()
	rows, err := d.sql.QueryContext(ctx, `
		SELECT h.id, h.video_id, COALESCE(v.title, h.video_id),
		       COALESCE(v.channel, ''), COALESCE(v.channel_id, ''), COALESCE(v.duration, 0),
		       COALESCE(v.view_count, 0), COALESCE(v.upload_date, ''),
		       h.event_type, COALESCE(h.details,''), h.timestamp
		FROM history h
		LEFT JOIN videos v ON v.id = h.video_id
		WHERE h.video_id = ?
		ORDER BY h.timestamp DESC
	`, videoID)
	if err != nil {
		return nil, fmt.Errorf("VideoHistory query: %w", err)
	}
	defer rows.Close()
	var result []domain.HistoryEntry
	for rows.Next() {
		var e domain.HistoryEntry
		if err := rows.Scan(
			&e.ID, &e.VideoID, &e.Title,
			&e.Channel, &e.ChannelID, &e.Duration, &e.ViewCount, &e.UploadDate,
			&e.EventType, &e.Details, &e.Timestamp,
		); err != nil {
			return nil, fmt.Errorf("VideoHistory scan: %w", err)
		}
		result = append(result, e)
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("VideoHistory rows: %w", err)
	}
	return result, nil
}

// History returns recent history entries with video titles.
func (d *DB) History(limit int) ([]domain.HistoryEntry, error) {
	ctx := context.Background()
	rows, err := d.sql.QueryContext(ctx, `
		SELECT h.id, h.video_id, COALESCE(v.title, h.video_id),
		       COALESCE(v.channel, ''), COALESCE(v.channel_id, ''), COALESCE(v.duration, 0),
		       COALESCE(v.view_count, 0), COALESCE(v.upload_date, ''),
		       h.event_type, COALESCE(h.details,''), h.timestamp
		FROM history h
		LEFT JOIN videos v ON v.id=h.video_id
		ORDER BY h.timestamp DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("History query: %w", err)
	}
	defer rows.Close()
	var result []domain.HistoryEntry
	for rows.Next() {
		var e domain.HistoryEntry
		if err := rows.Scan(
			&e.ID, &e.VideoID, &e.Title,
			&e.Channel, &e.ChannelID, &e.Duration, &e.ViewCount, &e.UploadDate,
			&e.EventType, &e.Details, &e.Timestamp,
		); err != nil {
			return nil, fmt.Errorf("History scan: %w", err)
		}
		result = append(result, e)
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("History rows: %w", err)
	}
	return result, nil
}

// ClearHistory removes all history entries.
func (d *DB) ClearHistory() error {
	ctx := context.Background()
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM history`); err != nil {
		return fmt.Errorf("ClearHistory: %w", err)
	}
	return nil
}

// LogActivity records a user action in the activity log.
func (d *DB) LogActivity(e domain.ActivityEntry) error {
	isLocal := 0
	if e.IsLocal {
		isLocal = 1
	}
	ctx := context.Background()
	if _, err := d.sql.ExecContext(ctx, `
		INSERT INTO activity_log
			(type, is_local, channel_id, channel_name, playlist_id, playlist_local_id, playlist_name, video_id, video_title)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, e.Type, isLocal, e.ChannelID, e.ChannelName, e.PlaylistID, nullInt64(e.PlaylistLocalID), e.PlaylistName, e.VideoID, e.VideoTitle); err != nil {
		return fmt.Errorf("LogActivity: %w", err)
	}
	return nil
}

// GetActivityLog returns the most recent activity entries, newest first.
func (d *DB) GetActivityLog(limit int) ([]domain.ActivityEntry, error) {
	ctx := context.Background()
	rows, err := d.sql.QueryContext(ctx, `
		SELECT id, type, is_local,
		       COALESCE(channel_id,''), COALESCE(channel_name,''),
		       COALESCE(playlist_id,''), COALESCE(playlist_local_id,0), COALESCE(playlist_name,''),
		       COALESCE(video_id,''), COALESCE(video_title,''), timestamp
		FROM activity_log ORDER BY timestamp DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("GetActivityLog query: %w", err)
	}
	defer rows.Close()
	var entries []domain.ActivityEntry
	for rows.Next() {
		var e domain.ActivityEntry
		var isLocal int
		if err := rows.Scan(&e.ID, &e.Type, &isLocal,
			&e.ChannelID, &e.ChannelName,
			&e.PlaylistID, &e.PlaylistLocalID, &e.PlaylistName,
			&e.VideoID, &e.VideoTitle, &e.Timestamp); err != nil {
			return nil, fmt.Errorf("GetActivityLog scan: %w", err)
		}
		e.IsLocal = isLocal != 0
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return entries, fmt.Errorf("GetActivityLog rows: %w", err)
	}
	return entries, nil
}
