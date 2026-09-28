package db

import (
	"context"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// The recommended feed carries no view counts, so its videos are stored with
// view_count=0 — the same value the startup prune reads as "members-only". The
// prune must only remove zero-view videos nobody references; before, it emptied
// the recommended cache and erased hides, playlist entries and History rows
// (via ON DELETE CASCADE) on every start.
func TestDeleteMemberVideosKeepsReferencedVideos(t *testing.T) {
	ctx := context.Background()
	d := newTestDB(t)
	zero := func(id string) {
		t.Helper()
		if err := d.UpsertVideo(ctx, id, "T "+id, "C", "UC1", 600, 0, "20260901", ""); err != nil {
			t.Fatalf("UpsertVideo(%q): %v", id, err)
		}
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.sql.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	zero("member") // only in a channel crawl: the one prune target
	exec(`INSERT INTO channel_videos (channel_id, video_id) VALUES ('UC1', 'member')`)

	if err := d.SaveFeedCache(ctx, "recommended", []domain.Video{{ID: "rec", Title: "T rec", ChannelID: "UC1"}}); err != nil {
		t.Fatalf("SaveFeedCache: %v", err)
	}
	zero("hidden")
	if err := d.HideRecVideo(ctx, "hidden"); err != nil {
		t.Fatalf("HideRecVideo: %v", err)
	}
	zero("listed")
	exec(`INSERT INTO collection_videos (collection_id, video_id) VALUES ('pl1', 'listed')`)
	zero("watched")
	if err := d.AddHistory(ctx, "watched", "streamVideo", ""); err != nil {
		t.Fatalf("AddHistory: %v", err)
	}

	if err := d.deleteMemberVideos(); err != nil {
		t.Fatalf("deleteMemberVideos: %v", err)
	}

	exists := func(q, id string) bool {
		t.Helper()
		var n int
		if err := d.sql.QueryRowContext(ctx, q, id).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n > 0
	}
	const inVideos = `SELECT count(*) FROM videos WHERE id=?`
	if exists(inVideos, "member") {
		t.Error("unreferenced zero-view video survived the prune")
	}
	for _, id := range []string{"rec", "hidden", "listed", "watched"} {
		if !exists(inVideos, id) {
			t.Errorf("%s: pruned, want kept", id)
		}
	}
	if !exists(`SELECT count(*) FROM feed_cache WHERE feed='recommended' AND video_id=?`, "rec") {
		t.Error("recommended cache row pruned")
	}
	if !exists(`SELECT count(*) FROM hidden_rec_videos WHERE video_id=?`, "hidden") {
		t.Error("hide pruned")
	}
	if !exists(`SELECT count(*) FROM collection_videos WHERE video_id=?`, "listed") {
		t.Error("playlist entry pruned")
	}
	if !exists(`SELECT count(*) FROM history WHERE video_id=?`, "watched") {
		t.Error("History row pruned")
	}
}

// A zero (unknown) count must not overwrite one already recorded: the
// recommended feed re-saves videos a channel crawl or search saw with a count.
func TestUpsertKeepsKnownViewCount(t *testing.T) {
	ctx := context.Background()
	d := newTestDB(t)
	if err := d.UpsertVideo(ctx, "v", "T", "C", "UC1", 600, 5000, "20260901", ""); err != nil {
		t.Fatalf("UpsertVideo: %v", err)
	}
	if err := d.UpsertVideo(ctx, "v", "T2", "C", "UC1", 600, 0, "20260901", ""); err != nil {
		t.Fatalf("UpsertVideo (unknown count): %v", err)
	}
	var views int64
	var title string
	if err := d.sql.QueryRowContext(ctx, `SELECT view_count, title FROM videos WHERE id='v'`).Scan(&views, &title); err != nil {
		t.Fatalf("select: %v", err)
	}
	if views != 5000 {
		t.Errorf("view_count = %d, want the known 5000 kept", views)
	}
	if title != "T2" {
		t.Errorf("title = %q, want the other columns still updated", title)
	}

	if err := d.UpsertVideo(ctx, "v", "T2", "C", "UC1", 600, 7000, "20260901", ""); err != nil {
		t.Fatalf("UpsertVideo (new count): %v", err)
	}
	if err := d.sql.QueryRowContext(ctx, `SELECT view_count FROM videos WHERE id='v'`).Scan(&views); err != nil {
		t.Fatalf("select: %v", err)
	}
	if views != 7000 {
		t.Errorf("view_count = %d, want a newer known count to replace it", views)
	}
}
