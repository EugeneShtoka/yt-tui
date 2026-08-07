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

func (d *DB) migrate() error { //nolint:funlen // flat DDL: one CREATE per table + one exec loop, no logic to extract
	ctx := context.Background()
	_, err := d.sql.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS videos (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			channel TEXT,
			channel_id TEXT,
			duration INTEGER DEFAULT 0,
			view_count INTEGER DEFAULT 0,
			upload_date TEXT,
			url TEXT,
			added_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS local_videos (
			id TEXT PRIMARY KEY REFERENCES videos(id),
			file_path TEXT NOT NULL,
			download_type TEXT DEFAULT 'video',
			downloaded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			status TEXT DEFAULT 'new',
			last_played DATETIME,
			last_position_ms INTEGER NOT NULL DEFAULT 0,
			file_size INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			video_id TEXT REFERENCES videos(id) ON DELETE CASCADE,
			event_type TEXT NOT NULL,
			details TEXT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS playlists (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS playlist_videos (
			playlist_id INTEGER REFERENCES playlists(id) ON DELETE CASCADE,
			video_id TEXT REFERENCES videos(id),
			position INTEGER DEFAULT 0,
			added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (playlist_id, video_id)
		);

		CREATE TABLE IF NOT EXISTS watch_later (
			video_id TEXT PRIMARY KEY,
			title TEXT,
			channel TEXT,
			url TEXT,
			added_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS feed_cache (
			feed TEXT NOT NULL,
			video_id TEXT NOT NULL,
			position INTEGER NOT NULL,
			PRIMARY KEY (feed, video_id)
		);

		CREATE INDEX IF NOT EXISTS idx_feed_cache_feed ON feed_cache(feed, position);
		CREATE INDEX IF NOT EXISTS idx_history_timestamp ON history(timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_history_video ON history(video_id);
		CREATE INDEX IF NOT EXISTS idx_videos_upload_date ON videos(upload_date DESC);

		CREATE TABLE IF NOT EXISTS hidden_rec_videos (
			video_id TEXT PRIMARY KEY,
			hidden_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return fmt.Errorf("migrate schema: %w", err)
	}
	for _, stmt := range schemaStatements {
		if _, err = d.sql.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate create table: %w", err)
		}
	}
	return nil
}

// schemaStatements holds the remaining CREATE TABLE/INDEX (and index cleanup)
// statements that don't fit the single inline block above. All are idempotent so
// they simply run on every startup; together with the inline block they are the
// complete, authoritative schema (this is the first DB version — no migrations).
var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS subscribed_channels (
		channel_id          TEXT PRIMARY KEY,
		name                TEXT NOT NULL DEFAULT '',
		url                 TEXT NOT NULL DEFAULT '',
		subscribers         INTEGER NOT NULL DEFAULT 0,
		alias               TEXT NOT NULL DEFAULT '',
		tags                TEXT NOT NULL DEFAULT '',
		is_local            INTEGER NOT NULL DEFAULT 0,
		subscription_state  TEXT NOT NULL DEFAULT 'none',
		blocked             INTEGER NOT NULL DEFAULT 0,
		videos_refreshed_at INTEGER NOT NULL DEFAULT 0,
		fetched_videos      INTEGER NOT NULL DEFAULT 0,
		last_activity_at    INTEGER NOT NULL DEFAULT 0,
		updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS channel_videos (
		channel_id TEXT NOT NULL,
		video_id   TEXT NOT NULL REFERENCES videos(id),
		fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (channel_id, video_id)
	)`,
	// Superseded by idx_channel_videos_video: this duplicated the table's own
	// PRIMARY KEY column-for-column, adding write overhead with no query benefit.
	`DROP INDEX IF EXISTS idx_channel_videos_channel`,
	`CREATE INDEX IF NOT EXISTS idx_channel_videos_video ON channel_videos(video_id)`,
	`CREATE TABLE IF NOT EXISTS yt_playlists (
		id         TEXT PRIMARY KEY,
		title      TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS yt_playlist_videos (
		playlist_id TEXT NOT NULL,
		video_id    TEXT NOT NULL REFERENCES videos(id),
		position    INTEGER DEFAULT 0,
		fetched_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (playlist_id, video_id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_yt_playlist_videos_video ON yt_playlist_videos(video_id)`,
	`CREATE INDEX IF NOT EXISTS idx_playlist_videos_video ON playlist_videos(video_id)`,
	`CREATE INDEX IF NOT EXISTS idx_feed_cache_video ON feed_cache(video_id)`,
	`CREATE TABLE IF NOT EXISTS video_details_cache (
		video_id      TEXT PRIMARY KEY,
		description   TEXT NOT NULL DEFAULT '',
		thumbnail_url TEXT NOT NULL DEFAULT '',
		subscribers   INTEGER NOT NULL DEFAULT 0,
		links         TEXT,
		chapters      TEXT,
		sb_segments   TEXT,
		fetched_at    DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS activity_log (
		id               INTEGER PRIMARY KEY AUTOINCREMENT,
		type             TEXT NOT NULL,
		is_local         INTEGER NOT NULL DEFAULT 0,
		channel_id       TEXT,
		channel_name     TEXT,
		playlist_id      TEXT,
		playlist_local_id INTEGER,
		playlist_name    TEXT,
		video_id         TEXT,
		video_title      TEXT,
		timestamp        DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE TABLE IF NOT EXISTS video_positions (
		video_id    TEXT PRIMARY KEY REFERENCES videos(id) ON DELETE CASCADE,
		position_ms INTEGER NOT NULL DEFAULT 0,
		updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	// blocked_names holds legacy name-only blocks whose channel_id isn't known
	// yet (the DB-side successor to config's name-only blacklist entries). They
	// graduate to a blocked subscribed_channels row on first ID resolution.
	`CREATE TABLE IF NOT EXISTS blocked_names (
		name TEXT PRIMARY KEY
	)`,
}

func (d *DB) cleanEmojiTitles() error {
	ctx := context.Background()
	type tableCol struct{ table, idCol, titleCol string }
	targets := []tableCol{
		{"videos", "id", "title"},
		{"watch_later", "video_id", "title"},
		{"yt_playlists", "id", "title"},
	}
	for _, t := range targets {
		rows, err := d.sql.QueryContext(ctx, "SELECT "+t.idCol+", "+t.titleCol+" FROM "+t.table)
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
		`DELETE FROM yt_playlist_videos WHERE video_id IN (SELECT id FROM videos WHERE view_count=0 AND id NOT IN (SELECT id FROM local_videos))`,
		`DELETE FROM playlist_videos WHERE video_id IN (SELECT id FROM videos WHERE view_count=0 AND id NOT IN (SELECT id FROM local_videos))`,
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
