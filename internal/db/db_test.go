package db

import (
	"context"
	"strings"
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
	if err := db.UpsertVideo(context.Background(), id, "Test Title", "Test Channel", "ch-"+id, 300, 1000, "20240101", "https://example.com/"+id); err != nil {
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

// Reopening with stripEmojis=true must scrub emojis from channel names too —
// both the denormalized videos.channel and subscribed_channels.name — not just
// video/playlist titles.
func TestStripEmojisChannelNames(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	today := time.Now().Format("20060102") // recent so the age prune keeps it

	db, err := New(dir, false, 90)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err = db.UpsertVideo(ctx, "v1", "Hello 😀 World", "Cool Chan 😎", "ch1", 100, 10, today, "u"); err != nil {
		t.Fatalf("UpsertVideo: %v", err)
	}
	if err = db.AddSubscribedChannel(ctx, domain.Channel{ID: "ch1", Name: "Cool Chan 😎", State: domain.SubYT}); err != nil {
		t.Fatalf("AddSubscribedChannel: %v", err)
	}
	if err = db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db2, err := New(dir, true, 90) // stripEmojis=true → cleanEmojiTitles runs
	if err != nil {
		t.Fatalf("reopen with strip: %v", err)
	}
	t.Cleanup(func() { _ = db2.Close() })

	var title, ch, name string
	if err := db2.sql.QueryRowContext(ctx, `SELECT title, COALESCE(channel,'') FROM videos WHERE id='v1'`).Scan(&title, &ch); err != nil {
		t.Fatalf("read video: %v", err)
	}
	if err := db2.sql.QueryRowContext(ctx, `SELECT name FROM subscribed_channels WHERE channel_id='ch1'`).Scan(&name); err != nil {
		t.Fatalf("read channel: %v", err)
	}
	for label, got := range map[string]string{"video.title": title, "video.channel": ch, "channel.name": name} {
		if strings.ContainsRune(got, '😀') || strings.ContainsRune(got, '😎') {
			t.Errorf("%s still contains an emoji: %q", label, got)
		}
		if got == "" {
			t.Errorf("%s was blanked by stripping", label)
		}
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

// ── UpsertVideo round-trip ────────────────────────────────────────────────────

func TestUpsertVideoRoundTrip(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")

	videos, err := db.GetFeedCache(context.Background(), "nosuchfeed")
	if err != nil {
		t.Fatalf("GetFeedCache: %v", err)
	}
	if len(videos) != 0 {
		t.Errorf("expected empty feed cache, got %d", len(videos))
	}

	err = db.SaveFeedCache(context.Background(), "test-feed", []domain.Video{
		{ID: "vid1", Title: "Test Title", Channel: "Test Channel", ChannelID: "ch-vid1", Duration: 300, ViewCount: 1000, UploadDate: "20240101"},
	})
	if err != nil {
		t.Fatalf("SaveFeedCache: %v", err)
	}

	got, err := db.GetFeedCache(context.Background(), "test-feed")
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

	if err := db.AddHistory(context.Background(), "vid1", "playVideo", ""); err != nil {
		t.Fatalf("AddHistory: %v", err)
	}

	entries, err := db.HistoryVideos(context.Background(), 10)
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
		if err := db.AddHistory(context.Background(), "vid1", evt, ""); err != nil {
			t.Fatalf("AddHistory(%q): %v", evt, err)
		}
	}

	entries, err := db.VideoHistory(context.Background(), "vid1")
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

	if err := db.AddHistory(context.Background(), "vid1", "playVideo", ""); err != nil {
		t.Fatalf("AddHistory 1: %v", err)
	}
	if err := db.AddHistory(context.Background(), "vid1", "streamVideo", ""); err != nil {
		t.Fatalf("AddHistory 2: %v", err)
	}

	entries, err := db.HistoryVideos(context.Background(), 10)
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

	if err := db.AddHistory(context.Background(), "vid1", "playVideo", ""); err != nil {
		t.Fatalf("AddHistory: %v", err)
	}
	if err := db.DeleteVideoHistory(context.Background(), "vid1"); err != nil {
		t.Fatalf("DeleteVideoHistory: %v", err)
	}

	entries, err := db.HistoryVideos(context.Background(), 10)
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
		if err := db.AddHistory(context.Background(), "", "search", q); err != nil {
			t.Fatalf("AddHistory search %q: %v", q, err)
		}
	}

	results, err := db.SearchQueries(context.Background())
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
		if err := db.AddHistory(context.Background(), "", "search", "golang"); err != nil {
			t.Fatalf("AddHistory: %v", err)
		}
	}

	results, err := db.SearchQueries(context.Background())
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

	if err := db.AddHistory(context.Background(), "vid1", "playVideo", ""); err != nil {
		t.Fatalf("AddHistory: %v", err)
	}
	if err := db.AddHistory(context.Background(), "", "search", "test"); err != nil {
		t.Fatalf("AddHistory search: %v", err)
	}
	if err := db.ClearHistory(context.Background()); err != nil {
		t.Fatalf("ClearHistory: %v", err)
	}

	entries, err := db.HistoryVideos(context.Background(), 10)
	if err != nil {
		t.Fatalf("HistoryVideos: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("HistoryVideos after ClearHistory: got %d, want 0", len(entries))
	}

	searches, err := db.SearchQueries(context.Background())
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

	if err := db.SaveVideoPosition(context.Background(), "vid1", 42000); err != nil {
		t.Fatalf("SaveVideoPosition: %v", err)
	}

	ms, ok, err := db.VideoPosition(context.Background(), "vid1")
	if err != nil {
		t.Fatalf("VideoPosition: %v", err)
	}
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

	if err := db.SaveVideoPosition(context.Background(), "vid1", 1000); err != nil {
		t.Fatalf("SaveVideoPosition initial: %v", err)
	}
	if err := db.SaveVideoPosition(context.Background(), "vid1", 9999); err != nil {
		t.Fatalf("SaveVideoPosition update: %v", err)
	}

	ms, ok, err := db.VideoPosition(context.Background(), "vid1")
	if err != nil {
		t.Fatalf("VideoPosition: %v", err)
	}
	if !ok {
		t.Fatal("VideoPosition: not found")
	}
	if ms != 9999 {
		t.Errorf("VideoPosition after update = %d, want 9999", ms)
	}
}

func TestVideoPositionNotFound(t *testing.T) {
	db := newTestDB(t)

	_, ok, err := db.VideoPosition(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("VideoPosition: %v", err)
	}
	if ok {
		t.Error("VideoPosition for unknown ID should return ok=false")
	}
}

func TestDeleteVideoPosition(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")

	if err := db.SaveVideoPosition(context.Background(), "vid1", 5000); err != nil {
		t.Fatalf("SaveVideoPosition: %v", err)
	}
	if err := db.DeleteVideoPosition(context.Background(), "vid1"); err != nil {
		t.Fatalf("DeleteVideoPosition: %v", err)
	}

	_, ok, err := db.VideoPosition(context.Background(), "vid1")
	if err != nil {
		t.Fatalf("VideoPosition: %v", err)
	}
	if ok {
		t.Error("VideoPosition after delete should return ok=false")
	}
}

func TestAllVideoPositions(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")
	upsertTestVideo(t, db, "vid2")

	if err := db.SaveVideoPosition(context.Background(), "vid1", 1000); err != nil {
		t.Fatalf("SaveVideoPosition vid1: %v", err)
	}
	if err := db.SaveVideoPosition(context.Background(), "vid2", 2000); err != nil {
		t.Fatalf("SaveVideoPosition vid2: %v", err)
	}

	positions, err := db.AllVideoPositions(context.Background())
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

	id, err := db.CreatePlaylist(context.Background(), "My Playlist")
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	if id == "" {
		t.Fatal("CreatePlaylist returned id=0")
	}

	lists, err := db.Playlists(context.Background())
	if err != nil {
		t.Fatalf("Playlists: %v", err)
	}
	if len(lists) != 1 || lists[0].Name != "My Playlist" {
		t.Errorf("Playlists after create: %+v", lists)
	}

	if err = db.DeletePlaylist(context.Background(), id); err != nil {
		t.Fatalf("DeletePlaylist: %v", err)
	}

	lists, err = db.Playlists(context.Background())
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

	plID, err := db.CreatePlaylist(context.Background(), "Test Playlist")
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}

	if err = db.AddToPlaylist(context.Background(), plID, "vid1"); err != nil {
		t.Fatalf("AddToPlaylist: %v", err)
	}

	ids, err := db.PlaylistVideoIDs(context.Background(), plID)
	if err != nil {
		t.Fatalf("PlaylistVideoIDs: %v", err)
	}
	if len(ids) != 1 || ids[0] != "vid1" {
		t.Errorf("PlaylistVideoIDs = %v, want [vid1]", ids)
	}

	if err = db.RemoveFromPlaylist(context.Background(), plID, "vid1"); err != nil {
		t.Fatalf("RemoveFromPlaylist: %v", err)
	}

	ids, err = db.PlaylistVideoIDs(context.Background(), plID)
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

	plID, err := db.CreatePlaylist(context.Background(), "Cascade Test")
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	if err = db.AddToPlaylist(context.Background(), plID, "vid1"); err != nil {
		t.Fatalf("AddToPlaylist: %v", err)
	}

	if err = db.DeletePlaylist(context.Background(), plID); err != nil {
		t.Fatalf("DeletePlaylist: %v", err)
	}

	ids, err := db.PlaylistVideoIDs(context.Background(), plID)
	if err != nil {
		t.Fatalf("PlaylistVideoIDs after cascade delete: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("collection_videos not deleted with playlist: got %v", ids)
	}
}

func TestCreatePlaylistDuplicateName(t *testing.T) {
	db := newTestDB(t)

	id1, err := db.CreatePlaylist(context.Background(), "Unique")
	if err != nil {
		t.Fatalf("CreatePlaylist first: %v", err)
	}
	if id1 == "" {
		t.Fatal("CreatePlaylist first: got id=0")
	}

	// Creating a duplicate name must not error, must not create a second row,
	// and must return the existing playlist's id (H-4: a prior implementation
	// returned SQLite's connection-global last-insert-rowid, which is left
	// unchanged by an ignored insert and can point at an unrelated playlist).
	id2, err := db.CreatePlaylist(context.Background(), "Unique")
	if err != nil {
		t.Fatalf("CreatePlaylist duplicate should not error: %v", err)
	}
	if id2 != id1 {
		t.Errorf("CreatePlaylist duplicate: got id=%q, want %q (the existing playlist)", id2, id1)
	}

	lists, err := db.Playlists(context.Background())
	if err != nil {
		t.Fatalf("Playlists: %v", err)
	}
	if len(lists) != 1 {
		t.Errorf("Playlists after duplicate create: got %d, want 1", len(lists))
	}
}

// TestCreatePlaylistDuplicateNameAfterOtherInserts reproduces the exact H-4
// scenario: after other playlists have advanced the connection's
// last-insert-rowid past the target row, re-creating an existing name must
// still return that row's own id, not whatever was inserted most recently.
func TestCreatePlaylistDuplicateNameAfterOtherInserts(t *testing.T) {
	db := newTestDB(t)

	idA, err := db.CreatePlaylist(context.Background(), "A")
	if err != nil {
		t.Fatalf("CreatePlaylist A: %v", err)
	}
	idB, err := db.CreatePlaylist(context.Background(), "B")
	if err != nil {
		t.Fatalf("CreatePlaylist B: %v", err)
	}
	if idA == idB {
		t.Fatalf("CreatePlaylist A and B got the same id %q", idA)
	}

	gotA, err := db.CreatePlaylist(context.Background(), "A")
	if err != nil {
		t.Fatalf("CreatePlaylist A duplicate: %v", err)
	}
	if gotA != idA {
		t.Errorf("CreatePlaylist A duplicate: got id=%q, want %q (A's own id, not B's %q)", gotA, idA, idB)
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
	if err := db.AddLocalVideo(context.Background(), lv); err != nil {
		t.Fatalf("AddLocalVideo: %v", err)
	}

	got, ok, err := db.HasLocalVideo(context.Background(), "vid1")
	if err != nil {
		t.Fatalf("HasLocalVideo: %v", err)
	}
	if !ok {
		t.Fatal("HasLocalVideo: not found after add")
	}
	if got.FilePath != lv.FilePath {
		t.Errorf("FilePath = %q, want %q", got.FilePath, lv.FilePath)
	}

	if err = db.DeleteLocalVideo(context.Background(), "vid1"); err != nil {
		t.Fatalf("DeleteLocalVideo: %v", err)
	}

	_, ok, err = db.HasLocalVideo(context.Background(), "vid1")
	if err != nil {
		t.Fatalf("HasLocalVideo: %v", err)
	}
	if ok {
		t.Error("HasLocalVideo should return false after delete")
	}
}

// TestLocalVideoResumePositionFromPositions guards Target 3 of the schema
// consolidation: LastPositionMs no longer comes from the dead
// local_videos.last_position_ms column but is LEFT JOINed from video_positions,
// the sole table SaveVideoPosition (and UpdateLastPosition) writes to. Both
// HasLocalVideo and LocalVideos must surface the saved position.
func TestLocalVideoResumePositionFromPositions(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")

	if err := db.AddLocalVideo(ctx, domain.LocalVideo{
		ID:           "vid1",
		FilePath:     "/tmp/vid1.mp4",
		DownloadType: "video",
		DownloadedAt: time.Now(),
	}); err != nil {
		t.Fatalf("AddLocalVideo: %v", err)
	}

	// Freshly downloaded, no position saved yet → 0.
	got, ok, err := db.HasLocalVideo(ctx, "vid1")
	if err != nil || !ok {
		t.Fatalf("HasLocalVideo: ok=%v err=%v", ok, err)
	}
	if got.LastPositionMs != 0 {
		t.Errorf("LastPositionMs before save = %d, want 0", got.LastPositionMs)
	}

	if err = db.UpdateLastPosition(ctx, "vid1", 42000); err != nil {
		t.Fatalf("UpdateLastPosition: %v", err)
	}

	got, _, err = db.HasLocalVideo(ctx, "vid1")
	if err != nil {
		t.Fatalf("HasLocalVideo after save: %v", err)
	}
	if got.LastPositionMs != 42000 {
		t.Errorf("HasLocalVideo LastPositionMs = %d, want 42000", got.LastPositionMs)
	}

	list, err := db.LocalVideos(ctx)
	if err != nil {
		t.Fatalf("LocalVideos: %v", err)
	}
	if len(list) != 1 || list[0].LastPositionMs != 42000 {
		t.Errorf("LocalVideos LastPositionMs = %v, want [42000]", list)
	}
}

// TestSetVideoStatusLastPlayed guards H-5: LastPlayed was scanned via a
// hardcoded string layout that never matched what the driver actually wrote,
// so it silently stayed the zero time forever. LocalVideos is the only
// reader, so the round-trip must go through it.
func TestSetVideoStatusLastPlayed(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "vid1")

	lv := domain.LocalVideo{
		ID:           "vid1",
		FilePath:     "/tmp/vid1.mp4",
		DownloadType: "video",
		DownloadedAt: time.Now(),
	}
	if err := db.AddLocalVideo(context.Background(), lv); err != nil {
		t.Fatalf("AddLocalVideo: %v", err)
	}

	all, err := db.LocalVideos(context.Background())
	if err != nil {
		t.Fatalf("LocalVideos before play: %v", err)
	}
	if len(all) != 1 || !all[0].LastPlayed.IsZero() {
		t.Fatalf("LastPlayed before any play should be zero, got %v", all[0].LastPlayed)
	}

	before := time.Now()
	if err = db.SetVideoStatus(context.Background(), "vid1", domain.StatusStarted); err != nil {
		t.Fatalf("SetVideoStatus: %v", err)
	}
	after := time.Now()

	all, err = db.LocalVideos(context.Background())
	if err != nil {
		t.Fatalf("LocalVideos after play: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("LocalVideos after play: got %d rows, want 1", len(all))
	}
	got := all[0].LastPlayed
	if got.IsZero() {
		t.Fatal("LastPlayed is still zero after SetVideoStatus")
	}
	if got.Before(before.Add(-time.Second)) || got.After(after.Add(time.Second)) {
		t.Errorf("LastPlayed = %v, want between %v and %v", got, before, after)
	}
}

// ── Feed cache round-trips ────────────────────────────────────────────────────

func TestSaveAndGetFeedCache(t *testing.T) {
	db := newTestDB(t)

	videos := []domain.Video{
		{ID: "a", Title: "Alpha", Channel: "Chan", ChannelID: "ch1", Duration: 100, ViewCount: 500, UploadDate: "20240101"},
		{ID: "b", Title: "Beta", Channel: "Chan", ChannelID: "ch1", Duration: 200, ViewCount: 1000, UploadDate: "20240102"},
	}
	if err := db.SaveFeedCache(context.Background(), "my-feed", videos); err != nil {
		t.Fatalf("SaveFeedCache: %v", err)
	}

	got, err := db.GetFeedCache(context.Background(), "my-feed")
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
	if err := db.SaveFeedCache(context.Background(), "feed", v1); err != nil {
		t.Fatalf("SaveFeedCache first: %v", err)
	}

	v2 := []domain.Video{{ID: "new", Title: "New", Channel: "C", ChannelID: "ch1", Duration: 200, ViewCount: 200}}
	if err := db.SaveFeedCache(context.Background(), "feed", v2); err != nil {
		t.Fatalf("SaveFeedCache second: %v", err)
	}

	got, err := db.GetFeedCache(context.Background(), "feed")
	if err != nil {
		t.Fatalf("GetFeedCache: %v", err)
	}
	if len(got) != 1 || got[0].ID != "new" {
		t.Errorf("SaveFeedCache replace: got %+v, want single 'new'", got)
	}
}
