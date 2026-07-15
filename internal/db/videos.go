package db

import (
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

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
func (d *DB) AddLocalVideo(v domain.LocalVideo) error {
	_, err := d.sql.Exec(`
		INSERT INTO local_videos (id, file_path, download_type, downloaded_at, status)
		VALUES (?, ?, ?, ?, 'new')
		ON CONFLICT(id) DO UPDATE SET file_path=excluded.file_path, download_type=excluded.download_type
	`, v.ID, v.FilePath, v.DownloadType, v.DownloadedAt)
	return err
}

// SetVideoStatus updates playback status.
func (d *DB) SetVideoStatus(id string, status domain.VideoStatus) error {
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
func (d *DB) LocalVideos() ([]domain.LocalVideo, error) {
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
	var result []domain.LocalVideo
	for rows.Next() {
		var lv domain.LocalVideo
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

// AllVideoPositions returns all saved positions as a map of videoID → position_ms.
func (d *DB) AllVideoPositions() (map[string]int64, error) {
	rows, err := d.sql.Query(`SELECT video_id, position_ms FROM video_positions WHERE position_ms > 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]int64)
	for rows.Next() {
		var id string
		var ms int64
		if err := rows.Scan(&id, &ms); err == nil {
			m[id] = ms
		}
	}
	return m, rows.Err()
}

// SaveVideoPosition upserts the last known playback position for any video (local or streamed).
func (d *DB) SaveVideoPosition(videoID string, ms int64) error {
	_, err := d.sql.Exec(`
		INSERT INTO video_positions (video_id, position_ms, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(video_id) DO UPDATE SET position_ms=excluded.position_ms, updated_at=excluded.updated_at
	`, videoID, ms)
	return err
}

// DeleteVideoPosition removes the saved playback position for a video.
func (d *DB) DeleteVideoPosition(videoID string) error {
	_, err := d.sql.Exec(`DELETE FROM video_positions WHERE video_id = ?`, videoID)
	return err
}

// VideoPosition returns the last saved position for any video, or 0 if none.
func (d *DB) VideoPosition(videoID string) (int64, bool) {
	var ms int64
	err := d.sql.QueryRow(`SELECT position_ms FROM video_positions WHERE video_id=?`, videoID).Scan(&ms)
	if err != nil || ms == 0 {
		return 0, false
	}
	return ms, true
}

// WatchedVideoIDs returns the set of video IDs that have any play or stream history event.
func (d *DB) WatchedVideoIDs() (map[string]bool, error) {
	rows, err := d.sql.Query(`
		SELECT DISTINCT video_id FROM history
		WHERE event_type IN ('playVideo','playAudio','streamVideo','streamAudio')
		AND video_id != ''
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids[id] = true
		}
	}
	return ids, rows.Err()
}

// UpdateLastPosition saves the last known playback position for a local video.
func (d *DB) UpdateLastPosition(id string, ms int64) error {
	return d.SaveVideoPosition(id, ms)
}

// HasLocalVideo returns the local video record if it exists.
func (d *DB) HasLocalVideo(id string) (domain.LocalVideo, bool) {
	var lv domain.LocalVideo
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
		return domain.LocalVideo{}, false
	}
	return lv, true
}

// ClearDownloads removes all local_videos DB entries and returns their file paths for deletion.
func (d *DB) ClearDownloads() ([]string, error) {
	rows, err := d.sql.Query(`SELECT file_path FROM local_videos WHERE file_path != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	if _, err := d.sql.Exec(`DELETE FROM local_videos`); err != nil {
		return nil, err
	}
	return paths, nil
}
