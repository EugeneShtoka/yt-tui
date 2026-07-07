package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/youtube"
	_ "modernc.org/sqlite"
)

type VideoStatus string

const (
	StatusNew     VideoStatus = "new"
	StatusStarted VideoStatus = "started"
	StatusWatched VideoStatus = "watched"
)

type LocalVideo struct {
	ID             string
	Title          string
	Channel        string
	Duration       int
	ViewCount      int64
	UploadDate     string
	FilePath       string
	DownloadType   string // "video" or "audio"
	DownloadedAt   time.Time
	Status         VideoStatus
	LastPlayed     time.Time
	LastPositionMs int64
}

type Playlist struct {
	ID        int64
	Name      string
	CreatedAt time.Time
}

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

type DB struct {
	sql *sql.DB
}

func New(dataDir string, stripEmojis bool) (*DB, error) {
	path := filepath.Join(dataDir, "yt-tui.db")
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Single connection serializes all writes; prevents SQLITE_BUSY from concurrent goroutines.
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`); err != nil {
		return nil, err
	}
	d := &DB{sql: sqlDB}
	if err := d.migrate(); err != nil {
		return nil, err
	}
	if stripEmojis {
		if err := d.cleanEmojiTitles(); err != nil {
			return nil, err
		}
	}
	if err := d.deleteMemberVideos(); err != nil {
		return nil, err
	}
	if err := d.pruneRecommendedFeed(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *DB) Close() error {
	return d.sql.Close()
}

func (d *DB) migrate() error {
	_, err := d.sql.Exec(`
		CREATE TABLE IF NOT EXISTS videos (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			channel TEXT,
			channel_id TEXT,
			duration INTEGER DEFAULT 0,
			view_count INTEGER DEFAULT 0,
			upload_date TEXT,
			url TEXT,
			added_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS local_videos (
			id TEXT PRIMARY KEY REFERENCES videos(id),
			file_path TEXT NOT NULL,
			download_type TEXT DEFAULT 'video',
			downloaded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			status TEXT DEFAULT 'new',
			last_played DATETIME
		);

		CREATE TABLE IF NOT EXISTS history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			video_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			details TEXT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS playlists (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS playlist_videos (
			playlist_id INTEGER REFERENCES playlists(id) ON DELETE CASCADE,
			video_id TEXT REFERENCES videos(id),
			position INTEGER DEFAULT 0,
			added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (playlist_id, video_id)
		);

		CREATE TABLE IF NOT EXISTS watch_later (
			video_id TEXT PRIMARY KEY,
			title TEXT,
			channel TEXT,
			url TEXT,
			added_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS feed_cache (
			feed TEXT NOT NULL,
			video_id TEXT NOT NULL,
			position INTEGER NOT NULL,
			PRIMARY KEY (feed, video_id)
		);

		CREATE INDEX IF NOT EXISTS idx_feed_cache_feed ON feed_cache(feed, position);
		CREATE INDEX IF NOT EXISTS idx_history_timestamp ON history(timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_history_video ON history(video_id);
		CREATE INDEX IF NOT EXISTS idx_videos_upload_date ON videos(upload_date DESC);

		CREATE TABLE IF NOT EXISTS hidden_rec_videos (
			video_id TEXT PRIMARY KEY,
			hidden_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return err
	}
	// Columns added after initial schema; safe to ignore "duplicate column" errors.
	for _, col := range []string{
		`ALTER TABLE local_videos ADD COLUMN last_position_ms INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE subscribed_channels ADD COLUMN name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE subscribed_channels ADD COLUMN url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE subscribed_channels ADD COLUMN subscribers INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE subscribed_channels ADD COLUMN alias TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE subscribed_channels ADD COLUMN tags TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE subscribed_channels ADD COLUMN is_local INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err = d.sql.Exec(col); err != nil && !isColumnExists(err) {
			return err
		}
	}
	// Tables added after initial schema.
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS subscribed_channels (
			channel_id TEXT PRIMARY KEY,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS channel_videos (
			channel_id TEXT NOT NULL,
			video_id   TEXT NOT NULL REFERENCES videos(id),
			fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (channel_id, video_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_channel_videos_channel ON channel_videos(channel_id, video_id)`,
		`CREATE TABLE IF NOT EXISTS yt_playlists (
			id         TEXT PRIMARY KEY,
			title      TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS yt_playlist_videos (
			playlist_id TEXT NOT NULL,
			video_id    TEXT NOT NULL REFERENCES videos(id),
			position    INTEGER DEFAULT 0,
			fetched_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (playlist_id, video_id)
		)`,
		`CREATE TABLE IF NOT EXISTS video_details_cache (
			video_id      TEXT PRIMARY KEY,
			description   TEXT NOT NULL DEFAULT '',
			thumbnail_url TEXT NOT NULL DEFAULT '',
			subscribers   INTEGER NOT NULL DEFAULT 0,
			fetched_at    DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS activity_log (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			type             TEXT NOT NULL,
			is_local         INTEGER NOT NULL DEFAULT 0,
			channel_id       TEXT,
			channel_name     TEXT,
			playlist_id      TEXT,
			playlist_local_id INTEGER,
			playlist_name    TEXT,
			video_id         TEXT,
			video_title      TEXT,
			timestamp        DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	} {
		if _, err = d.sql.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) cleanEmojiTitles() error {
	type tableCol struct{ table, idCol, titleCol string }
	targets := []tableCol{
		{"videos", "id", "title"},
		{"watch_later", "video_id", "title"},
		{"yt_playlists", "id", "title"},
	}
	for _, t := range targets {
		rows, err := d.sql.Query("SELECT " + t.idCol + ", " + t.titleCol + " FROM " + t.table)
		if err != nil {
			return err
		}
		type row struct{ id, title string }
		var updates []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.title); err != nil {
				rows.Close()
				return err
			}
			if clean := youtube.StripEmojis(r.title); clean != r.title {
				updates = append(updates, row{r.id, clean})
			}
		}
		rows.Close()
		for _, u := range updates {
			if _, err := d.sql.Exec("UPDATE "+t.table+" SET "+t.titleCol+"=? WHERE "+t.idCol+"=?", u.title, u.id); err != nil {
				return err
			}
		}
	}
	return nil
}

// deleteMemberVideos removes member-only videos (view_count=0) from the DB.
// Videos that have been downloaded (present in local_videos) are preserved.
func (d *DB) deleteMemberVideos() error {
	for _, stmt := range []string{
		`DELETE FROM feed_cache WHERE video_id IN (SELECT id FROM videos WHERE view_count=0 AND id NOT IN (SELECT id FROM local_videos))`,
		`DELETE FROM channel_videos WHERE video_id IN (SELECT id FROM videos WHERE view_count=0 AND id NOT IN (SELECT id FROM local_videos))`,
		`DELETE FROM yt_playlist_videos WHERE video_id IN (SELECT id FROM videos WHERE view_count=0 AND id NOT IN (SELECT id FROM local_videos))`,
		`DELETE FROM playlist_videos WHERE video_id IN (SELECT id FROM videos WHERE view_count=0 AND id NOT IN (SELECT id FROM local_videos))`,
		`DELETE FROM hidden_rec_videos WHERE video_id IN (SELECT id FROM videos WHERE view_count=0 AND id NOT IN (SELECT id FROM local_videos))`,
		`DELETE FROM video_details_cache WHERE video_id IN (SELECT id FROM videos WHERE view_count=0 AND id NOT IN (SELECT id FROM local_videos))`,
		`DELETE FROM videos WHERE view_count=0 AND id NOT IN (SELECT id FROM local_videos)`,
	} {
		if _, err := d.sql.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// SaveChannelVideos upserts all videos for a channel and links them.
func (d *DB) SaveChannelVideos(channelID string, videos []youtube.Video) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, v := range videos {
		if _, err := tx.Exec(`
			INSERT INTO videos (id, title, channel, channel_id, duration, view_count, upload_date, url)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				title=excluded.title, channel=excluded.channel,
				channel_id=COALESCE(NULLIF(excluded.channel_id,''), channel_id),
				duration=excluded.duration, view_count=excluded.view_count,
				upload_date=excluded.upload_date, url=excluded.url
		`, v.ID, v.Title, v.Channel, v.ChannelID, v.Duration, v.ViewCount, v.UploadDate, v.URL); err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO channel_videos (channel_id, video_id) VALUES (?, ?)
		`, channelID, v.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetChannelVideos returns persisted videos for a channel, newest first.
func (d *DB) GetChannelVideos(channelID string) ([]youtube.Video, error) {
	rows, err := d.sql.Query(`
		SELECT v.id, v.title, COALESCE(v.channel,''), COALESCE(v.channel_id,''),
		       COALESCE(v.duration,0), COALESCE(v.view_count,0),
		       COALESCE(v.upload_date,''), COALESCE(v.url,'')
		FROM channel_videos cv
		JOIN videos v ON v.id = cv.video_id
		WHERE cv.channel_id = ?
		ORDER BY v.upload_date DESC
	`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []youtube.Video
	for rows.Next() {
		var v youtube.Video
		if err := rows.Scan(&v.ID, &v.Title, &v.Channel, &v.ChannelID,
			&v.Duration, &v.ViewCount, &v.UploadDate, &v.URL); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// SaveSubscribedChannels persists the full channel list, preserving alias and tags for existing channels.
// Only non-local (YT-subscribed) channels can be removed; local subscriptions are preserved.
func (d *DB) SaveSubscribedChannels(channels []youtube.Channel) error {
	if len(channels) == 0 {
		return nil
	}
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Remove YT-managed channels that are no longer subscribed (preserve is_local=1 entries).
	ph := make([]string, len(channels))
	ids := make([]interface{}, len(channels))
	for i, ch := range channels {
		ph[i] = "?"
		ids[i] = ch.ID
	}
	if _, err := tx.Exec(
		`DELETE FROM subscribed_channels WHERE is_local=0 AND channel_id NOT IN (`+strings.Join(ph, ",")+`)`,
		ids...,
	); err != nil {
		return err
	}
	// Upsert — alias and tags are intentionally excluded from the UPDATE SET so they are preserved.
	for _, ch := range channels {
		if ch.ID == "" {
			continue
		}
		if _, err := tx.Exec(`
			INSERT INTO subscribed_channels (channel_id, name, url, subscribers, is_local)
			VALUES (?, ?, ?, ?, 0)
			ON CONFLICT(channel_id) DO UPDATE SET
				name=excluded.name, url=excluded.url,
				subscribers=excluded.subscribers,
				is_local=0,
				updated_at=CURRENT_TIMESTAMP
		`, ch.ID, ch.Name, ch.URL, ch.Subscribers); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RemoveSubscribedChannel removes a single channel from the local subscriptions DB.
func (d *DB) RemoveSubscribedChannel(channelID string) error {
	_, err := d.sql.Exec(`DELETE FROM subscribed_channels WHERE channel_id=?`, channelID)
	return err
}

// DeleteChannelVideos removes all channel_videos rows for a given channel.
func (d *DB) DeleteChannelVideos(channelID string) error {
	_, err := d.sql.Exec(`DELETE FROM channel_videos WHERE channel_id=?`, channelID)
	return err
}

// GetSubscribedChannels returns the persisted channel list including any user-set alias and tags.
func (d *DB) GetSubscribedChannels() ([]youtube.Channel, error) {
	rows, err := d.sql.Query(`
		SELECT channel_id, name, url, subscribers,
		       COALESCE(alias,''), COALESCE(tags,''), COALESCE(is_local,0)
		FROM subscribed_channels ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []youtube.Channel
	for rows.Next() {
		var ch youtube.Channel
		var tagsStr string
		var isLocal int
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.URL, &ch.Subscribers, &ch.Alias, &tagsStr, &isLocal); err != nil {
			return nil, err
		}
		if tagsStr != "" {
			ch.Tags = strings.Split(tagsStr, ",")
		}
		ch.IsLocal = isLocal == 1
		out = append(out, ch)
	}
	return out, rows.Err()
}

// AddSubscribedChannel upserts a single channel, preserving any existing alias and tags.
func (d *DB) AddSubscribedChannel(ch youtube.Channel) error {
	isLocal := 0
	if ch.IsLocal {
		isLocal = 1
	}
	_, err := d.sql.Exec(`
		INSERT INTO subscribed_channels (channel_id, name, url, subscribers, is_local)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(channel_id) DO UPDATE SET
			name=excluded.name, url=excluded.url,
			subscribers=excluded.subscribers,
			is_local=excluded.is_local,
			updated_at=CURRENT_TIMESTAMP
	`, ch.ID, ch.Name, ch.URL, ch.Subscribers, isLocal)
	return err
}

// SetChannelAlias sets or clears the display-name alias for a subscribed channel.
func (d *DB) SetChannelAlias(channelID, alias string) error {
	_, err := d.sql.Exec(`UPDATE subscribed_channels SET alias=? WHERE channel_id=?`, alias, channelID)
	return err
}

// SetChannelTags replaces the tag list for a subscribed channel.
func (d *DB) SetChannelTags(channelID string, tags []string) error {
	_, err := d.sql.Exec(`UPDATE subscribed_channels SET tags=? WHERE channel_id=?`,
		strings.Join(tags, ","), channelID)
	return err
}

func isColumnExists(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "duplicate column name") || strings.Contains(msg, "already exists")
}

// UpsertVideo inserts or updates a video record.
func (d *DB) UpsertVideo(id, title, channel, channelID string, duration int, viewCount int64, uploadDate, url string) error {
	_, err := d.sql.Exec(`
		INSERT INTO videos (id, title, channel, channel_id, duration, view_count, upload_date, url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title=excluded.title, channel=excluded.channel,
			channel_id=COALESCE(NULLIF(excluded.channel_id,''), channel_id),
			duration=excluded.duration, view_count=excluded.view_count,
			upload_date=excluded.upload_date, url=excluded.url
	`, id, title, channel, channelID, duration, viewCount, uploadDate, url)
	return err
}

// AddLocalVideo records a downloaded video.
func (d *DB) AddLocalVideo(v LocalVideo) error {
	_, err := d.sql.Exec(`
		INSERT INTO local_videos (id, file_path, download_type, downloaded_at, status)
		VALUES (?, ?, ?, ?, 'new')
		ON CONFLICT(id) DO UPDATE SET file_path=excluded.file_path, download_type=excluded.download_type
	`, v.ID, v.FilePath, v.DownloadType, v.DownloadedAt)
	return err
}

// SetVideoStatus updates playback status.
func (d *DB) SetVideoStatus(id string, status VideoStatus) error {
	now := time.Now()
	_, err := d.sql.Exec(`
		UPDATE local_videos SET status=?, last_played=? WHERE id=?
	`, string(status), now, id)
	return err
}

// DeleteLocalVideo removes a local video record.
func (d *DB) DeleteLocalVideo(id string) error {
	_, err := d.sql.Exec(`DELETE FROM local_videos WHERE id=?`, id)
	return err
}

// LocalVideos returns all downloaded videos ordered by download date.
func (d *DB) LocalVideos() ([]LocalVideo, error) {
	rows, err := d.sql.Query(`
		SELECT lv.id, v.title, v.channel, v.duration,
		       COALESCE(v.view_count, 0), COALESCE(v.upload_date, ''),
		       lv.file_path, lv.download_type, lv.downloaded_at, lv.status,
		       COALESCE(lv.last_played, ''), COALESCE(lv.last_position_ms, 0)
		FROM local_videos lv
		JOIN videos v ON v.id = lv.id
		ORDER BY lv.downloaded_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []LocalVideo
	for rows.Next() {
		var lv LocalVideo
		var lastPlayed string
		if err := rows.Scan(
			&lv.ID, &lv.Title, &lv.Channel, &lv.Duration,
			&lv.ViewCount, &lv.UploadDate,
			&lv.FilePath, &lv.DownloadType, &lv.DownloadedAt,
			&lv.Status, &lastPlayed, &lv.LastPositionMs,
		); err != nil {
			return nil, err
		}
		if lastPlayed != "" {
			lv.LastPlayed, _ = time.Parse("2006-01-02T15:04:05Z", lastPlayed)
		}
		result = append(result, lv)
	}
	return result, rows.Err()
}

// UpdateLastPosition saves the last known playback position for a local video.
func (d *DB) UpdateLastPosition(id string, ms int64) error {
	_, err := d.sql.Exec(`UPDATE local_videos SET last_position_ms=? WHERE id=?`, ms, id)
	return err
}

// HasLocalVideo returns the local video record if it exists.
func (d *DB) HasLocalVideo(id string) (LocalVideo, bool) {
	var lv LocalVideo
	err := d.sql.QueryRow(`
		SELECT lv.id, v.title, v.channel, v.duration,
		       COALESCE(v.view_count, 0), COALESCE(v.upload_date, ''),
		       lv.file_path, lv.download_type, lv.downloaded_at, lv.status,
		       COALESCE(lv.last_played, '')
		FROM local_videos lv JOIN videos v ON v.id=lv.id
		WHERE lv.id=?
	`, id).Scan(
		&lv.ID, &lv.Title, &lv.Channel, &lv.Duration,
		&lv.ViewCount, &lv.UploadDate,
		&lv.FilePath, &lv.DownloadType, &lv.DownloadedAt,
		&lv.Status, new(string),
	)
	if err != nil {
		return LocalVideo{}, false
	}
	return lv, true
}

// AddHistory records an event.
func (d *DB) AddHistory(videoID, eventType, details string) error {
	_, err := d.sql.Exec(`
		INSERT INTO history (video_id, event_type, details) VALUES (?, ?, ?)
	`, videoID, eventType, details)
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
			WHERE h.video_id != ''
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

// SaveYTPlaylists persists the YouTube playlist list.
func (d *DB) SaveYTPlaylists(playlists []youtube.YTPlaylist) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM yt_playlists`); err != nil {
		return err
	}
	for _, pl := range playlists {
		if _, err := tx.Exec(`INSERT INTO yt_playlists (id, title) VALUES (?, ?)`, pl.ID, pl.Title); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetYTPlaylists returns the cached YouTube playlist list.
func (d *DB) GetYTPlaylists() ([]youtube.YTPlaylist, error) {
	rows, err := d.sql.Query(`SELECT id, title FROM yt_playlists ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []youtube.YTPlaylist
	for rows.Next() {
		var pl youtube.YTPlaylist
		if err := rows.Scan(&pl.ID, &pl.Title); err != nil {
			return nil, err
		}
		out = append(out, pl)
	}
	return out, rows.Err()
}

// Playlists returns all custom playlists.
func (d *DB) Playlists() ([]Playlist, error) {
	rows, err := d.sql.Query(`SELECT id, name, created_at FROM playlists ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Playlist
	for rows.Next() {
		var p Playlist
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// CreatePlaylist creates a new playlist.
func (d *DB) CreatePlaylist(name string) (int64, error) {
	res, err := d.sql.Exec(`INSERT OR IGNORE INTO playlists (name) VALUES (?)`, name)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DeletePlaylist removes a playlist.
func (d *DB) DeletePlaylist(id int64) error {
	_, err := d.sql.Exec(`DELETE FROM playlists WHERE id=?`, id)
	return err
}

// AddToPlaylist adds a video to a playlist.
func (d *DB) AddToPlaylist(playlistID int64, videoID string) error {
	_, err := d.sql.Exec(`
		INSERT OR IGNORE INTO playlist_videos (playlist_id, video_id) VALUES (?, ?)
	`, playlistID, videoID)
	return err
}

// RemoveFromPlaylist removes a video from a playlist.
func (d *DB) RemoveFromPlaylist(playlistID int64, videoID string) error {
	_, err := d.sql.Exec(`
		DELETE FROM playlist_videos WHERE playlist_id=? AND video_id=?
	`, playlistID, videoID)
	return err
}

// PlaylistVideoIDs returns video IDs in a playlist (needs cross-reference with a video cache).
func (d *DB) PlaylistVideoIDs(playlistID int64) ([]string, error) {
	rows, err := d.sql.Query(`
		SELECT video_id FROM playlist_videos
		WHERE playlist_id=? ORDER BY position, added_at
	`, playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// PlaylistVideos returns full video details for all videos in a playlist.
func (d *DB) PlaylistVideos(playlistID int64) ([]youtube.Video, error) {
	rows, err := d.sql.Query(`
		SELECT v.id, v.title, COALESCE(v.channel,''), COALESCE(v.channel_id,''),
		       COALESCE(v.duration,0), COALESCE(v.view_count,0),
		       COALESCE(v.upload_date,''), COALESCE(v.url,'')
		FROM playlist_videos pv
		JOIN videos v ON v.id = pv.video_id
		WHERE pv.playlist_id = ?
		ORDER BY pv.position, pv.added_at
	`, playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []youtube.Video
	for rows.Next() {
		var v youtube.Video
		if err := rows.Scan(&v.ID, &v.Title, &v.Channel, &v.ChannelID,
			&v.Duration, &v.ViewCount, &v.UploadDate, &v.URL); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// AddWatchLater adds a video to watch later.
func (d *DB) AddWatchLater(id, title, channel, url string) error {
	_, err := d.sql.Exec(`
		INSERT OR REPLACE INTO watch_later (video_id, title, channel, url) VALUES (?, ?, ?, ?)
	`, id, title, channel, url)
	return err
}

// RemoveWatchLater removes a video from watch later.
func (d *DB) RemoveWatchLater(id string) error {
	_, err := d.sql.Exec(`DELETE FROM watch_later WHERE video_id=?`, id)
	return err
}

type WatchLaterEntry struct {
	VideoID string
	Title   string
	Channel string
	URL     string
	AddedAt time.Time
}

// WatchLater returns all watch-later entries.
func (d *DB) WatchLater() ([]WatchLaterEntry, error) {
	rows, err := d.sql.Query(`
		SELECT video_id, title, channel, url, added_at
		FROM watch_later ORDER BY added_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []WatchLaterEntry
	for rows.Next() {
		var e WatchLaterEntry
		if err := rows.Scan(&e.VideoID, &e.Title, &e.Channel, &e.URL, &e.AddedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// SaveFeedCache replaces the cached video list for a feed.
func (d *DB) SaveFeedCache(feed string, videos []youtube.Video) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM feed_cache WHERE feed=?`, feed); err != nil {
		return err
	}
	for i, v := range videos {
		if _, err := tx.Exec(`
			INSERT INTO videos (id, title, channel, channel_id, duration, view_count, upload_date, url)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				title=excluded.title, channel=excluded.channel,
				channel_id=COALESCE(NULLIF(excluded.channel_id,''), channel_id),
				duration=excluded.duration, view_count=excluded.view_count,
				upload_date=excluded.upload_date
		`, v.ID, v.Title, v.Channel, v.ChannelID, v.Duration, v.ViewCount, v.UploadDate, v.URL); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT OR REPLACE INTO feed_cache (feed, video_id, position) VALUES (?, ?, ?)`,
			feed, v.ID, i,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetFeedCache returns the cached video list for a feed ordered by position.
func (d *DB) GetFeedCache(feed string) ([]youtube.Video, error) {
	rows, err := d.sql.Query(`
		SELECT v.id, v.title, COALESCE(v.channel,''), COALESCE(v.channel_id,''),
		       COALESCE(v.duration,0), COALESCE(v.view_count,0),
		       COALESCE(v.upload_date,''), COALESCE(v.url,'')
		FROM feed_cache fc
		JOIN videos v ON v.id = fc.video_id
		WHERE fc.feed = ?
		ORDER BY fc.position
	`, feed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []youtube.Video
	for rows.Next() {
		var v youtube.Video
		if err := rows.Scan(&v.ID, &v.Title, &v.Channel, &v.ChannelID,
			&v.Duration, &v.ViewCount, &v.UploadDate, &v.URL); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// PurgeFeedCacheMissingChannelID removes entries from feed_cache whose video
// has no channel_id so the next fetch repopulates them with correct IDs.
func (d *DB) PurgeFeedCacheMissingChannelID(feed string) error {
	_, err := d.sql.Exec(`
		DELETE FROM feed_cache
		WHERE feed = ?
		  AND video_id IN (
			SELECT id FROM videos WHERE channel_id IS NULL OR channel_id = ''
		  )
	`, feed)
	return err
}

// HideRecVideo records a video as hidden from the recommended feed.
func (d *DB) HideRecVideo(videoID string) error {
	if _, err := d.sql.Exec(`INSERT OR IGNORE INTO hidden_rec_videos (video_id) VALUES (?)`, videoID); err != nil {
		return err
	}
	_, err := d.sql.Exec(`DELETE FROM video_details_cache WHERE video_id=?`, videoID)
	return err
}

// SaveVideoDetailsCache stores description, thumbnail URL and subscriber count for a video.
func (d *DB) SaveVideoDetailsCache(videoID, description, thumbnailURL string, subscribers int64) error {
	_, err := d.sql.Exec(`
		INSERT OR REPLACE INTO video_details_cache (video_id, description, thumbnail_url, subscribers)
		VALUES (?, ?, ?, ?)
	`, videoID, description, thumbnailURL, subscribers)
	return err
}

type CachedDetails struct {
	Description  string
	ThumbnailURL string
	Subscribers  int64
}

// GetVideoDetailsCache returns cached details for a video, false if not cached.
func (d *DB) GetVideoDetailsCache(videoID string) (CachedDetails, bool, error) {
	var c CachedDetails
	err := d.sql.QueryRow(`
		SELECT description, thumbnail_url, subscribers FROM video_details_cache WHERE video_id=?
	`, videoID).Scan(&c.Description, &c.ThumbnailURL, &c.Subscribers)
	if err == sql.ErrNoRows {
		return c, false, nil
	}
	return c, err == nil, err
}

// pruneRecommendedFeed removes recommended feed entries and their cached details for videos
// older than two weeks.
func (d *DB) pruneRecommendedFeed() error {
	cutoff := time.Now().AddDate(0, 0, -14).Format("20060102")
	if _, err := d.sql.Exec(`
		DELETE FROM video_details_cache WHERE video_id IN (
			SELECT fc.video_id FROM feed_cache fc
			JOIN videos v ON v.id = fc.video_id
			WHERE v.upload_date != '' AND v.upload_date < ?
		)
	`, cutoff); err != nil {
		return err
	}
	_, err := d.sql.Exec(`
		DELETE FROM feed_cache WHERE video_id IN (
			SELECT id FROM videos WHERE upload_date != '' AND upload_date < ?
		)
	`, cutoff)
	return err
}

// HiddenRecVideoIDs returns a set of video IDs hidden from recommended.
func (d *DB) HiddenRecVideoIDs() (map[string]bool, error) {
	rows, err := d.sql.Query(`SELECT video_id FROM hidden_rec_videos`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// GetAllChannelVideos returns all videos for the given channel IDs, newest first.
func (d *DB) GetAllChannelVideos(channelIDs []string) ([]youtube.Video, error) {
	if len(channelIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(channelIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(channelIDs))
	for i, id := range channelIDs {
		args[i] = id
	}
	rows, err := d.sql.Query(`
		SELECT v.id, v.title, COALESCE(v.channel,''), cv.channel_id,
		       COALESCE(v.duration,0), COALESCE(v.view_count,0),
		       COALESCE(v.upload_date,''), COALESCE(v.url,'')
		FROM channel_videos cv
		JOIN videos v ON v.id = cv.video_id
		WHERE cv.channel_id IN (`+placeholders+`)
		ORDER BY v.upload_date DESC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []youtube.Video
	for rows.Next() {
		var v youtube.Video
		if err := rows.Scan(&v.ID, &v.Title, &v.Channel, &v.ChannelID,
			&v.Duration, &v.ViewCount, &v.UploadDate, &v.URL); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetChannelLatestAll returns the most recent video per channel derived from channel_videos.
func (d *DB) GetChannelLatestAll() (map[string]youtube.Video, error) {
	rows, err := d.sql.Query(`
		WITH latest AS (
			SELECT cv.channel_id, MAX(v.upload_date) AS max_date
			FROM channel_videos cv
			JOIN videos v ON v.id = cv.video_id
			GROUP BY cv.channel_id
		)
		SELECT l.channel_id, v.id, v.title, COALESCE(v.channel,''), COALESCE(v.channel_id,''),
		       COALESCE(v.duration,0), COALESCE(v.view_count,0),
		       COALESCE(v.upload_date,''), COALESCE(v.url,'')
		FROM latest l
		JOIN channel_videos cv ON cv.channel_id = l.channel_id
		JOIN videos v ON v.id = cv.video_id AND v.upload_date = l.max_date
		GROUP BY l.channel_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]youtube.Video)
	for rows.Next() {
		var chID string
		var v youtube.Video
		if err := rows.Scan(&chID, &v.ID, &v.Title, &v.Channel, &v.ChannelID,
			&v.Duration, &v.ViewCount, &v.UploadDate, &v.URL); err != nil {
			return nil, err
		}
		out[chID] = v
	}
	return out, rows.Err()
}

// ChannelHideStats returns count of hidden videos and played videos for a channel.
func (d *DB) ChannelHideStats(channelID string) (hidden, played int, err error) {
	err = d.sql.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM hidden_rec_videos hrv
			 JOIN videos v ON v.id = hrv.video_id
			 WHERE v.channel_id = ?) AS hidden_count,
			(SELECT COUNT(*) FROM history h
			 JOIN videos v ON v.id = h.video_id
			 WHERE v.channel_id = ? AND h.event_type = 'play') AS play_count
	`, channelID, channelID).Scan(&hidden, &played)
	return hidden, played, err
}

// SaveYTPlaylistVideos replaces the cached video list for a YT playlist.
func (d *DB) SaveYTPlaylistVideos(playlistID string, videos []youtube.Video) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM yt_playlist_videos WHERE playlist_id=?`, playlistID); err != nil {
		return err
	}
	for i, v := range videos {
		if _, err := tx.Exec(`
			INSERT INTO videos (id, title, channel, channel_id, duration, view_count, upload_date, url)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				title=excluded.title, channel=excluded.channel,
				channel_id=COALESCE(NULLIF(excluded.channel_id,''), channel_id),
				duration=excluded.duration, view_count=excluded.view_count,
				upload_date=excluded.upload_date, url=excluded.url
		`, v.ID, v.Title, v.Channel, v.ChannelID, v.Duration, v.ViewCount, v.UploadDate, v.URL); err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT OR REPLACE INTO yt_playlist_videos (playlist_id, video_id, position)
			VALUES (?, ?, ?)
		`, playlistID, v.ID, i); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetYTPlaylistVideos returns cached videos for a YT playlist in position order.
func (d *DB) GetYTPlaylistVideos(playlistID string) ([]youtube.Video, error) {
	rows, err := d.sql.Query(`
		SELECT v.id, v.title, COALESCE(v.channel,''), COALESCE(v.channel_id,''),
		       COALESCE(v.duration,0), COALESCE(v.view_count,0),
		       COALESCE(v.upload_date,''), COALESCE(v.url,'')
		FROM yt_playlist_videos pv
		JOIN videos v ON v.id = pv.video_id
		WHERE pv.playlist_id = ?
		ORDER BY pv.position
	`, playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []youtube.Video
	for rows.Next() {
		var v youtube.Video
		if err := rows.Scan(&v.ID, &v.Title, &v.Channel, &v.ChannelID,
			&v.Duration, &v.ViewCount, &v.UploadDate, &v.URL); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
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

func nullInt64(v int64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}
