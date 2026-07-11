package db

import "time"

// HistoryEntry is a single event from the history or activity log.
type HistoryEntry struct {
	ID         int64
	VideoID    string
	Title      string
	Channel    string
	ChannelID  string
	Duration   int
	ViewCount  int64
	UploadDate string
	EventType  string
	Details    string
	Timestamp  time.Time
}

// ActivityEntry records a user action such as subscribe or playlist modification.
type ActivityEntry struct {
	ID              int64
	Type            string // "subscribe", "create_playlist", "add_to_playlist"
	IsLocal         bool
	ChannelID       string
	ChannelName     string
	PlaylistID      string // YT playlist ID
	PlaylistLocalID int64  // local playlist DB ID
	PlaylistName    string
	VideoID         string
	VideoTitle      string
	Timestamp       time.Time
}

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
	_, err := d.sql.Exec(`
		INSERT INTO history (video_id, event_type, details) VALUES (?, ?, ?)
	`, vid, eventType, details)
	return err
}

// SearchQueries returns recent unique search queries, newest first.
func (d *DB) SearchQueries(limit int) ([]string, error) {
	rows, err := d.sql.Query(`
		SELECT details FROM history
		WHERE event_type = 'search' AND details != ''
		GROUP BY details
		ORDER BY MAX(timestamp) DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var q string
		if err := rows.Scan(&q); err != nil {
			return nil, err
		}
		result = append(result, q)
	}
	return result, rows.Err()
}

// HistoryVideos returns one entry per video (most recent event) plus one entry per
// unique search query, all ordered by recency.
func (d *DB) HistoryVideos(limit int) ([]HistoryEntry, error) {
	rows, err := d.sql.Query(`
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
		return nil, err
	}
	defer rows.Close()
	var result []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		var tsStr string
		if err := rows.Scan(
			&e.ID, &e.VideoID, &e.Title,
			&e.Channel, &e.ChannelID, &e.Duration, &e.ViewCount, &e.UploadDate,
			&e.EventType, &e.Details, &tsStr,
		); err != nil {
			return nil, err
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
	return result, rows.Err()
}

// DeleteVideoHistory removes all history events for a video.
func (d *DB) DeleteVideoHistory(videoID string) error {
	_, err := d.sql.Exec(`DELETE FROM history WHERE video_id = ?`, videoID)
	return err
}

// DeleteSearchHistory removes all history events for a search query.
func (d *DB) DeleteSearchHistory(query string) error {
	_, err := d.sql.Exec(`DELETE FROM history WHERE event_type = 'search' AND details = ?`, query)
	return err
}

// VideoHistory returns all events for a single video, newest first.
func (d *DB) VideoHistory(videoID string) ([]HistoryEntry, error) {
	rows, err := d.sql.Query(`
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
		return nil, err
	}
	defer rows.Close()
	var result []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(
			&e.ID, &e.VideoID, &e.Title,
			&e.Channel, &e.ChannelID, &e.Duration, &e.ViewCount, &e.UploadDate,
			&e.EventType, &e.Details, &e.Timestamp,
		); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// History returns recent history entries with video titles.
func (d *DB) History(limit int) ([]HistoryEntry, error) {
	rows, err := d.sql.Query(`
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
		return nil, err
	}
	defer rows.Close()
	var result []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(
			&e.ID, &e.VideoID, &e.Title,
			&e.Channel, &e.ChannelID, &e.Duration, &e.ViewCount, &e.UploadDate,
			&e.EventType, &e.Details, &e.Timestamp,
		); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// ClearHistory removes all history entries.
func (d *DB) ClearHistory() error {
	_, err := d.sql.Exec(`DELETE FROM history`)
	return err
}

// LogActivity records a user action in the activity log.
func (d *DB) LogActivity(e ActivityEntry) error {
	isLocal := 0
	if e.IsLocal {
		isLocal = 1
	}
	_, err := d.sql.Exec(`
		INSERT INTO activity_log
			(type, is_local, channel_id, channel_name, playlist_id, playlist_local_id, playlist_name, video_id, video_title)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, e.Type, isLocal, e.ChannelID, e.ChannelName, e.PlaylistID, nullInt64(e.PlaylistLocalID), e.PlaylistName, e.VideoID, e.VideoTitle)
	return err
}

// GetActivityLog returns the most recent activity entries, newest first.
func (d *DB) GetActivityLog(limit int) ([]ActivityEntry, error) {
	rows, err := d.sql.Query(`
		SELECT id, type, is_local,
		       COALESCE(channel_id,''), COALESCE(channel_name,''),
		       COALESCE(playlist_id,''), COALESCE(playlist_local_id,0), COALESCE(playlist_name,''),
		       COALESCE(video_id,''), COALESCE(video_title,''), timestamp
		FROM activity_log ORDER BY timestamp DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []ActivityEntry
	for rows.Next() {
		var e ActivityEntry
		var isLocal int
		if err := rows.Scan(&e.ID, &e.Type, &isLocal,
			&e.ChannelID, &e.ChannelName,
			&e.PlaylistID, &e.PlaylistLocalID, &e.PlaylistName,
			&e.VideoID, &e.VideoTitle, &e.Timestamp); err != nil {
			return nil, err
		}
		e.IsLocal = isLocal != 0
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
