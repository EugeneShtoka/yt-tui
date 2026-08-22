package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// legacyBaselineSQL is the schema a pre-runner database carries: the 0001
// baseline plus the four drifts 0002 reconciles (feed_cache.position,
// local_videos.last_position_ms, an INTEGER activity_log.playlist_local_id, a
// leftover blocked_names table, and no activity_log timestamp index). It is
// stamped user_version = 2 by hand, exactly as the pre-runner code did.
const legacyBaselineSQL = `
CREATE TABLE videos (
    id TEXT PRIMARY KEY, title TEXT NOT NULL, channel TEXT, channel_id TEXT,
    duration INTEGER DEFAULT 0, view_count INTEGER DEFAULT 0, upload_date TEXT,
    url TEXT, added_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE local_videos (
    id TEXT PRIMARY KEY REFERENCES videos(id), file_path TEXT NOT NULL,
    download_type TEXT DEFAULT 'video', downloaded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    status TEXT DEFAULT 'new', last_played DATETIME,
    last_position_ms INTEGER NOT NULL DEFAULT 0, file_size INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE history (
    id INTEGER PRIMARY KEY AUTOINCREMENT, video_id TEXT REFERENCES videos(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL, details TEXT, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE collections (
    id TEXT PRIMARY KEY, kind TEXT NOT NULL, name TEXT NOT NULL,
    synced INTEGER NOT NULL DEFAULT 0, created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE collection_videos (
    collection_id TEXT NOT NULL, video_id TEXT NOT NULL REFERENCES videos(id),
    added_at DATETIME DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (collection_id, video_id)
);
CREATE TABLE feed_cache (
    feed TEXT NOT NULL, video_id TEXT NOT NULL, position INTEGER NOT NULL,
    PRIMARY KEY (feed, video_id)
);
CREATE TABLE hidden_rec_videos (
    video_id TEXT PRIMARY KEY, hidden_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE subscribed_channels (
    channel_id TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '', url TEXT NOT NULL DEFAULT '',
    subscribers INTEGER NOT NULL DEFAULT 0, alias TEXT NOT NULL DEFAULT '',
    tags TEXT NOT NULL DEFAULT '', is_local INTEGER NOT NULL DEFAULT 0,
    subscription_state TEXT NOT NULL DEFAULT 'none', blocked INTEGER NOT NULL DEFAULT 0,
    videos_refreshed_at INTEGER NOT NULL DEFAULT 0, fetched_videos INTEGER NOT NULL DEFAULT 0,
    last_activity_at INTEGER NOT NULL DEFAULT 0, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE channel_videos (
    channel_id TEXT NOT NULL, video_id TEXT NOT NULL REFERENCES videos(id),
    fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (channel_id, video_id)
);
CREATE TABLE video_details_cache (
    video_id TEXT PRIMARY KEY, description TEXT NOT NULL DEFAULT '',
    thumbnail_url TEXT NOT NULL DEFAULT '', subscribers INTEGER NOT NULL DEFAULT 0,
    links TEXT, chapters TEXT, sb_segments TEXT, fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE activity_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT, type TEXT NOT NULL,
    is_local INTEGER NOT NULL DEFAULT 0, channel_id TEXT, channel_name TEXT,
    playlist_id TEXT, playlist_local_id INTEGER, playlist_name TEXT,
    video_id TEXT, video_title TEXT, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');
CREATE TABLE video_positions (
    video_id TEXT PRIMARY KEY REFERENCES videos(id) ON DELETE CASCADE,
    position_ms INTEGER NOT NULL DEFAULT 0, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE blocked_names (name TEXT PRIMARY KEY);
CREATE UNIQUE INDEX idx_collections_local_name ON collections(name) WHERE kind='local';
CREATE INDEX idx_feed_cache_feed ON feed_cache(feed, position);
CREATE INDEX idx_history_timestamp ON history(timestamp DESC);
CREATE INDEX idx_history_video ON history(video_id);
CREATE INDEX idx_videos_upload_date ON videos(upload_date DESC);
CREATE INDEX idx_channel_videos_video ON channel_videos(video_id);
CREATE INDEX idx_collection_videos_video ON collection_videos(video_id);
CREATE INDEX idx_feed_cache_video ON feed_cache(video_id);
PRAGMA user_version = 2;
`

// newLegacyDataDir writes a legacy-shaped database into a temp data dir, seeds it
// with rows in the drifted tables, and returns the dir for db.New to open.
func newLegacyDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	raw, err := sql.Open("sqlite", filepath.Join(dir, "yt-tui.db"))
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer func() { _ = raw.Close() }()
	ctx := context.Background()
	if _, err := raw.ExecContext(ctx, legacyBaselineSQL); err != nil {
		t.Fatalf("apply legacy schema: %v", err)
	}
	// Seed rows the rebuild must carry across: a video, a downloaded file, a
	// ranked feed_cache entry and an activity_log entry with a string local id.
	// upload_date stays empty so the startup age prune leaves it alone, and the
	// download points at a real file so reconcileDownloads keeps its row.
	if err := os.WriteFile(legacyDownloadPath(dir), []byte("x"), 0o600); err != nil {
		t.Fatalf("write legacy download file: %v", err)
	}
	seed := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO videos (id, title, channel, channel_id, upload_date, view_count)
		  VALUES ('vid1', 'Legacy Video', 'Chan', 'ch1', '', 42)`, nil},
		{`INSERT INTO local_videos (id, file_path, last_position_ms) VALUES ('vid1', ?, 5000)`,
			[]any{legacyDownloadPath(dir)}},
		{`INSERT INTO feed_cache (feed, video_id, position) VALUES ('recommended', 'vid1', 0)`, nil},
		{`INSERT INTO activity_log (type, playlist_local_id, playlist_name)
		  VALUES ('playlist_add', 'local:7', 'Watch Later')`, nil},
	}
	for _, s := range seed {
		if _, err := raw.ExecContext(ctx, s.query, s.args...); err != nil {
			t.Fatalf("seed legacy db: %v", err)
		}
	}
	return dir
}

// legacyDownloadPath is the on-disk file the seeded local_videos row points at.
func legacyDownloadPath(dir string) string { return filepath.Join(dir, "vid1.mp4") }

// TestMigrateReconcilesLegacySchema is the regression for the broken recommended
// feed: a legacy database stamped user_version = 2 skipped every migration, so
// feed_cache kept a NOT NULL `position` column SaveFeedCache never writes. Every
// cache write failed the constraint and rolled back, leaving the feed to refetch
// from scratch on each open and never persist.
func TestMigrateReconcilesLegacySchema(t *testing.T) {
	dir := newLegacyDataDir(t)
	db, err := New(dir, false, 90)
	if err != nil {
		t.Fatalf("New on legacy database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	// The write path that the drift broke now round-trips.
	videos := []domain.Video{
		{ID: "rec1", Title: "First", Channel: "Chan", ChannelID: "ch1", URL: "https://example.com/rec1"},
		{ID: "rec2", Title: "Second", Channel: "Chan", ChannelID: "ch1", URL: "https://example.com/rec2"},
	}
	if err = db.SaveFeedCache(ctx, "recommended", videos); err != nil {
		t.Fatalf("SaveFeedCache after reconcile: %v", err)
	}
	got, err := db.GetFeedCache(ctx, "recommended")
	if err != nil {
		t.Fatalf("GetFeedCache: %v", err)
	}
	if len(got) != 2 || got[0].ID != "rec1" || got[1].ID != "rec2" {
		t.Fatalf("GetFeedCache = %+v, want rec1, rec2 in write order", got)
	}

	// Drifted columns are gone and the legacy leftovers cleaned up.
	var has bool
	for _, c := range []struct{ table, column string }{
		{"feed_cache", "position"},
		{"local_videos", "last_position_ms"},
	} {
		has, err = db.hasColumn(ctx, c.table, c.column)
		if err != nil {
			t.Fatalf("hasColumn: %v", err)
		}
		if has {
			t.Errorf("%s.%s still present after reconcile", c.table, c.column)
		}
	}
	assertObjectAbsent(t, db, "table", "blocked_names")
	assertObjectPresent(t, db, "index", "idx_activity_log_timestamp")

	// playlist_local_id holds "local:N" strings; the rebuild retyped it to TEXT.
	var colType string
	if err = db.sql.QueryRowContext(ctx,
		`SELECT type FROM pragma_table_info('activity_log') WHERE name='playlist_local_id'`).Scan(&colType); err != nil {
		t.Fatalf("read playlist_local_id type: %v", err)
	}
	if colType != "TEXT" {
		t.Errorf("activity_log.playlist_local_id type = %q, want TEXT", colType)
	}

	// Rebuilds preserved the seeded rows.
	assertCountArgs(t, db, `SELECT COUNT(*) FROM local_videos WHERE id='vid1' AND file_path=?`, 1, legacyDownloadPath(dir))
	assertCount(t, db, `SELECT COUNT(*) FROM activity_log WHERE playlist_local_id='local:7'`, 1)

	// The database now tracks the real migration count, so a reopen is a no-op
	// rather than a second reconcile.
	want, err := latestSchemaVersion()
	if err != nil {
		t.Fatalf("latestSchemaVersion: %v", err)
	}
	v, err := db.SchemaVersion(ctx)
	if err != nil || v != want {
		t.Fatalf("SchemaVersion = %d (err %v), want %d", v, err, want)
	}
	legacy, err := db.hasLegacySchema(ctx)
	if err != nil {
		t.Fatalf("hasLegacySchema: %v", err)
	}
	if legacy {
		t.Error("hasLegacySchema still true after reconcile; migrations would re-run forever")
	}
	if err := db.migrate(); err != nil {
		t.Fatalf("re-migrate after reconcile: %v", err)
	}
}

// TestFreshDatabaseIsNotLegacy guards the detector against misfiring on a
// database this runner built: a false positive would replay 0002 on every open.
func TestFreshDatabaseIsNotLegacy(t *testing.T) {
	db := newTestDB(t)
	legacy, err := db.hasLegacySchema(context.Background())
	if err != nil {
		t.Fatalf("hasLegacySchema: %v", err)
	}
	if legacy {
		t.Error("hasLegacySchema = true on a freshly migrated database")
	}
}

func assertObjectAbsent(t *testing.T, db *DB, kind, name string) {
	t.Helper()
	assertCount(t, db, fmt.Sprintf(`SELECT COUNT(*) FROM sqlite_master WHERE type='%s' AND name='%s'`, kind, name), 0)
}

func assertObjectPresent(t *testing.T, db *DB, kind, name string) {
	t.Helper()
	assertCount(t, db, fmt.Sprintf(`SELECT COUNT(*) FROM sqlite_master WHERE type='%s' AND name='%s'`, kind, name), 1)
}

func assertCount(t *testing.T, db *DB, query string, want int) {
	t.Helper()
	assertCountArgs(t, db, query, want)
}

func assertCountArgs(t *testing.T, db *DB, query string, want int, args ...any) {
	t.Helper()
	var n int
	if err := db.sql.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	if n != want {
		t.Errorf("%s = %d, want %d", query, n, want)
	}
}
