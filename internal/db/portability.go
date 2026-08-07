package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// sqliteTimeLayout is the text format SQLite's CURRENT_TIMESTAMP writes (UTC
// 'YYYY-MM-DD HH:MM:SS'). Imported history rows are stamped in the same format
// so they sort and parse identically to organically-recorded events.
const sqliteTimeLayout = "2006-01-02 15:04:05"

// UpsertChannel writes a complete channel row (name/url/subscribers/alias/tags/
// state/blocked) in one statement. It is the import counterpart to the various
// narrow setters (AddSubscribedChannel/SetChannelAlias/SetChannelTags/…): the
// caller supplies a fully-resolved row (the service layer has already applied
// the bundle merge policy — union tags, incoming-wins alias, etc.), and this
// writes exactly those values. The block invariant is enforced in SQL: a blocked
// row is forced to subscription_state='none' regardless of the state passed in,
// so a malformed bundle can never persist a blocked+subscribed channel. No-op on
// an empty channel ID (the primary key).
func (d *DB) UpsertChannel(ctx context.Context, ch domain.Channel) error {
	if ch.ID == "" {
		return nil
	}
	state := ch.SubState()
	if ch.Blocked {
		state = domain.SubNone
	}
	isLocal := 0
	if state == domain.SubLocal {
		isLocal = 1
	}
	blocked := 0
	if ch.Blocked {
		blocked = 1
	}
	if _, err := d.sql.ExecContext(ctx, `
		INSERT INTO subscribed_channels
			(channel_id, name, url, subscribers, alias, tags, is_local, subscription_state, blocked)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(channel_id) DO UPDATE SET
			name=excluded.name, url=excluded.url, subscribers=excluded.subscribers,
			alias=excluded.alias, tags=excluded.tags, is_local=excluded.is_local,
			subscription_state=excluded.subscription_state, blocked=excluded.blocked,
			updated_at=CURRENT_TIMESTAMP
	`, ch.ID, ch.Name, ch.URL, ch.Subscribers, ch.Alias, strings.Join(ch.Tags, ","),
		isLocal, string(state), blocked); err != nil {
		return fmt.Errorf("UpsertChannel: %w", err)
	}
	return nil
}

// AddBlockedName records a name-only block in the blocked_names side table (used
// when a bundle carries a blocked entry whose channel_id is unknown). Idempotent
// via INSERT OR IGNORE; no-op on an empty name.
func (d *DB) AddBlockedName(ctx context.Context, name string) error {
	if name == "" {
		return nil
	}
	if _, err := d.sql.ExecContext(ctx, `INSERT OR IGNORE INTO blocked_names (name) VALUES (?)`, name); err != nil {
		return fmt.Errorf("AddBlockedName: %w", err)
	}
	return nil
}

// AddHistoryEvent records a history event at an explicit timestamp — the import
// counterpart to AddHistory, which always stamps the current time. Preserving
// the original event time keeps an imported log ordered as it was on the source
// machine. An empty videoID is stored as NULL (search events), mirroring
// AddHistory. Dedup is the caller's concern (the service filters events already
// present before calling this).
func (d *DB) AddHistoryEvent(ctx context.Context, videoID, eventType, details string, ts time.Time) error {
	var vid interface{}
	if videoID != "" {
		vid = videoID
	}
	if _, err := d.sql.ExecContext(ctx, `
		INSERT INTO history (video_id, event_type, details, timestamp) VALUES (?, ?, ?, ?)
	`, vid, eventType, details, ts.UTC().Format(sqliteTimeLayout)); err != nil {
		return fmt.Errorf("AddHistoryEvent: %w", err)
	}
	return nil
}
