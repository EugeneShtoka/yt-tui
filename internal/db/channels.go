package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// execByChannelID runs a single-statement UPDATE/DELETE keyed by channel_id,
// wrapping any error with label. query must end in "WHERE channel_id=?"; any SET
// values precede channelID in args. DRY backbone for the several one-line
// mutations that differ only in their SET clause.
func (d *DB) execByChannelID(ctx context.Context, label, query string, args ...any) error {
	if _, err := d.sql.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

// SaveChannelVideos upserts all videos for a channel and links them.
func (d *DB) SaveChannelVideos(ctx context.Context, channelID string, videos []domain.Video) error {
	return d.withTx(ctx, "SaveChannelVideos", func(tx *sql.Tx) error {
		for _, v := range videos {
			if err := upsertVideoTx(ctx, tx, v); err != nil {
				return fmt.Errorf("SaveChannelVideos upsert video: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT OR IGNORE INTO channel_videos (channel_id, video_id) VALUES (?, ?)
			`, channelID, v.ID); err != nil {
				return fmt.Errorf("SaveChannelVideos link: %w", err)
			}
		}
		return nil
	})
}

// TouchChannelVideosRefreshed records that a channel's videos were just fetched
// from the source, so the client can throttle redundant auto-refreshes across
// restarts. No-op (0 rows) for channels absent from subscribed_channels.
func (d *DB) TouchChannelVideosRefreshed(ctx context.Context, channelID string) error {
	return d.execByChannelID(ctx, "TouchChannelVideosRefreshed",
		`UPDATE subscribed_channels SET videos_refreshed_at=? WHERE channel_id=?`,
		time.Now().Unix(), channelID)
}

// SetChannelFetchOffset records the deep-crawl resume cursor for a channel: a
// positive offset while a back-catalog crawl is paused mid-way, or
// domain.FetchOffsetComplete once the whole catalog is pulled. Lets backfill
// resume across runs instead of restarting from the top, and tell a
// fully-crawled channel from a partially-crawled one (both distinct from a
// latest-N refresh, which advances videos_refreshed_at only). No-op for channels
// absent from subscribed_channels.
func (d *DB) SetChannelFetchOffset(ctx context.Context, channelID string, offset int64) error {
	return d.execByChannelID(ctx, "SetChannelFetchOffset",
		`UPDATE subscribed_channels SET fetched_videos=? WHERE channel_id=?`,
		offset, channelID)
}

// StampChannelActivity advances last_activity_at to now (unix seconds) for each
// given channel, keeping the maximum so an earlier signal never moves it
// backward. Channels absent from subscribed_channels are silently skipped (a
// no-op UPDATE), mirroring TouchChannelVideosRefreshed — only known/annotated
// channels carry a stale timer. Empty IDs are ignored. Feeds the
// stale-tagged-channel filter (drill-in and rec-feed signals).
func (d *DB) StampChannelActivity(ctx context.Context, channelIDs ...string) error {
	now := time.Now().Unix()
	return d.stampChannelActivityAt(ctx, channelIDs, now)
}

// stampChannelActivityAt is the testable core of StampChannelActivity with an
// explicit timestamp. Runs all updates in one transaction.
func (d *DB) stampChannelActivityAt(ctx context.Context, channelIDs []string, now int64) error {
	if len(channelIDs) == 0 {
		return nil
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("StampChannelActivity begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed
	seen := make(map[string]bool, len(channelIDs))
	for _, id := range channelIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := tx.ExecContext(ctx,
			`UPDATE subscribed_channels SET last_activity_at=MAX(last_activity_at, ?) WHERE channel_id=?`,
			now, id,
		); err != nil {
			return fmt.Errorf("StampChannelActivity update: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("StampChannelActivity commit: %w", err)
	}
	return nil
}

// GetChannelVideos returns persisted videos for a channel, newest first.
func (d *DB) GetChannelVideos(ctx context.Context, channelID string) ([]domain.Video, error) {
	result, err := queryList(ctx, d.sql, `
		SELECT v.id, v.title, COALESCE(v.channel,''), COALESCE(v.channel_id,''),
		       COALESCE(v.duration,0), COALESCE(v.view_count,0),
		       COALESCE(v.upload_date,''), COALESCE(v.url,'')
		FROM channel_videos cv
		JOIN videos v ON v.id = cv.video_id
		WHERE cv.channel_id = ?
		ORDER BY v.upload_date DESC
	`, scanVideoRow, channelID)
	if err != nil {
		return nil, fmt.Errorf("GetChannelVideos: %w", err)
	}
	return result, nil
}

// SaveSubscribedChannels reconciles the YT subscription list against the DB.
// A row can now carry user data (alias/tags) and a blocked flag independent of
// its subscription, so a YT sync that no longer sees a channel must NOT delete
// it outright — that would discard annotations and blocks. Instead:
//
//   - YT-subscribed rows absent from the fetch and carrying no user data are
//     garbage-collected (safe: nothing to preserve);
//   - YT-subscribed rows absent from the fetch but annotated transition to
//     subscription_state='none', keeping their alias/tags;
//   - local subscriptions and blocked/none rows are never touched here.
//
// The upsert never resurrects a blocked channel: an incoming YT fetch that still
// lists a blocked channel keeps it at 'none' (block invariant).
//
// An empty list is treated as "nothing to sync" and is a deliberate no-op rather
// than "delete every YT-subscribed channel": callers pass the result of a fresh
// fetch, and a fetch that comes back empty is far more likely to be a transient
// upstream failure than a genuine unsubscribe-from-everything. To actually clear
// all subscriptions, remove them individually via RemoveSubscribedChannel.
func (d *DB) SaveSubscribedChannels(ctx context.Context, channels []domain.Channel) error {
	if len(channels) == 0 {
		return nil
	}
	return d.withTx(ctx, "SaveSubscribedChannels", func(tx *sql.Tx) error {
		ids := make([]interface{}, len(channels))
		for i := range channels {
			ids[i] = channels[i].ID
		}
		notIn := `subscription_state='subscribed_yt' AND channel_id NOT IN (` + placeholders(len(channels)) + `)`
		// GC annotation-free YT subs that dropped off the fetch.
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM subscribed_channels WHERE `+notIn+` AND alias='' AND tags=''`,
			ids...,
		); err != nil {
			return fmt.Errorf("SaveSubscribedChannels gc: %w", err)
		}
		// Transition the annotated remainder to 'none', preserving alias/tags.
		if _, err := tx.ExecContext(ctx,
			`UPDATE subscribed_channels SET subscription_state='none', is_local=0, updated_at=CURRENT_TIMESTAMP
			 WHERE `+notIn+` AND (alias!='' OR tags!='')`,
			ids...,
		); err != nil {
			return fmt.Errorf("SaveSubscribedChannels transition: %w", err)
		}
		// Upsert — alias and tags are intentionally excluded from the UPDATE SET so
		// they are preserved; a blocked row stays at 'none' rather than resurfacing.
		for i := range channels {
			ch := &channels[i]
			if ch.ID == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO subscribed_channels (channel_id, name, url, subscribers, is_local, subscription_state)
				VALUES (?, ?, ?, ?, 0, ?)
				ON CONFLICT(channel_id) DO UPDATE SET
					name=excluded.name, url=excluded.url,
					subscribers=excluded.subscribers,
					is_local=0,
					subscription_state=CASE WHEN subscribed_channels.blocked=1 THEN 'none' ELSE excluded.subscription_state END,
					updated_at=CURRENT_TIMESTAMP
			`, ch.ID, ch.Name, ch.URL, ch.Subscribers, string(domain.SubYT)); err != nil {
				return fmt.Errorf("SaveSubscribedChannels upsert: %w", err)
			}
		}
		return nil
	})
}

// SetChannelState transitions a channel's subscription_state (and keeps the
// legacy is_local flag in sync). It enforces the block invariant: moving a
// blocked channel to a subscribed state is rejected with domain.ErrChannelBlocked
// — the caller must UnblockChannel first. Moving to 'none' is always allowed.
// Runs in a transaction so the blocked check and the write can't race (the DB
// uses a single connection, but the transaction keeps the rule self-contained).
func (d *DB) SetChannelState(ctx context.Context, channelID string, state domain.SubscriptionState) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("SetChannelState begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed
	if state != domain.SubNone {
		var blocked int
		err := tx.QueryRowContext(ctx, `SELECT COALESCE(blocked,0) FROM subscribed_channels WHERE channel_id=?`, channelID).Scan(&blocked)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("SetChannelState check: %w", err)
		}
		if blocked == 1 {
			return domain.ErrChannelBlocked
		}
	}
	isLocal := 0
	if state == domain.SubLocal {
		isLocal = 1
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO subscribed_channels (channel_id, subscription_state, is_local)
		VALUES (?, ?, ?)
		ON CONFLICT(channel_id) DO UPDATE SET
			subscription_state=excluded.subscription_state,
			is_local=excluded.is_local,
			updated_at=CURRENT_TIMESTAMP
	`, channelID, string(state), isLocal); err != nil {
		return fmt.Errorf("SetChannelState update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("SetChannelState commit: %w", err)
	}
	return nil
}

// RemoveSubscribedChannel removes a single channel from the local subscriptions DB.
func (d *DB) RemoveSubscribedChannel(ctx context.Context, channelID string) error {
	return d.execByChannelID(ctx, "RemoveSubscribedChannel",
		`DELETE FROM subscribed_channels WHERE channel_id=?`, channelID)
}

// DeleteChannelVideos removes all channel_videos rows for a given channel.
func (d *DB) DeleteChannelVideos(ctx context.Context, channelID string) error {
	return d.execByChannelID(ctx, "DeleteChannelVideos",
		`DELETE FROM channel_videos WHERE channel_id=?`, channelID)
}

// channelColumns is the shared SELECT projection for a subscribed_channels row,
// consumed by scanChannel. Kept in one place so every channel query
// (subscriptions, all channels, blocked) reads the same columns in the same order.
const channelColumns = `channel_id, name, url, subscribers,
	COALESCE(alias,''), COALESCE(tags,''), COALESCE(is_local,0),
	COALESCE(subscription_state,'none'), COALESCE(blocked,0),
	COALESCE(videos_refreshed_at,0), COALESCE(fetched_videos,0), COALESCE(last_activity_at,0)`

// scanChannel reads one subscribed_channels row in channelColumns order into a
// domain.Channel, deriving the Tags slice, IsLocal, State, and Blocked fields.
func scanChannel(rows *sql.Rows) (domain.Channel, error) {
	var ch domain.Channel
	var tagsStr, state string
	var isLocal, blocked int
	if err := rows.Scan(&ch.ID, &ch.Name, &ch.URL, &ch.Subscribers, &ch.Alias, &tagsStr, &isLocal, &state, &blocked, &ch.VideosRefreshedAt, &ch.FetchedVideos, &ch.LastActivityAt); err != nil {
		return domain.Channel{}, fmt.Errorf("scanChannel: %w", err)
	}
	if tagsStr != "" {
		ch.Tags = strings.Split(tagsStr, ",")
	}
	ch.IsLocal = isLocal == 1
	ch.State = domain.SubscriptionState(state)
	ch.Blocked = blocked == 1
	return ch, nil
}

// queryChannels runs a channelColumns SELECT with the given WHERE clause (empty
// for all rows) and scans every row, always ordered by name. DRY backbone for
// GetSubscribedChannels / AllChannels / BlockedChannels.
func (d *DB) queryChannels(ctx context.Context, where string) ([]domain.Channel, error) {
	q := `SELECT ` + channelColumns + ` FROM subscribed_channels`
	if where != "" {
		q += ` WHERE ` + where
	}
	q += ` ORDER BY name`
	out, err := queryList(ctx, d.sql, q, scanChannel)
	if err != nil {
		return nil, fmt.Errorf("queryChannels: %w", err)
	}
	return out, nil
}

// GetSubscribedChannels returns the persisted subscription list including any
// user-set alias and tags. Rows at subscription_state='none' (e.g. blocked
// channels, or channels annotated but not subscribed) are excluded so callers
// keep seeing subscriptions only — AllChannels/BlockedChannels expose the rest.
func (d *DB) GetSubscribedChannels(ctx context.Context) ([]domain.Channel, error) {
	return d.queryChannels(ctx, `COALESCE(subscription_state,'none') != 'none'`)
}

// AllChannels returns every known channel row regardless of subscription state:
// subscribed (yt/local), annotated-but-unsubscribed (state='none'), and blocked.
// The universe backing the Channels/Tags panels' "all" and "recommended" modes.
func (d *DB) AllChannels(ctx context.Context) ([]domain.Channel, error) {
	return d.queryChannels(ctx, "")
}

// BlockedChannels returns the channels currently on the blocklist (blocked=1).
func (d *DB) BlockedChannels(ctx context.Context) ([]domain.Channel, error) {
	return d.queryChannels(ctx, `blocked=1`)
}

// AddSubscribedChannel upserts a single channel, preserving any existing alias
// and tags. It refuses to subscribe a blocked channel (block invariant),
// returning domain.ErrChannelBlocked so the caller can prompt an unblock first.
func (d *DB) AddSubscribedChannel(ctx context.Context, ch domain.Channel) error {
	isLocal := 0
	if ch.IsLocal {
		isLocal = 1
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("AddSubscribedChannel begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed
	var blocked int
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(blocked,0) FROM subscribed_channels WHERE channel_id=?`, ch.ID).Scan(&blocked)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("AddSubscribedChannel check: %w", err)
	}
	if blocked == 1 {
		return domain.ErrChannelBlocked
	}
	// Adding/annotating a channel counts as activity, so a freshly-tagged
	// recommended-feed channel (materialized here) starts fresh rather than
	// immediately reading as stale. Kept monotonic (MAX) on conflict.
	now := time.Now().Unix()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO subscribed_channels (channel_id, name, url, subscribers, is_local, subscription_state, last_activity_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(channel_id) DO UPDATE SET
			name=excluded.name, url=excluded.url,
			subscribers=excluded.subscribers,
			is_local=excluded.is_local,
			subscription_state=excluded.subscription_state,
			last_activity_at=MAX(subscribed_channels.last_activity_at, excluded.last_activity_at),
			updated_at=CURRENT_TIMESTAMP
	`, ch.ID, ch.Name, ch.URL, ch.Subscribers, isLocal, string(ch.SubState()), now); err != nil {
		return fmt.Errorf("AddSubscribedChannel: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("AddSubscribedChannel commit: %w", err)
	}
	return nil
}

// SetChannelAlias sets or clears the display-name alias for a subscribed channel.
func (d *DB) SetChannelAlias(ctx context.Context, channelID, alias string) error {
	return d.execByChannelID(ctx, "SetChannelAlias",
		`UPDATE subscribed_channels SET alias=? WHERE channel_id=?`, alias, channelID)
}

// SetChannelTags replaces the tag list for a subscribed channel.
func (d *DB) SetChannelTags(ctx context.Context, channelID string, tags []string) error {
	return d.execByChannelID(ctx, "SetChannelTags",
		`UPDATE subscribed_channels SET tags=? WHERE channel_id=?`,
		strings.Join(tags, ","), channelID)
}

// GetAllChannelVideos returns all videos for the given channel IDs, newest first.
func (d *DB) GetAllChannelVideos(ctx context.Context, channelIDs []string) ([]domain.Video, error) {
	if len(channelIDs) == 0 {
		return nil, nil
	}
	args := make([]any, len(channelIDs))
	for i, id := range channelIDs {
		args[i] = id
	}
	out, err := queryList(ctx, d.sql, `
		SELECT v.id, v.title, COALESCE(v.channel,''), cv.channel_id,
		       COALESCE(v.duration,0), COALESCE(v.view_count,0),
		       COALESCE(v.upload_date,''), COALESCE(v.url,'')
		FROM channel_videos cv
		JOIN videos v ON v.id = cv.video_id
		WHERE cv.channel_id IN (`+placeholders(len(channelIDs))+`)
		ORDER BY v.upload_date DESC
	`, scanVideoRow, args...)
	if err != nil {
		return nil, fmt.Errorf("GetAllChannelVideos: %w", err)
	}
	return out, nil
}

// GetChannelLatestAll returns the most recent video per channel derived from channel_videos.
// Same-day uploads are broken deterministically by video id (descending) — a bare
// MAX()+GROUP BY self-join lets SQLite pick an arbitrary row on ties, which made
// channel ordering flicker between refreshes.
func (d *DB) GetChannelLatestAll(ctx context.Context) (map[string]domain.Video, error) {
	out, err := queryMap(ctx, d.sql, `
		WITH ranked AS (
			SELECT cv.channel_id AS ch_id, v.id, v.title, v.channel, v.channel_id,
			       v.duration, v.view_count, v.upload_date, v.url,
			       ROW_NUMBER() OVER (
			           PARTITION BY cv.channel_id ORDER BY v.upload_date DESC, v.id DESC
			       ) AS rn
			FROM channel_videos cv
			JOIN videos v ON v.id = cv.video_id
		)
		SELECT ch_id, id, title, COALESCE(channel,''), COALESCE(channel_id,''),
		       COALESCE(duration,0), COALESCE(view_count,0),
		       COALESCE(upload_date,''), COALESCE(url,'')
		FROM ranked WHERE rn = 1
	`, func(rows *sql.Rows) (string, domain.Video, error) {
		var chID string
		v, err := scanVideo(rows, &chID)
		return chID, v, err
	})
	if err != nil {
		return nil, fmt.Errorf("GetChannelLatestAll: %w", err)
	}
	return out, nil
}

// ChannelHideStats returns count of hidden videos and played videos for a channel.
func (d *DB) ChannelHideStats(ctx context.Context, channelID string) (hidden, played int, err error) {
	if err = d.sql.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM hidden_rec_videos hrv
			 JOIN videos v ON v.id = hrv.video_id
			 WHERE v.channel_id = ?) AS hidden_count,
			(SELECT COUNT(*) FROM history h
			 JOIN videos v ON v.id = h.video_id
			 WHERE v.channel_id = ? AND h.event_type IN ('playVideo','playAudio','streamVideo','streamAudio')) AS play_count
	`, channelID, channelID).Scan(&hidden, &played); err != nil {
		return 0, 0, fmt.Errorf("ChannelHideStats: %w", err)
	}
	return hidden, played, nil
}
