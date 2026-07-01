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
	Duration   int
	ViewCount  int64
	UploadDate string
	EventType  string
	Details    string
	Timestamp  time.Time
}

type DB struct {
	sql *sql.DB
}

func New(dataDir string) (*DB, error) {
	path := filepath.Join(dataDir, "yt-tui.db")
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	d := &DB{sql: sqlDB}
	if err := d.migrate(); err != nil {
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

		CREATE TABLE IF NOT EXISTS hidden_rec_videos (
			video_id TEXT PRIMARY KEY,
			hidden_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS channel_removals (
			channel_id   TEXT NOT NULL,
			channel_name TEXT NOT NULL,
			remove_count INTEGER DEFAULT 1,
			last_removed DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (channel_id)
		);
	`)
	if err != nil {
		return err
	}
	// Column added after initial schema; safe to ignore "duplicate column" error.
	_, err = d.sql.Exec(`ALTER TABLE local_videos ADD COLUMN last_position_ms INTEGER NOT NULL DEFAULT 0`)
	if err != nil && !isColumnExists(err) {
		return err
	}
	return nil
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

// History returns recent history entries with video titles.
func (d *DB) History(limit int) ([]HistoryEntry, error) {
	rows, err := d.sql.Query(`
		SELECT h.id, h.video_id, COALESCE(v.title, h.video_id),
		       COALESCE(v.channel, ''), COALESCE(v.duration, 0),
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
			&e.Channel, &e.Duration, &e.ViewCount, &e.UploadDate,
			&e.EventType, &e.Details, &e.Timestamp,
		); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
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

// HideRecVideo records a video as hidden from the recommended feed.
func (d *DB) HideRecVideo(videoID string) error {
	_, err := d.sql.Exec(
		`INSERT OR IGNORE INTO hidden_rec_videos (video_id) VALUES (?)`, videoID)
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

// HideRecChannel upserts a channel removal record and increments its count.
func (d *DB) HideRecChannel(channelID, channelName string) error {
	_, err := d.sql.Exec(`
		INSERT INTO channel_removals (channel_id, channel_name, remove_count, last_removed)
		VALUES (?, ?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT(channel_id) DO UPDATE SET
			remove_count = remove_count + 1,
			last_removed = CURRENT_TIMESTAMP
	`, channelID, channelName)
	return err
}

// ChannelRemovalCount returns how many times a channel has been hidden from recommended.
func (d *DB) ChannelRemovalCount(channelID string) (int, error) {
	var count int
	err := d.sql.QueryRow(
		`SELECT remove_count FROM channel_removals WHERE channel_id=?`, channelID,
	).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return count, err
}

// ChannelViewCount returns play-event count for videos from a channel.
func (d *DB) ChannelViewCount(channelID string) (int, error) {
	var count int
	err := d.sql.QueryRow(`
		SELECT COUNT(*) FROM history h
		JOIN videos v ON v.id = h.video_id
		WHERE v.channel_id = ? AND h.event_type = 'play'
	`, channelID).Scan(&count)
	return count, err
}
