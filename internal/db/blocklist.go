package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Blocklist returns the IDs of channels flagged blocked=1 — the projection used
// to filter the recommended feed.
func (d *DB) Blocklist(ctx context.Context) ([]string, error) {
	ids, err := d.scanColumn(ctx, `SELECT channel_id FROM subscribed_channels WHERE blocked=1`)
	if err != nil {
		return nil, fmt.Errorf("Blocklist: %w", err)
	}
	return ids, nil
}

// scanColumn runs a single-column query and collects the values into a slice.
func (d *DB) scanColumn(ctx context.Context, query string) ([]string, error) {
	out, err := queryList(ctx, d.sql, query, scanString)
	if err != nil {
		return nil, fmt.Errorf("scanColumn: %w", err)
	}
	return out, nil
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
	if err := blockChannelIDTx(ctx, tx, channelID); err != nil {
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
// A new row is created with an empty name; an existing row's name is preserved.
func blockChannelIDTx(ctx context.Context, tx *sql.Tx, id string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO subscribed_channels (channel_id, name, blocked, subscription_state)
		VALUES (?, '', 1, 'none')
		ON CONFLICT(channel_id) DO UPDATE SET
			blocked=1,
			subscription_state='none',
			updated_at=CURRENT_TIMESTAMP
	`, id); err != nil {
		return fmt.Errorf("blockChannelIDTx: %w", err)
	}
	return nil
}
