package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Blocklist returns the current block projection used to filter the feed: the
// IDs of channels flagged blocked=1, plus any unresolved name-only blocks from
// the blocked_names side table.
func (d *DB) Blocklist(ctx context.Context) (ids, names []string, err error) {
	if ids, err = d.scanColumn(ctx, `SELECT channel_id FROM subscribed_channels WHERE blocked=1`); err != nil {
		return nil, nil, fmt.Errorf("Blocklist ids: %w", err)
	}
	if names, err = d.scanColumn(ctx, `SELECT name FROM blocked_names`); err != nil {
		return nil, nil, fmt.Errorf("Blocklist names: %w", err)
	}
	return ids, names, nil
}

// scanColumn runs a single-column query and collects the values into a slice.
func (d *DB) scanColumn(ctx context.Context, query string) ([]string, error) {
	out, err := queryList(ctx, d.sql, query, scanString)
	if err != nil {
		return nil, fmt.Errorf("scanColumn: %w", err)
	}
	return out, nil
}

// ResolveBlockedName upgrades a name-only block to an ID-keyed block once a video
// reveals the channel's ID: it flags the channel row blocked=1/state='none' and
// drops the now-redundant blocked_names entry, atomically in one transaction.
// No-op on empty input.
func (d *DB) ResolveBlockedName(ctx context.Context, name, channelID string) error {
	if name == "" || channelID == "" {
		return nil
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ResolveBlockedName begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed
	if err := blockChannelIDTx(ctx, tx, channelID, name); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM blocked_names WHERE name=?`, name); err != nil {
		return fmt.Errorf("ResolveBlockedName delete: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ResolveBlockedName commit: %w", err)
	}
	return nil
}

// BlockChannel blocks a channel by ID: a guarded transition that atomically sets
// blocked=1 and subscription_state='none' (the block invariant — blocking
// unsubscribes). Idempotent; creates the row if absent. The channel's cached
// videos are the caller's concern (the service layer clears them, matching
// Unsubscribe). An existing display name is preserved.
func (d *DB) BlockChannel(ctx context.Context, channelID string) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("BlockChannel begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed
	if err := blockChannelIDTx(ctx, tx, channelID, ""); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("BlockChannel commit: %w", err)
	}
	return nil
}

// UnblockChannel clears the blocked flag on a channel, leaving it at
// subscription_state='none' (we never auto-restore a prior subscription — the
// user re-subscribes deliberately). No-op if the channel isn't blocked.
func (d *DB) UnblockChannel(ctx context.Context, channelID string) error {
	if _, err := d.sql.ExecContext(ctx,
		`UPDATE subscribed_channels SET blocked=0, updated_at=CURRENT_TIMESTAMP WHERE channel_id=?`,
		channelID,
	); err != nil {
		return fmt.Errorf("UnblockChannel: %w", err)
	}
	return nil
}

// blockChannelIDTx marks a channel blocked by ID inside tx, enforcing the block
// invariant (blocked=1 ⟹ subscription_state='none', i.e. blocking unsubscribes).
// An existing non-empty name is preserved when name is empty.
func blockChannelIDTx(ctx context.Context, tx *sql.Tx, id, name string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO subscribed_channels (channel_id, name, blocked, subscription_state)
		VALUES (?, ?, 1, 'none')
		ON CONFLICT(channel_id) DO UPDATE SET
			blocked=1,
			subscription_state='none',
			name=CASE WHEN excluded.name != '' THEN excluded.name ELSE subscribed_channels.name END,
			updated_at=CURRENT_TIMESTAMP
	`, id, name); err != nil {
		return fmt.Errorf("blockChannelIDTx: %w", err)
	}
	return nil
}
