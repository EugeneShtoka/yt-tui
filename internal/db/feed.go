package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// SaveFeedCache replaces the cached video list for a feed.
func (d *DB) SaveFeedCache(feed string, videos []domain.Video) error {
	ctx := context.Background()
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM feed_cache WHERE feed=?`, feed); err != nil {
		return err
	}
	for i, v := range videos {
		if _, err := tx.ExecContext(ctx, `
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
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO feed_cache (feed, video_id, position) VALUES (?, ?, ?)`,
			feed, v.ID, i,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetFeedCache returns the cached video list for a feed ordered by position.
func (d *DB) GetFeedCache(feed string) ([]domain.Video, error) {
	ctx := context.Background()
	rows, err := d.sql.QueryContext(ctx, `
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
	var result []domain.Video
	for rows.Next() {
		var v domain.Video
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
	ctx := context.Background()
	_, err := d.sql.ExecContext(ctx, `
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
	ctx := context.Background()
	if _, err := d.sql.ExecContext(ctx, `INSERT OR IGNORE INTO hidden_rec_videos (video_id) VALUES (?)`, videoID); err != nil {
		return err
	}
	_, err := d.sql.ExecContext(ctx, `DELETE FROM video_details_cache WHERE video_id=?`, videoID)
	return err
}

// HiddenRecVideoIDs returns a set of video IDs hidden from recommended.
func (d *DB) HiddenRecVideoIDs() (map[string]bool, error) {
	ctx := context.Background()
	rows, err := d.sql.QueryContext(ctx, `SELECT video_id FROM hidden_rec_videos`)
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

// SaveVideoDetailsCache stores description, thumbnail URL and subscriber count for a video.
func (d *DB) SaveVideoDetailsCache(videoID, description, thumbnailURL string, subscribers int64) error {
	ctx := context.Background()
	_, err := d.sql.ExecContext(ctx, `
		INSERT OR REPLACE INTO video_details_cache (video_id, description, thumbnail_url, subscribers)
		VALUES (?, ?, ?, ?)
	`, videoID, description, thumbnailURL, subscribers)
	return err
}

// GetVideoDetailsCache returns cached details for a video, false if not cached.
func (d *DB) GetVideoDetailsCache(videoID string) (domain.CachedDetails, bool, error) {
	var c domain.CachedDetails
	var linksJSON, chaptersJSON, sbJSON *string
	ctx := context.Background()
	err := d.sql.QueryRowContext(ctx, `
		SELECT description, thumbnail_url, subscribers, links, chapters, sb_segments
		FROM video_details_cache WHERE video_id=?
	`, videoID).Scan(&c.Description, &c.ThumbnailURL, &c.Subscribers, &linksJSON, &chaptersJSON, &sbJSON)
	if err == sql.ErrNoRows {
		return c, false, nil
	}
	if err != nil {
		return c, false, err
	}
	if linksJSON != nil {
		var links []domain.Link
		if json.Unmarshal([]byte(*linksJSON), &links) == nil {
			c.Links = &links
		}
	}
	if chaptersJSON != nil {
		var chapters []domain.Chapter
		if json.Unmarshal([]byte(*chaptersJSON), &chapters) == nil {
			c.Chapters = &chapters
		}
	}
	if sbJSON != nil {
		var segs []domain.SBSegment
		if json.Unmarshal([]byte(*sbJSON), &segs) == nil {
			c.SBSegments = &segs
		}
	}
	return c, true, nil
}

// SaveVideoChapters stores the pre-processed, SponsorBlock-adjusted chapter list for a video.
func (d *DB) SaveVideoChapters(videoID string, chapters []domain.Chapter) error {
	data, err := json.Marshal(chapters)
	if err != nil {
		return err
	}
	ctx := context.Background()
	_, err = d.sql.ExecContext(ctx, `UPDATE video_details_cache SET chapters=? WHERE video_id=?`, string(data), videoID)
	return err
}

// SaveVideoSBSegments stores the raw SponsorBlock cut ranges for a video.
func (d *DB) SaveVideoSBSegments(videoID string, segs []domain.SBSegment) error {
	data, err := json.Marshal(segs)
	if err != nil {
		return err
	}
	ctx := context.Background()
	_, err = d.sql.ExecContext(ctx, `UPDATE video_details_cache SET sb_segments=? WHERE video_id=?`, string(data), videoID)
	return err
}

// SaveVideoLinks stores the parsed link list for a video. An empty slice means
// the description was parsed and contained no links (distinct from NULL = not parsed).
func (d *DB) SaveVideoLinks(videoID string, links []domain.Link) error {
	data, err := json.Marshal(links)
	if err != nil {
		return err
	}
	ctx := context.Background()
	_, err = d.sql.ExecContext(ctx, `UPDATE video_details_cache SET links=? WHERE video_id=?`, string(data), videoID)
	return err
}

// pruneRecommendedFeed removes recommended feed entries and their cached details for videos
// older than maxDays days.
func (d *DB) pruneRecommendedFeed(maxDays int) error {
	ctx := context.Background()
	cutoff := time.Now().AddDate(0, 0, -maxDays).Format("20060102")
	if _, err := d.sql.ExecContext(ctx, `
		DELETE FROM video_details_cache WHERE video_id IN (
			SELECT fc.video_id FROM feed_cache fc
			JOIN videos v ON v.id = fc.video_id
			WHERE v.upload_date != '' AND v.upload_date < ?
			AND fc.video_id NOT IN (SELECT video_id FROM channel_videos)
			AND fc.video_id NOT IN (SELECT video_id FROM playlist_videos)
			AND fc.video_id NOT IN (SELECT video_id FROM yt_playlist_videos)
			AND fc.video_id NOT IN (SELECT id FROM local_videos)
		)
	`, cutoff); err != nil {
		return err
	}
	_, err := d.sql.ExecContext(ctx, `
		DELETE FROM feed_cache WHERE video_id IN (
			SELECT id FROM videos WHERE upload_date != '' AND upload_date < ?
		)
	`, cutoff)
	return err
}

// ClearRecommended removes all recommended feed entries.
func (d *DB) ClearRecommended() error {
	ctx := context.Background()
	_, err := d.sql.ExecContext(ctx, `DELETE FROM feed_cache WHERE feed='recommended'`)
	return err
}

// ClearVideoDetailsCache removes all cached video detail entries.
func (d *DB) ClearVideoDetailsCache() error {
	ctx := context.Background()
	_, err := d.sql.ExecContext(ctx, `DELETE FROM video_details_cache`)
	return err
}
