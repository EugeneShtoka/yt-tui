package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// newestSubscribedCTE ranks every subscribed-channel video newest-first within
// its channel. Shared by the eligibility helpers so the "newest N per channel"
// rule is defined in exactly one place. Ordering ties break by id so the set is
// deterministic even though enriched/approximate upload_dates collide heavily.
const newestSubscribedCTE = `
	SELECT v.id AS id, ROW_NUMBER() OVER (
		PARTITION BY cv.channel_id ORDER BY v.upload_date DESC, v.id
	) AS rn
	FROM channel_videos cv
	JOIN subscribed_channels sc
		ON sc.channel_id = cv.channel_id AND sc.subscription_state != 'none'
	JOIN videos v ON v.id = cv.video_id
`

// ThumbnailEligible reports whether a video qualifies for a locally-cached
// thumbnail: it is in the recommended feed, or among the newest perChannel
// videos of a subscribed channel. This is the predicate the lazy path uses to
// decide whether to persist a thumbnail fetched on first view.
func (d *DB) ThumbnailEligible(ctx context.Context, videoID string, perChannel int) (bool, error) {
	if perChannel < 0 {
		perChannel = 0
	}
	var eligible bool
	err := d.sql.QueryRowContext(ctx, `
		SELECT
			EXISTS(SELECT 1 FROM feed_cache WHERE feed='recommended' AND video_id=?)
			OR EXISTS(
				SELECT 1 FROM (`+newestSubscribedCTE+`
					WHERE cv.channel_id IN (SELECT channel_id FROM channel_videos WHERE video_id=?)
				) WHERE id=? AND rn<=?
			)
	`, videoID, videoID, videoID, perChannel).Scan(&eligible)
	if err != nil {
		return false, fmt.Errorf("ThumbnailEligible: %w", err)
	}
	return eligible, nil
}

// ThumbnailEligibleIDs returns the full set of video IDs whose thumbnails should
// be cached: every recommended-feed video, unioned with the newest perChannel
// videos of each subscribed channel. The enricher uses it both to drive the
// thumbnail sub-pass and, via Retain, to evict everything else.
func (d *DB) ThumbnailEligibleIDs(ctx context.Context, perChannel int) (map[string]bool, error) {
	if perChannel < 0 {
		perChannel = 0
	}
	ids, err := queryMap(ctx, d.sql, `
		SELECT video_id FROM feed_cache WHERE feed='recommended'
		UNION
		SELECT id FROM (`+newestSubscribedCTE+`) WHERE rn<=?
	`, func(rows *sql.Rows) (string, bool, error) {
		var id string
		err := rows.Scan(&id)
		return id, true, err
	}, perChannel)
	if err != nil {
		return nil, fmt.Errorf("ThumbnailEligibleIDs: %w", err)
	}
	return ids, nil
}

// RecommendedVideosWithoutDetails returns recommended-feed videos that have no
// cached full details yet — the high-priority enrichment batch (small, capped,
// user-visible, and it makes the recommended age filter act on real dates).
func (d *DB) RecommendedVideosWithoutDetails(ctx context.Context) ([]domain.VideoRef, error) {
	return d.videoRefsWithoutDetails(ctx, `
		SELECT fc.video_id, COALESCE(v.url,'')
		FROM feed_cache fc
		JOIN videos v ON v.id = fc.video_id
		LEFT JOIN video_details_cache dc ON dc.video_id = fc.video_id
		WHERE fc.feed='recommended' AND dc.video_id IS NULL
	`, "RecommendedVideosWithoutDetails")
}

// SubscribedVideosWithoutDetails returns subscribed-channel videos lacking cached
// details, newest-first. limit <= 0 means no limit (the full ~30h grind).
func (d *DB) SubscribedVideosWithoutDetails(ctx context.Context, limit int) ([]domain.VideoRef, error) {
	q := `
		SELECT DISTINCT v.id, COALESCE(v.url,'')
		FROM channel_videos cv
		JOIN subscribed_channels sc
			ON sc.channel_id = cv.channel_id AND sc.subscription_state != 'none'
		JOIN videos v ON v.id = cv.video_id
		LEFT JOIN video_details_cache dc ON dc.video_id = v.id
		WHERE dc.video_id IS NULL
		ORDER BY v.upload_date DESC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	return d.videoRefsWithoutDetails(ctx, q, "SubscribedVideosWithoutDetails")
}

// videoRefsWithoutDetails runs a query projecting (id, url) and collects VideoRefs.
func (d *DB) videoRefsWithoutDetails(ctx context.Context, query, label string) ([]domain.VideoRef, error) {
	refs, err := queryList(ctx, d.sql, query, func(rows *sql.Rows) (domain.VideoRef, error) {
		var r domain.VideoRef
		err := rows.Scan(&r.ID, &r.URL)
		return r, err
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	return refs, nil
}
