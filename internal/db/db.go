package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/youtube"
	_ "modernc.org/sqlite"
)

// DB wraps a SQLite connection for all yt-tui persistence.
type DB struct {
	sql *sql.DB
}

// New opens (or creates) the database, runs all migrations, and applies
// startup maintenance (emoji cleanup, member-video pruning, feed age pruning).
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
	if err := d.runVersionedMigrations(); err != nil {
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
	return d, nil
}

// Close closes the underlying SQLite connection.
func (d *DB) Close() error {
	if err := d.sql.Close(); err != nil {
		return fmt.Errorf("DB.Close: %w", err)
	}
	return nil
}

// versionedMigrations is an ordered list of one-time data migrations. To add a new
// migration, append an entry — the version number must be strictly increasing.
// user_version is advanced automatically after each migration runs.
// Use this for non-schema one-time actions; schema changes to video_details_cache
// are detected automatically by checkAndClearCacheIfChanged.
var versionedMigrations = []struct {
	version int
	run     func(db *sql.DB) error
}{
	{
		version: 1,
		run: func(db *sql.DB) error {
			ctx := context.Background()
			if _, err := db.ExecContext(ctx, `UPDATE history SET event_type = 'playVideo' WHERE event_type = 'play'`); err != nil {
				return fmt.Errorf("migration v1: %w", err)
			}
			return nil
		},
	},
	{
		version: 2,
		run: func(db *sql.DB) error {
			ctx := context.Background()
			if _, err := db.ExecContext(ctx, `
				INSERT OR IGNORE INTO video_positions (video_id, position_ms)
				SELECT id, last_position_ms FROM local_videos WHERE last_position_ms > 0
			`); err != nil {
				return fmt.Errorf("migration v2: %w", err)
			}
			return nil
		},
	},
	{
		// Recreate video_positions with FK → videos ON DELETE CASCADE.
		// Orphaned rows (no matching videos entry) are dropped.
		version: 3,
		run: func(db *sql.DB) error {
			ctx := context.Background()
			if _, err := db.ExecContext(ctx, `
				CREATE TABLE video_positions_new (
					video_id    TEXT PRIMARY KEY REFERENCES videos(id) ON DELETE CASCADE,
					position_ms INTEGER NOT NULL DEFAULT 0,
					updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
				);
				INSERT INTO video_positions_new (video_id, position_ms, updated_at)
					SELECT video_id, position_ms, updated_at
					FROM video_positions
					WHERE video_id IN (SELECT id FROM videos);
				DROP TABLE video_positions;
				ALTER TABLE video_positions_new RENAME TO video_positions;
			`); err != nil {
				return fmt.Errorf("migration v3: %w", err)
			}
			return nil
		},
	},
	{
		// Recreate history with nullable video_id FK → videos ON DELETE CASCADE.
		// Search entries (video_id = '') become NULL; orphaned video entries are dropped.
		version: 4,
		run: func(db *sql.DB) error {
			ctx := context.Background()
			if _, err := db.ExecContext(ctx, `
				CREATE TABLE history_new (
					id         INTEGER PRIMARY KEY AUTOINCREMENT,
					video_id   TEXT REFERENCES videos(id) ON DELETE CASCADE,
					event_type TEXT NOT NULL,
					details    TEXT,
					timestamp  DATETIME DEFAULT CURRENT_TIMESTAMP
				);
				INSERT INTO history_new (id, video_id, event_type, details, timestamp)
					SELECT id,
					       CASE WHEN video_id = '' THEN NULL ELSE video_id END,
					       event_type, details, timestamp
					FROM history
					WHERE video_id = ''
					   OR video_id IN (SELECT id FROM videos);
				DROP TABLE history;
				ALTER TABLE history_new RENAME TO history;
				CREATE INDEX IF NOT EXISTS idx_history_timestamp ON history(timestamp DESC);
				CREATE INDEX IF NOT EXISTS idx_history_video ON history(video_id);
			`); err != nil {
				return fmt.Errorf("migration v4: %w", err)
			}
			return nil
		},
	},
	{
		version: 5,
		run: func(db *sql.DB) error {
			ctx := context.Background()
			if _, err := db.ExecContext(ctx, `UPDATE history SET event_type = 'streamVideo' WHERE event_type = 'stream'`); err != nil {
				return fmt.Errorf("migration v5: %w", err)
			}
			return nil
		},
	},
}

func (d *DB) runVersionedMigrations() error {
	ctx := context.Background()
	var current int
	if err := d.sql.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("runVersionedMigrations read version: %w", err)
	}
	for _, m := range versionedMigrations {
		if m.version <= current {
			continue
		}
		if err := m.run(d.sql); err != nil {
			return err
		}
		if _, err := d.sql.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, m.version)); err != nil {
			return fmt.Errorf("runVersionedMigrations set version %d: %w", m.version, err)
		}
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

func (d *DB) migrate() error {
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
			last_played DATETIME
		);

		CREATE TABLE IF NOT EXISTS history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			video_id TEXT NOT NULL,
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
	// Tables added after initial schema.
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS subscribed_channels (
			channel_id TEXT PRIMARY KEY,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS channel_videos (
			channel_id TEXT NOT NULL,
			video_id   TEXT NOT NULL REFERENCES videos(id),
			fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (channel_id, video_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_channel_videos_channel ON channel_videos(channel_id, video_id)`,
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
		`CREATE TABLE IF NOT EXISTS video_details_cache (
			video_id      TEXT PRIMARY KEY,
			description   TEXT NOT NULL DEFAULT '',
			thumbnail_url TEXT NOT NULL DEFAULT '',
			subscribers   INTEGER NOT NULL DEFAULT 0,
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
			video_id    TEXT PRIMARY KEY,
			position_ms INTEGER NOT NULL DEFAULT 0,
			updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	} {
		if _, err = d.sql.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate create table: %w", err)
		}
	}
	// Columns added after initial schema; safe to ignore "duplicate column" errors.
	// These must run after all CREATE TABLE statements so the tables exist on fresh installs.
	for _, col := range []string{
		`ALTER TABLE local_videos ADD COLUMN last_position_ms INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE video_details_cache ADD COLUMN links TEXT`,
		`ALTER TABLE video_details_cache ADD COLUMN chapters TEXT`,
		`ALTER TABLE video_details_cache ADD COLUMN sb_segments TEXT`,
		`ALTER TABLE subscribed_channels ADD COLUMN name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE subscribed_channels ADD COLUMN url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE subscribed_channels ADD COLUMN subscribers INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE subscribed_channels ADD COLUMN alias TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE subscribed_channels ADD COLUMN tags TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE subscribed_channels ADD COLUMN is_local INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err = d.sql.ExecContext(ctx, col); err != nil && !isColumnExists(err) {
			return fmt.Errorf("migrate alter: %w", err)
		}
	}
	return nil
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
			if clean := youtube.StripEmojis(r.title); clean != r.title {
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

// isColumnExists returns true when an ALTER TABLE error means the column already exists.
func isColumnExists(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "duplicate column name") || strings.Contains(msg, "already exists")
}
