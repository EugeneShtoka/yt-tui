package db

import (
	"context"
	"database/sql"
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
	var parts []string
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt interface{}
		if err = rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("checkAndClearCacheIfChanged scan: %w", err)
		}
		parts = append(parts, name+":"+colType)
	}
	rows.Close()
	fingerprint := strings.Join(parts, ",")

	var stored string
	err = d.sql.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='cache_schema'`).Scan(&stored)
	if err != nil && err != sql.ErrNoRows {
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
		rows, err := d.sql.QueryContext(ctx, "SELECT "+t.idCol+", COALESCE("+t.titleCol+",'') FROM "+t.table)
		if err != nil {
			return fmt.Errorf("cleanEmojiTitles query %s: %w", t.table, err)
		}
		type row struct{ id, title string }
		var updates []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.title); err != nil {
				rows.Close()
				return fmt.Errorf("cleanEmojiTitles scan %s: %w", t.table, err)
			}
			if clean := text.StripEmojis(r.title); clean != r.title {
				updates = append(updates, row{r.id, clean})
			}
		}
		rows.Close()
		for _, u := range updates {
			if _, err := d.sql.ExecContext(ctx, "UPDATE "+t.table+" SET "+t.titleCol+"=? WHERE "+t.idCol+"=?", u.title, u.id); err != nil {
				return fmt.Errorf("cleanEmojiTitles update %s: %w", t.table, err)
			}
		}
	}
	return nil
}

// deleteMemberVideos removes member-only videos (view_count=0) from the DB.
// Videos that have been downloaded (present in local_videos) are preserved.
func (d *DB) deleteMemberVideos() error {
	ctx := context.Background()
	for _, stmt := range []string{
		`DELETE FROM feed_cache WHERE video_id IN (SELECT id FROM videos WHERE view_count=0 AND id NOT IN (SELECT id FROM local_videos))`,
		`DELETE FROM channel_videos WHERE video_id IN (SELECT id FROM videos WHERE view_count=0 AND id NOT IN (SELECT id FROM local_videos))`,
		`DELETE FROM collection_videos WHERE video_id IN (SELECT id FROM videos WHERE view_count=0 AND id NOT IN (SELECT id FROM local_videos))`,
		`DELETE FROM hidden_rec_videos WHERE video_id IN (SELECT id FROM videos WHERE view_count=0 AND id NOT IN (SELECT id FROM local_videos))`,
		`DELETE FROM video_details_cache WHERE video_id IN (SELECT id FROM videos WHERE view_count=0 AND id NOT IN (SELECT id FROM local_videos))`,
		`DELETE FROM videos WHERE view_count=0 AND id NOT IN (SELECT id FROM local_videos)`,
	} {
		if _, err := d.sql.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("deleteMemberVideos: %w", err)
		}
	}
	return nil
}
