package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// SaveFeedCache replaces the cached video list for a feed.
func (d *DB) SaveFeedCache(ctx context.Context, feed string, videos []domain.Video) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("SaveFeedCache begin: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM feed_cache WHERE feed=?`, feed); err != nil {
		return fmt.Errorf("SaveFeedCache delete: %w", err)
	}
	// Distinct channel IDs seen in this feed — stamped as active below so a
	// channel currently in the recommended feed never reads as stale.
	seenChannels := make(map[string]bool)
	for i, v := range videos {
		if err := upsertVideoTx(ctx, tx, v); err != nil {
			return fmt.Errorf("SaveFeedCache upsert video: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO feed_cache (feed, video_id, position) VALUES (?, ?, ?)`,
			feed, v.ID, i,
		); err != nil {
			return fmt.Errorf("SaveFeedCache insert: %w", err)
		}
		if v.ChannelID != "" {
			seenChannels[v.ChannelID] = true
		}
	}
	// Stamp channel activity for every channel present in the feed (no-op for
	// channels without a subscribed_channels row — untagged rec channels aren't
	// tracked). Monotonic MAX keeps an earlier stamp from moving backward.
	now := time.Now().Unix()
	for chID := range seenChannels {
		if _, err := tx.ExecContext(ctx,
			`UPDATE subscribed_channels SET last_activity_at=MAX(last_activity_at, ?) WHERE channel_id=?`,
			now, chID,
		); err != nil {
			return fmt.Errorf("SaveFeedCache stamp activity: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("SaveFeedCache commit: %w", err)
	}
	return nil
}

// GetFeedCache returns the cached video list for a feed ordered by position.
func (d *DB) GetFeedCache(ctx context.Context, feed string) ([]domain.Video, error) {
	result, err := queryList(ctx, d.sql, `
		SELECT v.id, v.title, COALESCE(v.channel,''), COALESCE(v.channel_id,''),
		       COALESCE(v.duration,0), COALESCE(v.view_count,0),
		       COALESCE(v.upload_date,''), COALESCE(v.url,'')
		FROM feed_cache fc
		JOIN videos v ON v.id = fc.video_id
		WHERE fc.feed = ?
		ORDER BY fc.position
	`, scanVideoRow, feed)
	if err != nil {
		return nil, fmt.Errorf("GetFeedCache: %w", err)
	}
	return result, nil
}

// PurgeFeedCacheMissingChannelID removes entries from feed_cache whose video
// has no channel_id so the next fetch repopulates them with correct IDs.
func (d *DB) PurgeFeedCacheMissingChannelID(ctx context.Context, feed string) error {
	if _, err := d.sql.ExecContext(ctx, `
		DELETE FROM feed_cache
		WHERE feed = ?
		  AND video_id IN (
			SELECT id FROM videos WHERE channel_id IS NULL OR channel_id = ''
		  )
	`, feed); err != nil {
		return fmt.Errorf("PurgeFeedCacheMissingChannelID: %w", err)
	}
	return nil
}

// HideRecVideo records a video as hidden from the recommended feed.
func (d *DB) HideRecVideo(ctx context.Context, videoID string) error {
	if _, err := d.sql.ExecContext(ctx, `INSERT OR IGNORE INTO hidden_rec_videos (video_id) VALUES (?)`, videoID); err != nil {
		return fmt.Errorf("HideRecVideo insert: %w", err)
	}
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM video_details_cache WHERE video_id=?`, videoID); err != nil {
		return fmt.Errorf("HideRecVideo delete: %w", err)
	}
	return nil
}

// HiddenRecVideoIDs returns a set of video IDs hidden from recommended.
func (d *DB) HiddenRecVideoIDs(ctx context.Context) (map[string]bool, error) {
	out, err := queryMap(ctx, d.sql, `SELECT video_id FROM hidden_rec_videos`,
		func(rows *sql.Rows) (string, bool, error) {
			var id string
			err := rows.Scan(&id)
			return id, true, err
		})
	if err != nil {
		return nil, fmt.Errorf("HiddenRecVideoIDs: %w", err)
	}
	return out, nil
}

// SaveVideoDetailsCache stores description, thumbnail URL and subscriber count for a video.
func (d *DB) SaveVideoDetailsCache(ctx context.Context, videoID, description, thumbnailURL string, subscribers int64) error {
	if _, err := d.sql.ExecContext(ctx, `
		INSERT OR REPLACE INTO video_details_cache (video_id, description, thumbnail_url, subscribers)
		VALUES (?, ?, ?, ?)
	`, videoID, description, thumbnailURL, subscribers); err != nil {
		return fmt.Errorf("SaveVideoDetailsCache: %w", err)
	}
	return nil
}

// GetVideoDetailsCache returns cached details for a video, false if not cached.
func (d *DB) GetVideoDetailsCache(ctx context.Context, videoID string) (domain.CachedDetails, bool, error) {
	var c domain.CachedDetails
	var linksJSON, chaptersJSON, sbJSON *string
	err := d.sql.QueryRowContext(ctx, `
		SELECT description, thumbnail_url, subscribers, links, chapters, sb_segments
		FROM video_details_cache WHERE video_id=?
	`, videoID).Scan(&c.Description, &c.ThumbnailURL, &c.Subscribers, &linksJSON, &chaptersJSON, &sbJSON)
	if err == sql.ErrNoRows {
		return c, false, nil
	}
	if err != nil {
		return c, false, fmt.Errorf("GetVideoDetailsCache: %w", err)
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
// Upserts rather than updates: if the details-cache row was evicted (e.g. by
// HideRecVideo or a schema-fingerprint clear) between the details fetch and this
// call, a plain UPDATE would silently affect zero rows and drop the data.
func (d *DB) SaveVideoChapters(ctx context.Context, videoID string, chapters []domain.Chapter) error {
	data, err := json.Marshal(chapters)
	if err != nil {
		return fmt.Errorf("SaveVideoChapters marshal: %w", err)
	}
	if _, err = d.sql.ExecContext(ctx, `
		INSERT INTO video_details_cache (video_id, chapters) VALUES (?, ?)
		ON CONFLICT(video_id) DO UPDATE SET chapters=excluded.chapters
	`, videoID, string(data)); err != nil {
		return fmt.Errorf("SaveVideoChapters: %w", err)
	}
	return nil
}

// SaveVideoSBSegments stores the raw SponsorBlock cut ranges for a video. See
// SaveVideoChapters for why this upserts instead of updating.
func (d *DB) SaveVideoSBSegments(ctx context.Context, videoID string, segs []domain.SBSegment) error {
	data, err := json.Marshal(segs)
	if err != nil {
		return fmt.Errorf("SaveVideoSBSegments marshal: %w", err)
	}
	if _, err = d.sql.ExecContext(ctx, `
		INSERT INTO video_details_cache (video_id, sb_segments) VALUES (?, ?)
		ON CONFLICT(video_id) DO UPDATE SET sb_segments=excluded.sb_segments
	`, videoID, string(data)); err != nil {
		return fmt.Errorf("SaveVideoSBSegments: %w", err)
	}
	return nil
}

// SaveVideoLinks stores the parsed link list for a video. An empty slice means
// the description was parsed and contained no links (distinct from NULL = not
// parsed). See SaveVideoChapters for why this upserts instead of updating.
func (d *DB) SaveVideoLinks(ctx context.Context, videoID string, links []domain.Link) error {
	data, err := json.Marshal(links)
	if err != nil {
		return fmt.Errorf("SaveVideoLinks marshal: %w", err)
	}
	if _, err = d.sql.ExecContext(ctx, `
		INSERT INTO video_details_cache (video_id, links) VALUES (?, ?)
		ON CONFLICT(video_id) DO UPDATE SET links=excluded.links
	`, videoID, string(data)); err != nil {
		return fmt.Errorf("SaveVideoLinks: %w", err)
	}
	return nil
}

// pruneRecommendedFeed removes recommended feed entries and their cached details for videos
// older than maxDays days, then sweeps any videos rows that are now unreferenced
// anywhere else. All three steps run in one transaction.
func (d *DB) pruneRecommendedFeed(maxDays int) error {
	ctx := context.Background()
	cutoff := time.Now().AddDate(0, 0, -maxDays).Format("20060102")
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("pruneRecommendedFeed begin: %w", err)
	}
	defer tx.Rollback()
	// Scoped to feed='recommended': without this filter, a video that also
	// happens to be cached under another feed had its details wiped too.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM video_details_cache WHERE video_id IN (
			SELECT fc.video_id FROM feed_cache fc
			JOIN videos v ON v.id = fc.video_id
			WHERE fc.feed = 'recommended'
			AND v.upload_date != '' AND v.upload_date < ?
			AND fc.video_id NOT IN (SELECT video_id FROM channel_videos)
			AND fc.video_id NOT IN (SELECT video_id FROM playlist_videos)
			AND fc.video_id NOT IN (SELECT video_id FROM yt_playlist_videos)
			AND fc.video_id NOT IN (SELECT id FROM local_videos)
		)
	`, cutoff); err != nil {
		return fmt.Errorf("pruneRecommendedFeed delete details: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM feed_cache WHERE feed='recommended' AND video_id IN (
			SELECT id FROM videos WHERE upload_date != '' AND upload_date < ?
		)
	`, cutoff); err != nil {
		return fmt.Errorf("pruneRecommendedFeed delete cache: %w", err)
	}
	// Orphan sweep: the videos table itself was never pruned, so aged-out rows
	// with no remaining reference anywhere accumulated forever. Safe now that
	// the feed_cache rows above (this round's only referrer) are gone.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM videos WHERE upload_date != '' AND upload_date < ?
		AND id NOT IN (SELECT video_id FROM feed_cache)
		AND id NOT IN (SELECT video_id FROM channel_videos)
		AND id NOT IN (SELECT video_id FROM playlist_videos)
		AND id NOT IN (SELECT video_id FROM yt_playlist_videos)
		AND id NOT IN (SELECT id FROM local_videos)
	`, cutoff); err != nil {
		return fmt.Errorf("pruneRecommendedFeed orphan sweep: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("pruneRecommendedFeed commit: %w", err)
	}
	return nil
}

// ClearRecommended removes all recommended feed entries.
func (d *DB) ClearRecommended(ctx context.Context) error {
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM feed_cache WHERE feed='recommended'`); err != nil {
		return fmt.Errorf("ClearRecommended: %w", err)
	}
	return nil
}

// ClearVideoDetailsCache removes all cached video detail entries.
func (d *DB) ClearVideoDetailsCache(ctx context.Context) error {
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM video_details_cache`); err != nil {
		return fmt.Errorf("ClearVideoDetailsCache: %w", err)
	}
	return nil
}
