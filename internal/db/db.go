package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/text"
	_ "modernc.org/sqlite"
)

// DB wraps a SQLite connection for all yt-tui persistence.
type DB struct {
	sql *sql.DB
}

// withTx runs fn inside a transaction, centralizing the BeginTx → defer Rollback
// → Commit envelope every transactional writer used to hand-roll. It rolls back
// on any error or panic and commits on success; label prefixes the begin/commit
// error context, while fn wraps its own statement errors. (Tx.Rollback is a
// best-effort no-op once Commit succeeds; errcheck excludes it.)
func (d *DB) withTx(ctx context.Context, label string, fn func(tx *sql.Tx) error) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s begin: %w", label, err)
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s commit: %w", label, err)
	}
	return nil
}

// placeholders returns a comma-separated run of n SQL "?" placeholders for an
// IN (...) clause — "?,?,?" for n==3. Returns "" for n<=0 so callers guard the
// empty case before building the query.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?,", n-1) + "?"
}

// New opens (or creates) the database, runs all migrations, and applies startup
// maintenance (emoji cleanup, member-video pruning, feed age pruning, and
// download reconciliation against out-of-band file changes).
func New(dataDir string, stripEmojis bool, recommendedMaxAgeDays int) (*DB, error) {
	path := filepath.Join(dataDir, "yt-tui.db")
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("New open: %w", err)
	}
	// Single connection serializes all writes; prevents SQLITE_BUSY from concurrent goroutines.
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.ExecContext(context.Background(), `PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;`); err != nil {
		return nil, fmt.Errorf("New pragma: %w", err)
	}
	d := &DB{sql: sqlDB}
	if err := d.migrate(); err != nil {
		return nil, err
	}
	if err := d.checkAndClearCacheIfChanged(); err != nil {
		return nil, err
	}
	if stripEmojis {
		if err := d.cleanEmojiTitles(); err != nil {
			return nil, err
		}
	}
	if err := d.deleteMemberVideos(); err != nil {
		return nil, err
	}
	if err := d.pruneRecommendedFeed(recommendedMaxAgeDays); err != nil {
		return nil, err
	}
	if err := d.reconcileDownloads(context.Background()); err != nil {
		return nil, err
	}
	return d, nil
}

// Close closes the underlying SQLite connection.
func (d *DB) Close() error {
	if err := d.sql.Close(); err != nil {
		return fmt.Errorf("DB.Close: %w", err)
	}
	return nil
}

// checkAndClearCacheIfChanged computes a fingerprint of video_details_cache columns
// and clears the table whenever the schema changes. This means adding or removing
// a column automatically invalidates all cached entries on next startup.
func (d *DB) checkAndClearCacheIfChanged() error {
	ctx := context.Background()
	rows, err := d.sql.QueryContext(ctx, `PRAGMA table_info(video_details_cache)`)
	if err != nil {
		return fmt.Errorf("checkAndClearCacheIfChanged query: %w", err)
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt any
		if err = rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return fmt.Errorf("checkAndClearCacheIfChanged scan: %w", err)
		}
		parts = append(parts, name+":"+colType)
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("checkAndClearCacheIfChanged rows: %w", err)
	}
	fingerprint := strings.Join(parts, ",")

	var stored string
	err = d.sql.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='cache_schema'`).Scan(&stored)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("checkAndClearCacheIfChanged read schema: %w", err)
	}
	if fingerprint == stored {
		return nil
	}
	if _, err = d.sql.ExecContext(ctx, `DELETE FROM video_details_cache`); err != nil {
		return fmt.Errorf("checkAndClearCacheIfChanged delete: %w", err)
	}
	if _, err = d.sql.ExecContext(ctx, `INSERT OR REPLACE INTO meta (key, value) VALUES ('cache_schema', ?)`, fingerprint); err != nil {
		return fmt.Errorf("checkAndClearCacheIfChanged update schema: %w", err)
	}
	return nil
}

func (d *DB) cleanEmojiTitles() error {
	ctx := context.Background()
	type tableCol struct{ table, idCol, titleCol string }
	targets := []tableCol{
		{"videos", "id", "title"},
		{"videos", "id", "channel"}, // denormalized channel name shown per video
		{"collections", "id", "name"},
		{"subscribed_channels", "channel_id", "name"}, // channel names in Channels/Tags
	}
	for _, t := range targets {
		// COALESCE so a nullable text column (e.g. videos.channel) scans as "".
		type row struct{ id, title string }
		all, err := queryList(ctx, d.sql, "SELECT "+t.idCol+", COALESCE("+t.titleCol+",'') FROM "+t.table,
			func(rows *sql.Rows) (row, error) {
				var r row
				return r, rows.Scan(&r.id, &r.title)
			})
		if err != nil {
			return fmt.Errorf("cleanEmojiTitles query %s: %w", t.table, err)
		}
		for _, r := range all {
			clean := text.StripEmojis(r.title)
			if clean == r.title {
				continue
			}
			if _, err := d.sql.ExecContext(ctx, "UPDATE "+t.table+" SET "+t.titleCol+"=? WHERE "+t.idCol+"=?", clean, r.id); err != nil {
				return fmt.Errorf("cleanEmojiTitles update %s: %w", t.table, err)
			}
		}
	}
	return nil
}

// deleteMemberVideos removes member-only videos (view_count=0) that a channel
// crawl saved before the parser filtered them. A zero count is only a guess at
// "members-only" — the recommended feed carries no view counts at all — so a
// video the user has touched is never pruned: downloaded, in the recommended
// cache, hidden, in a playlist, or in History. Deleting those erased user data
// on every start (playlist entries, hides, and History rows via ON DELETE
// CASCADE) and emptied the recommended cache.
func (d *DB) deleteMemberVideos() error {
	ctx := context.Background()
	const pruned = `SELECT id FROM videos WHERE view_count=0
		AND id NOT IN (SELECT id FROM local_videos)
		AND id NOT IN (SELECT video_id FROM feed_cache WHERE feed='recommended')
		AND id NOT IN (SELECT video_id FROM hidden_rec_videos)
		AND id NOT IN (SELECT video_id FROM collection_videos)
		AND id NOT IN (SELECT video_id FROM history WHERE video_id IS NOT NULL)`
	for _, stmt := range []string{
		`DELETE FROM feed_cache WHERE video_id IN (` + pruned + `)`,
		`DELETE FROM channel_videos WHERE video_id IN (` + pruned + `)`,
		`DELETE FROM video_details_cache WHERE video_id IN (` + pruned + `)`,
		`DELETE FROM videos WHERE id IN (` + pruned + `)`,
	} {
		if _, err := d.sql.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("deleteMemberVideos: %w", err)
		}
	}
	return nil
}
