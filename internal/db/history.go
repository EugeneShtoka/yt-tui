package db

import (
	"context"
	"database/sql"
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

// activityEvents are the history event types that count as channel activity for
// the stale-tagged-channel filter (playing, streaming, or downloading a video of
// the channel). Kept in sync with the event-type strings written by the TUI
// (internal/tui/app/root.go) and the download reconciler.
var activityEvents = map[string]bool{
	"playVideo": true, "playAudio": true,
	"streamVideo": true, "streamAudio": true,
	evtDownloadVideo: true, evtDownloadAudio: true,
}

// AddHistory records an event. An empty videoID is stored as NULL (search events).
// Play/stream/download events additionally stamp the video's channel as active
// (see StampChannelActivity) so engaging with a video keeps its channel fresh.
func (d *DB) AddHistory(ctx context.Context, videoID, eventType, details string) error {
	var vid interface{}
	if videoID != "" {
		vid = videoID
	}
	if _, err := d.sql.ExecContext(ctx, `
		INSERT INTO history (video_id, event_type, details) VALUES (?, ?, ?)
	`, vid, eventType, details); err != nil {
		return fmt.Errorf("AddHistory: %w", err)
	}
	if videoID != "" && activityEvents[eventType] {
		if _, err := d.sql.ExecContext(ctx, `
			UPDATE subscribed_channels SET last_activity_at=MAX(last_activity_at, ?)
			WHERE channel_id = (SELECT channel_id FROM videos WHERE id=?)
		`, time.Now().Unix(), videoID); err != nil {
			return fmt.Errorf("AddHistory stamp activity: %w", err)
		}
	}
	return nil
}

// SearchQueries returns all unique search queries, newest first.
func (d *DB) SearchQueries(ctx context.Context) ([]string, error) {
	result, err := queryList(ctx, d.sql, `
		SELECT details FROM history
		WHERE event_type = 'search' AND details != ''
		GROUP BY details
		ORDER BY MAX(timestamp) DESC
	`, scanString)
	if err != nil {
		return nil, fmt.Errorf("SearchQueries: %w", err)
	}
	return result, nil
}

// scanHistoryEntry is the queryList scan for the shared 11-column history
// projection (id, video_id, title, channel, channel_id, duration, view_count,
// upload_date, event_type, details, timestamp) that History / HistoryVideos /
// VideoHistory all select.
func scanHistoryEntry(rows *sql.Rows) (domain.HistoryEntry, error) {
	var e domain.HistoryEntry
	err := rows.Scan(
		&e.ID, &e.VideoID, &e.Title,
		&e.Channel, &e.ChannelID, &e.Duration, &e.ViewCount, &e.UploadDate,
		&e.EventType, &e.Details, &e.Timestamp,
	)
	return e, err
}

// HistoryVideos returns the most recent play/stream/download event per video,
// ordered by recency. Search and delete events are excluded.
func (d *DB) HistoryVideos(ctx context.Context, limit int) ([]domain.HistoryEntry, error) {
	result, err := queryList(ctx, d.sql, `
		SELECT h.id, h.video_id, COALESCE(v.title, h.video_id) AS title,
		       COALESCE(v.channel, '') AS channel, COALESCE(v.channel_id, '') AS channel_id,
		       COALESCE(v.duration, 0) AS duration,
		       COALESCE(v.view_count, 0) AS view_count, COALESCE(v.upload_date, '') AS upload_date,
		       h.event_type, COALESCE(h.details,'') AS details, h.timestamp
		FROM history h
		LEFT JOIN videos v ON v.id = h.video_id
		WHERE h.video_id IS NOT NULL
		AND h.event_type NOT IN ('search', 'delete')
		AND h.id = (
		    SELECT h2.id FROM history h2
		    WHERE h2.video_id = h.video_id
		    AND h2.event_type NOT IN ('search', 'delete')
		    ORDER BY h2.timestamp DESC, h2.id DESC
		    LIMIT 1
		)
		ORDER BY h.timestamp DESC
		LIMIT ?
	`, scanHistoryEntry, limit)
	if err != nil {
		return nil, fmt.Errorf("HistoryVideos: %w", err)
	}
	return result, nil
}

// DeleteVideoHistory removes all history events for a video.
func (d *DB) DeleteVideoHistory(ctx context.Context, videoID string) error {
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM history WHERE video_id = ?`, videoID); err != nil {
		return fmt.Errorf("DeleteVideoHistory: %w", err)
	}
	return nil
}

// DeleteSearchHistory removes all history events for a search query.
func (d *DB) DeleteSearchHistory(ctx context.Context, query string) error {
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM history WHERE event_type = 'search' AND details = ?`, query); err != nil {
		return fmt.Errorf("DeleteSearchHistory: %w", err)
	}
	return nil
}

// VideoHistory returns all events for a single video, newest first.
func (d *DB) VideoHistory(ctx context.Context, videoID string) ([]domain.HistoryEntry, error) {
	result, err := queryList(ctx, d.sql, `
		SELECT h.id, h.video_id, COALESCE(v.title, h.video_id),
		       COALESCE(v.channel, ''), COALESCE(v.channel_id, ''), COALESCE(v.duration, 0),
		       COALESCE(v.view_count, 0), COALESCE(v.upload_date, ''),
		       h.event_type, COALESCE(h.details,''), h.timestamp
		FROM history h
		LEFT JOIN videos v ON v.id = h.video_id
		WHERE h.video_id = ?
		ORDER BY h.timestamp DESC
	`, scanHistoryEntry, videoID)
	if err != nil {
		return nil, fmt.Errorf("VideoHistory: %w", err)
	}
	return result, nil
}

// History returns recent history entries with video titles.
func (d *DB) History(ctx context.Context, limit int) ([]domain.HistoryEntry, error) {
	result, err := queryList(ctx, d.sql, `
		SELECT h.id, COALESCE(h.video_id,''), COALESCE(v.title, h.video_id, ''),
		       COALESCE(v.channel, ''), COALESCE(v.channel_id, ''), COALESCE(v.duration, 0),
		       COALESCE(v.view_count, 0), COALESCE(v.upload_date, ''),
		       h.event_type, COALESCE(h.details,''), h.timestamp
		FROM history h
		LEFT JOIN videos v ON v.id=h.video_id
		ORDER BY h.timestamp DESC
		LIMIT ?
	`, scanHistoryEntry, limit)
	if err != nil {
		return nil, fmt.Errorf("History: %w", err)
	}
	return result, nil
}

// ClearHistory removes all history entries.
func (d *DB) ClearHistory(ctx context.Context) error {
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM history`); err != nil {
		return fmt.Errorf("ClearHistory: %w", err)
	}
	return nil
}

// LogActivity records a user action in the activity log.
func (d *DB) LogActivity(ctx context.Context, e domain.ActivityEntry) error {
	isLocal := 0
	if e.IsLocal {
		isLocal = 1
	}
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
func (d *DB) GetActivityLog(ctx context.Context, limit int) ([]domain.ActivityEntry, error) {
	entries, err := queryList(ctx, d.sql, `
		SELECT id, type, is_local,
		       COALESCE(channel_id,''), COALESCE(channel_name,''),
		       COALESCE(playlist_id,''), COALESCE(playlist_local_id,0), COALESCE(playlist_name,''),
		       COALESCE(video_id,''), COALESCE(video_title,''), timestamp
		FROM activity_log ORDER BY timestamp DESC LIMIT ?
	`, func(rows *sql.Rows) (domain.ActivityEntry, error) {
		var e domain.ActivityEntry
		var isLocal int
		if err := rows.Scan(&e.ID, &e.Type, &isLocal,
			&e.ChannelID, &e.ChannelName,
			&e.PlaylistID, &e.PlaylistLocalID, &e.PlaylistName,
			&e.VideoID, &e.VideoTitle, &e.Timestamp); err != nil {
			return e, err
		}
		e.IsLocal = isLocal != 0
		return e, nil
	}, limit)
	if err != nil {
		return nil, fmt.Errorf("GetActivityLog: %w", err)
	}
	return entries, nil
}
