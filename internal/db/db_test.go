package db

import (
	"context"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := New(t.TempDir(), false, 90)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// upsertTestVideo inserts a minimal video row so FK-dependent tests can proceed.
func upsertTestVideo(t *testing.T, db *DB, id string) {
	t.Helper()
	if err := db.UpsertVideo(id, "Test Title", "Test Channel", "ch-"+id, 300, 1000, "20240101", "https://example.com/"+id); err != nil {
		t.Fatalf("UpsertVideo(%q): %v", id, err)
	}
}

// ── New / migrations ──────────────────────────────────────────────────────────

func TestNewDB(t *testing.T) {
	db, err := New(t.TempDir(), false, 90)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err = db.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestNewDBStripEmojis(t *testing.T) {
	db, err := New(t.TempDir(), true, 90)
	if err != nil {
		t.Fatalf("New stripEmojis=true: %v", err)
	}
	if err = db.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestMigrationsIdempotent(t *testing.T) {
	dir := t.TempDir()
	db1, err := New(dir, false, 90)
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	if err = db1.Close(); err != nil {
		t.Errorf("db1.Close: %v", err)
	}

	db2, err := New(dir, false, 90)
	if err != nil {
		t.Fatalf("second New (re-open): %v", err)
	}
	if err = db2.Close(); err != nil {
		t.Errorf("db2.Close: %v", err)
	}
}

// ── Versioned migrations ──────────────────────────────────────────────────────

func TestVersionedMigrations(t *testing.T) {
	db := newTestDB(t)
	var version int
	if err := db.sql.QueryRowContext(context.Background(), `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}
	want := len(versionedMigrations)
	if version != want {
		t.Errorf("user_version = %d, want %d", version, want)
	}
}

// ── UpsertVideo round-trip ────────────────────────────────────────────────────

func TestUpsertVideoRoundTrip(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")

	videos, err := db.GetFeedCache("nosuchfeed")
	if err != nil {
		t.Fatalf("GetFeedCache: %v", err)
	}
	if len(videos) != 0 {
		t.Errorf("expected empty feed cache, got %d", len(videos))
	}

	err = db.SaveFeedCache("test-feed", []domain.Video{
		{ID: "vid1", Title: "Test Title", Channel: "Test Channel", ChannelID: "ch-vid1", Duration: 300, ViewCount: 1000, UploadDate: "20240101"},
	})
	if err != nil {
		t.Fatalf("SaveFeedCache: %v", err)
	}

	got, err := db.GetFeedCache("test-feed")
	if err != nil {
		t.Fatalf("GetFeedCache: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("GetFeedCache len = %d, want 1", len(got))
	}
	if got[0].ID != "vid1" || got[0].Title != "Test Title" {
		t.Errorf("unexpected video: %+v", got[0])
	}
}

// ── History round-trips ───────────────────────────────────────────────────────

func TestAddHistoryAndRetrieve(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")

	if err := db.AddHistory("vid1", "playVideo", ""); err != nil {
		t.Fatalf("AddHistory: %v", err)
	}

	entries, err := db.HistoryVideos(10)
	if err != nil {
		t.Fatalf("HistoryVideos: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("HistoryVideos len = %d, want 1", len(entries))
	}
	if entries[0].VideoID != "vid1" {
		t.Errorf("VideoID = %q, want %q", entries[0].VideoID, "vid1")
	}
	if entries[0].EventType != "playVideo" {
		t.Errorf("EventType = %q, want %q", entries[0].EventType, "playVideo")
	}
}

func TestVideoHistory(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")

	events := []string{"playVideo", "streamVideo", "playAudio"}
	for _, evt := range events {
		if err := db.AddHistory("vid1", evt, ""); err != nil {
			t.Fatalf("AddHistory(%q): %v", evt, err)
		}
	}

	entries, err := db.VideoHistory("vid1")
	if err != nil {
		t.Fatalf("VideoHistory: %v", err)
	}
	if len(entries) != len(events) {
		t.Fatalf("VideoHistory len = %d, want %d", len(entries), len(events))
	}
	// All events must be present; VideoID must match throughout.
	for _, e := range entries {
		if e.VideoID != "vid1" {
			t.Errorf("VideoHistory entry VideoID = %q, want %q", e.VideoID, "vid1")
		}
	}
	got := make(map[string]bool, len(entries))
	for _, e := range entries {
		got[e.EventType] = true
	}
	for _, evt := range events {
		if !got[evt] {
			t.Errorf("event %q missing from VideoHistory result", evt)
		}
	}
}

func TestHistoryVideosDeduplication(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")

	if err := db.AddHistory("vid1", "playVideo", ""); err != nil {
		t.Fatalf("AddHistory 1: %v", err)
	}
	if err := db.AddHistory("vid1", "streamVideo", ""); err != nil {
		t.Fatalf("AddHistory 2: %v", err)
	}

	entries, err := db.HistoryVideos(10)
	if err != nil {
		t.Fatalf("HistoryVideos: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("HistoryVideos len = %d, want 1 (deduped by video)", len(entries))
	}
}

func TestDeleteVideoHistory(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")

	if err := db.AddHistory("vid1", "playVideo", ""); err != nil {
		t.Fatalf("AddHistory: %v", err)
	}
	if err := db.DeleteVideoHistory("vid1"); err != nil {
		t.Fatalf("DeleteVideoHistory: %v", err)
	}

	entries, err := db.HistoryVideos(10)
	if err != nil {
		t.Fatalf("HistoryVideos: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("HistoryVideos after delete: got %d, want 0", len(entries))
	}
}

func TestSearchQueries(t *testing.T) {
	db := newTestDB(t)

	queries := []string{"golang", "bubble tea", "sqlite"}
	for _, q := range queries {
		if err := db.AddHistory("", "search", q); err != nil {
			t.Fatalf("AddHistory search %q: %v", q, err)
		}
	}

	results, err := db.SearchQueries()
	if err != nil {
		t.Fatalf("SearchQueries: %v", err)
	}
	if len(results) != len(queries) {
		t.Fatalf("SearchQueries len = %d, want %d", len(results), len(queries))
	}

	found := make(map[string]bool, len(results))
	for _, r := range results {
		found[r] = true
	}
	for _, q := range queries {
		if !found[q] {
			t.Errorf("query %q missing from SearchQueries results", q)
		}
	}
}

func TestSearchQueriesDeduplication(t *testing.T) {
	db := newTestDB(t)

	for i := 0; i < 3; i++ {
		if err := db.AddHistory("", "search", "golang"); err != nil {
			t.Fatalf("AddHistory: %v", err)
		}
	}

	results, err := db.SearchQueries()
	if err != nil {
		t.Fatalf("SearchQueries: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("SearchQueries with duplicate entries: got %d, want 1", len(results))
	}
}

func TestClearHistory(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")

	if err := db.AddHistory("vid1", "playVideo", ""); err != nil {
		t.Fatalf("AddHistory: %v", err)
	}
	if err := db.AddHistory("", "search", "test"); err != nil {
		t.Fatalf("AddHistory search: %v", err)
	}
	if err := db.ClearHistory(); err != nil {
		t.Fatalf("ClearHistory: %v", err)
	}

	entries, err := db.HistoryVideos(10)
	if err != nil {
		t.Fatalf("HistoryVideos: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("HistoryVideos after ClearHistory: got %d, want 0", len(entries))
	}

	searches, err := db.SearchQueries()
	if err != nil {
		t.Fatalf("SearchQueries: %v", err)
	}
	if len(searches) != 0 {
		t.Errorf("SearchQueries after ClearHistory: got %d, want 0", len(searches))
	}
}

// ── Video position round-trips ────────────────────────────────────────────────

func TestVideoPosition(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")

	if err := db.SaveVideoPosition("vid1", 42000); err != nil {
		t.Fatalf("SaveVideoPosition: %v", err)
	}

	ms, ok := db.VideoPosition("vid1")
	if !ok {
		t.Fatal("VideoPosition: not found")
	}
	if ms != 42000 {
		t.Errorf("VideoPosition = %d, want 42000", ms)
	}
}

func TestVideoPositionUpdate(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")

	if err := db.SaveVideoPosition("vid1", 1000); err != nil {
		t.Fatalf("SaveVideoPosition initial: %v", err)
	}
	if err := db.SaveVideoPosition("vid1", 9999); err != nil {
		t.Fatalf("SaveVideoPosition update: %v", err)
	}

	ms, ok := db.VideoPosition("vid1")
	if !ok {
		t.Fatal("VideoPosition: not found")
	}
	if ms != 9999 {
		t.Errorf("VideoPosition after update = %d, want 9999", ms)
	}
}

func TestVideoPositionNotFound(t *testing.T) {
	db := newTestDB(t)

	_, ok := db.VideoPosition("nonexistent")
	if ok {
		t.Error("VideoPosition for unknown ID should return ok=false")
	}
}

func TestDeleteVideoPosition(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")

	if err := db.SaveVideoPosition("vid1", 5000); err != nil {
		t.Fatalf("SaveVideoPosition: %v", err)
	}
	if err := db.DeleteVideoPosition("vid1"); err != nil {
		t.Fatalf("DeleteVideoPosition: %v", err)
	}

	_, ok := db.VideoPosition("vid1")
	if ok {
		t.Error("VideoPosition after delete should return ok=false")
	}
}

func TestAllVideoPositions(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")
	upsertTestVideo(t, db, "vid2")

	if err := db.SaveVideoPosition("vid1", 1000); err != nil {
		t.Fatalf("SaveVideoPosition vid1: %v", err)
	}
	if err := db.SaveVideoPosition("vid2", 2000); err != nil {
		t.Fatalf("SaveVideoPosition vid2: %v", err)
	}

	positions, err := db.AllVideoPositions()
	if err != nil {
		t.Fatalf("AllVideoPositions: %v", err)
	}
	if positions["vid1"] != 1000 {
		t.Errorf("positions[vid1] = %d, want 1000", positions["vid1"])
	}
	if positions["vid2"] != 2000 {
		t.Errorf("positions[vid2] = %d, want 2000", positions["vid2"])
	}
}

// ── Playlist round-trips ──────────────────────────────────────────────────────

func TestCreateAndDeletePlaylist(t *testing.T) {
	db := newTestDB(t)

	id, err := db.CreatePlaylist("My Playlist")
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	if id == 0 {
		t.Fatal("CreatePlaylist returned id=0")
	}

	lists, err := db.Playlists()
	if err != nil {
		t.Fatalf("Playlists: %v", err)
	}
	if len(lists) != 1 || lists[0].Name != "My Playlist" {
		t.Errorf("Playlists after create: %+v", lists)
	}

	if err = db.DeletePlaylist(id); err != nil {
		t.Fatalf("DeletePlaylist: %v", err)
	}

	lists, err = db.Playlists()
	if err != nil {
		t.Fatalf("Playlists after delete: %v", err)
	}
	if len(lists) != 0 {
		t.Errorf("Playlists after delete: got %d, want 0", len(lists))
	}
}

func TestPlaylistAddRemoveVideo(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")

	plID, err := db.CreatePlaylist("Test Playlist")
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}

	if err = db.AddToPlaylist(plID, "vid1"); err != nil {
		t.Fatalf("AddToPlaylist: %v", err)
	}

	ids, err := db.PlaylistVideoIDs(plID)
	if err != nil {
		t.Fatalf("PlaylistVideoIDs: %v", err)
	}
	if len(ids) != 1 || ids[0] != "vid1" {
		t.Errorf("PlaylistVideoIDs = %v, want [vid1]", ids)
	}

	if err = db.RemoveFromPlaylist(plID, "vid1"); err != nil {
		t.Fatalf("RemoveFromPlaylist: %v", err)
	}

	ids, err = db.PlaylistVideoIDs(plID)
	if err != nil {
		t.Fatalf("PlaylistVideoIDs after remove: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("PlaylistVideoIDs after remove: got %v, want []", ids)
	}
}

func TestPlaylistCascadeDelete(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")

	plID, err := db.CreatePlaylist("Cascade Test")
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	if err = db.AddToPlaylist(plID, "vid1"); err != nil {
		t.Fatalf("AddToPlaylist: %v", err)
	}

	if err = db.DeletePlaylist(plID); err != nil {
		t.Fatalf("DeletePlaylist: %v", err)
	}

	ids, err := db.PlaylistVideoIDs(plID)
	if err != nil {
		t.Fatalf("PlaylistVideoIDs after cascade delete: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("playlist_videos not cascade-deleted: got %v", ids)
	}
}

func TestCreatePlaylistDuplicateName(t *testing.T) {
	db := newTestDB(t)

	id1, err := db.CreatePlaylist("Unique")
	if err != nil {
		t.Fatalf("CreatePlaylist first: %v", err)
	}
	if id1 == 0 {
		t.Fatal("CreatePlaylist first: got id=0")
	}

	// INSERT OR IGNORE on a duplicate name must not error and must not create a second row.
	_, err = db.CreatePlaylist("Unique")
	if err != nil {
		t.Fatalf("CreatePlaylist duplicate should not error: %v", err)
	}

	lists, err := db.Playlists()
	if err != nil {
		t.Fatalf("Playlists: %v", err)
	}
	if len(lists) != 1 {
		t.Errorf("Playlists after duplicate create: got %d, want 1", len(lists))
	}
}

// ── Local video round-trips ───────────────────────────────────────────────────

func TestAddAndDeleteLocalVideo(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")

	lv := domain.LocalVideo{
		ID:           "vid1",
		FilePath:     "/tmp/vid1.mp4",
		DownloadType: "video",
		DownloadedAt: time.Now(),
	}
	if err := db.AddLocalVideo(lv); err != nil {
		t.Fatalf("AddLocalVideo: %v", err)
	}

	got, ok := db.HasLocalVideo("vid1")
	if !ok {
		t.Fatal("HasLocalVideo: not found after add")
	}
	if got.FilePath != lv.FilePath {
		t.Errorf("FilePath = %q, want %q", got.FilePath, lv.FilePath)
	}

	if err := db.DeleteLocalVideo("vid1"); err != nil {
		t.Fatalf("DeleteLocalVideo: %v", err)
	}

	_, ok = db.HasLocalVideo("vid1")
	if ok {
		t.Error("HasLocalVideo should return false after delete")
	}
}

// ── Feed cache round-trips ────────────────────────────────────────────────────

func TestSaveAndGetFeedCache(t *testing.T) {
	db := newTestDB(t)

	videos := []domain.Video{
		{ID: "a", Title: "Alpha", Channel: "Chan", ChannelID: "ch1", Duration: 100, ViewCount: 500, UploadDate: "20240101"},
		{ID: "b", Title: "Beta", Channel: "Chan", ChannelID: "ch1", Duration: 200, ViewCount: 1000, UploadDate: "20240102"},
	}
	if err := db.SaveFeedCache("my-feed", videos); err != nil {
		t.Fatalf("SaveFeedCache: %v", err)
	}

	got, err := db.GetFeedCache("my-feed")
	if err != nil {
		t.Fatalf("GetFeedCache: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetFeedCache len = %d, want 2", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("GetFeedCache order: got [%s %s], want [a b]", got[0].ID, got[1].ID)
	}
}

func TestSaveFeedCacheReplaces(t *testing.T) {
	db := newTestDB(t)

	v1 := []domain.Video{{ID: "old", Title: "Old", Channel: "C", ChannelID: "ch1", Duration: 100, ViewCount: 100}}
	if err := db.SaveFeedCache("feed", v1); err != nil {
		t.Fatalf("SaveFeedCache first: %v", err)
	}

	v2 := []domain.Video{{ID: "new", Title: "New", Channel: "C", ChannelID: "ch1", Duration: 200, ViewCount: 200}}
	if err := db.SaveFeedCache("feed", v2); err != nil {
		t.Fatalf("SaveFeedCache second: %v", err)
	}

	got, err := db.GetFeedCache("feed")
	if err != nil {
		t.Fatalf("GetFeedCache: %v", err)
	}
	if len(got) != 1 || got[0].ID != "new" {
		t.Errorf("SaveFeedCache replace: got %+v, want single 'new'", got)
	}
}

// ── Watch later round-trips ───────────────────────────────────────────────────

func TestWatchLaterAddRemove(t *testing.T) {
	db := newTestDB(t)

	if err := db.AddWatchLater("vid1", "Some Title", "Some Channel", "https://example.com/vid1"); err != nil {
		t.Fatalf("AddWatchLater: %v", err)
	}

	entries, err := db.WatchLater()
	if err != nil {
		t.Fatalf("WatchLater: %v", err)
	}
	if len(entries) != 1 || entries[0].VideoID != "vid1" {
		t.Errorf("WatchLater after add: %+v", entries)
	}

	if err = db.RemoveWatchLater("vid1"); err != nil {
		t.Fatalf("RemoveWatchLater: %v", err)
	}

	entries, err = db.WatchLater()
	if err != nil {
		t.Fatalf("WatchLater after remove: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("WatchLater after remove: got %d, want 0", len(entries))
	}
}
