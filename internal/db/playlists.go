package db

import (
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

// Playlist is a user-created local playlist.
type Playlist struct {
	ID        int64
	Name      string
	CreatedAt time.Time
}

// WatchLaterEntry is a video queued for later viewing.
type WatchLaterEntry struct {
	VideoID string
	Title   string
	Channel string
	URL     string
	AddedAt time.Time
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
