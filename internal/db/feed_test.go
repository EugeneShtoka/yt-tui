package db

import (
	"context"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// TestSaveFeedCacheUpdatesURL guards M-2: a copy of the video-upsert SQL used by
// SaveFeedCache omitted url=excluded.url, so a video's URL never refreshed via
// this path even though every other write path updated it.
func TestSaveFeedCacheUpdatesURL(t *testing.T) {
	db := newTestDB(t)

	v1 := []domain.Video{{ID: "v1", Title: "T", Channel: "C", ChannelID: "ch1", URL: "https://old.example.com/v1"}}
	if err := db.SaveFeedCache(context.Background(), "feed", v1); err != nil {
		t.Fatalf("SaveFeedCache first: %v", err)
	}
	v2 := []domain.Video{{ID: "v1", Title: "T", Channel: "C", ChannelID: "ch1", URL: "https://new.example.com/v1"}}
	if err := db.SaveFeedCache(context.Background(), "feed", v2); err != nil {
		t.Fatalf("SaveFeedCache second: %v", err)
	}

	got, err := db.GetFeedCache(context.Background(), "feed")
	if err != nil {
		t.Fatalf("GetFeedCache: %v", err)
	}
	if len(got) != 1 || got[0].URL != "https://new.example.com/v1" {
		t.Errorf("SaveFeedCache did not update URL on refresh: got %+v", got)
	}
}

// ── video_details_cache: nil vs. empty-slice semantics ───────────────────────

func TestVideoDetailsCacheNilVsEmptySemantics(t *testing.T) {
	db := newTestDB(t)

	if err := db.SaveVideoDetailsCache(context.Background(), "v1", "desc", "thumb", 42); err != nil {
		t.Fatalf("SaveVideoDetailsCache: %v", err)
	}

	// Never parsed -> nil.
	got, ok, err := db.GetVideoDetailsCache(context.Background(), "v1")
	if err != nil {
		t.Fatalf("GetVideoDetailsCache: %v", err)
	}
	if !ok {
		t.Fatal("expected cached details")
	}
	if got.Links != nil {
		t.Errorf("Links should be nil before SaveVideoLinks, got %v", got.Links)
	}

	// Parsed, found none -> non-nil empty slice, distinct from "never parsed".
	if saveErr := db.SaveVideoLinks(context.Background(), "v1", []domain.Link{}); saveErr != nil {
		t.Fatalf("SaveVideoLinks(empty): %v", saveErr)
	}
	got, _, err = db.GetVideoDetailsCache(context.Background(), "v1")
	if err != nil {
		t.Fatalf("GetVideoDetailsCache: %v", err)
	}
	if got.Links == nil {
		t.Fatal("Links should be non-nil after SaveVideoLinks([])")
	}
	if len(*got.Links) != 0 {
		t.Errorf("Links should be empty, got %v", *got.Links)
	}

	// Parsed with content.
	links := []domain.Link{{Label: "Site", URL: "https://example.com"}}
	if saveErr := db.SaveVideoLinks(context.Background(), "v1", links); saveErr != nil {
		t.Fatalf("SaveVideoLinks: %v", saveErr)
	}
	got, _, err = db.GetVideoDetailsCache(context.Background(), "v1")
	if err != nil {
		t.Fatalf("GetVideoDetailsCache: %v", err)
	}
	if got.Links == nil || len(*got.Links) != 1 || (*got.Links)[0].Label != "Site" {
		t.Errorf("Links round-trip failed: got %v", got.Links)
	}
}

// ── M-7: chapters/sb_segments/links upsert even when the row doesn't exist yet ─

func TestSaveVideoChaptersUpsertsMissingRow(t *testing.T) {
	db := newTestDB(t)

	chapters := []domain.Chapter{{Title: "Intro", OriginalStart: 0, OriginalEnd: 10}}
	if err := db.SaveVideoChapters(context.Background(), "v1", chapters); err != nil {
		t.Fatalf("SaveVideoChapters: %v", err)
	}

	got, ok, err := db.GetVideoDetailsCache(context.Background(), "v1")
	if err != nil {
		t.Fatalf("GetVideoDetailsCache: %v", err)
	}
	if !ok {
		t.Fatal("SaveVideoChapters on a missing row should create it (upsert)")
	}
	if got.Chapters == nil || len(*got.Chapters) != 1 || (*got.Chapters)[0].Title != "Intro" {
		t.Errorf("Chapters not persisted: %v", got.Chapters)
	}
}

func TestSaveVideoSBSegmentsUpsertsMissingRow(t *testing.T) {
	db := newTestDB(t)

	segs := []domain.SBSegment{{Start: 1, End: 2}}
	if err := db.SaveVideoSBSegments(context.Background(), "v1", segs); err != nil {
		t.Fatalf("SaveVideoSBSegments: %v", err)
	}

	got, ok, err := db.GetVideoDetailsCache(context.Background(), "v1")
	if err != nil {
		t.Fatalf("GetVideoDetailsCache: %v", err)
	}
	if !ok || got.SBSegments == nil || len(*got.SBSegments) != 1 {
		t.Fatalf("SBSegments not persisted via upsert: ok=%v got=%v", ok, got.SBSegments)
	}
}

func TestSaveVideoLinksUpsertsMissingRow(t *testing.T) {
	db := newTestDB(t)

	links := []domain.Link{{Label: "L", URL: "https://x"}}
	if err := db.SaveVideoLinks(context.Background(), "v1", links); err != nil {
		t.Fatalf("SaveVideoLinks: %v", err)
	}

	got, ok, err := db.GetVideoDetailsCache(context.Background(), "v1")
	if err != nil {
		t.Fatalf("GetVideoDetailsCache: %v", err)
	}
	if !ok || got.Links == nil || len(*got.Links) != 1 {
		t.Fatalf("Links not persisted via upsert: ok=%v got=%v", ok, got.Links)
	}
}

func TestSaveVideoChaptersPreservesOtherFields(t *testing.T) {
	db := newTestDB(t)

	if err := db.SaveVideoDetailsCache(context.Background(), "v1", "desc", "thumb", 42); err != nil {
		t.Fatalf("SaveVideoDetailsCache: %v", err)
	}
	if err := db.SaveVideoChapters(context.Background(), "v1", []domain.Chapter{{Title: "Intro"}}); err != nil {
		t.Fatalf("SaveVideoChapters: %v", err)
	}

	got, ok, err := db.GetVideoDetailsCache(context.Background(), "v1")
	if err != nil {
		t.Fatalf("GetVideoDetailsCache: %v", err)
	}
	if !ok {
		t.Fatal("expected row")
	}
	if got.Description != "desc" || got.ThumbnailURL != "thumb" || got.Subscribers != 42 {
		t.Errorf("SaveVideoChapters clobbered other fields: %+v", got)
	}
	if got.Chapters == nil || len(*got.Chapters) != 1 {
		t.Errorf("Chapters not saved: %v", got.Chapters)
	}
}

// ── HideRecVideo ──────────────────────────────────────────────────────────────

func TestHideRecVideo(t *testing.T) {
	db := newTestDB(t)

	if err := db.SaveVideoDetailsCache(context.Background(), "v1", "d", "t", 1); err != nil {
		t.Fatalf("SaveVideoDetailsCache: %v", err)
	}
	if err := db.HideRecVideo(context.Background(), "v1"); err != nil {
		t.Fatalf("HideRecVideo: %v", err)
	}

	hidden, err := db.HiddenRecVideoIDs(context.Background())
	if err != nil {
		t.Fatalf("HiddenRecVideoIDs: %v", err)
	}
	if !hidden["v1"] {
		t.Error("HideRecVideo: v1 not marked hidden")
	}

	_, ok, err := db.GetVideoDetailsCache(context.Background(), "v1")
	if err != nil {
		t.Fatalf("GetVideoDetailsCache: %v", err)
	}
	if ok {
		t.Error("HideRecVideo should delete the cached details")
	}
}

// ── pruneRecommendedFeed (M-6: feed scope + tx + orphan sweep) ───────────────

// TestPruneRecommendedFeedScopedToRecommendedFeed guards M-6: the details-cache
// delete subquery had no fc.feed='recommended' filter, so an aged video cached
// under a *different* feed (e.g. subscriptions) had its details wiped too.
func TestPruneRecommendedFeedScopedToRecommendedFeed(t *testing.T) {
	db := newTestDB(t)
	const old = "20200101"
	ctx := context.Background()

	if err := db.UpsertVideo(context.Background(), "videoA", "A", "C", "ch1", 100, 500, old, "https://a"); err != nil {
		t.Fatalf("UpsertVideo videoA: %v", err)
	}
	if err := db.UpsertVideo(context.Background(), "videoB", "B", "C", "ch1", 100, 500, old, "https://b"); err != nil {
		t.Fatalf("UpsertVideo videoB: %v", err)
	}
	if err := db.SaveVideoDetailsCache(context.Background(), "videoA", "descA", "thumbA", 1); err != nil {
		t.Fatalf("SaveVideoDetailsCache videoA: %v", err)
	}
	if err := db.SaveVideoDetailsCache(context.Background(), "videoB", "descB", "thumbB", 1); err != nil {
		t.Fatalf("SaveVideoDetailsCache videoB: %v", err)
	}

	// videoA belongs to a different feed; videoB belongs to "recommended".
	if _, err := db.sql.ExecContext(ctx,
		`INSERT INTO feed_cache (feed, video_id, position) VALUES ('subscriptions', 'videoA', 0)`); err != nil {
		t.Fatalf("seed subscriptions feed_cache: %v", err)
	}
	if _, err := db.sql.ExecContext(ctx,
		`INSERT INTO feed_cache (feed, video_id, position) VALUES ('recommended', 'videoB', 0)`); err != nil {
		t.Fatalf("seed recommended feed_cache: %v", err)
	}

	if err := db.pruneRecommendedFeed(1); err != nil {
		t.Fatalf("pruneRecommendedFeed: %v", err)
	}

	if _, ok, err := db.GetVideoDetailsCache(context.Background(), "videoA"); err != nil || !ok {
		t.Errorf("videoA (cached under a different feed) had its details wrongly evicted: ok=%v err=%v", ok, err)
	}
	if _, ok, err := db.GetVideoDetailsCache(context.Background(), "videoB"); err != nil || ok {
		t.Errorf("videoB (aged out of recommended) should have had its details evicted: ok=%v err=%v", ok, err)
	}
}

// TestPruneRecommendedFeedOrphanSweep guards M-6: nothing previously pruned the
// videos table itself, so aged, fully-unreferenced rows accumulated forever.
func TestPruneRecommendedFeedOrphanSweep(t *testing.T) {
	db := newTestDB(t)
	const old = "20200101"
	ctx := context.Background()

	// orphan: aged, cached only in recommended, referenced nowhere else.
	if err := db.UpsertVideo(context.Background(), "orphan", "O", "C", "ch1", 100, 500, old, "https://o"); err != nil {
		t.Fatalf("UpsertVideo orphan: %v", err)
	}
	if err := db.SaveFeedCache(context.Background(), "recommended", []domain.Video{
		{ID: "orphan", Title: "O", Channel: "C", ChannelID: "ch1", UploadDate: old, URL: "https://o"},
	}); err != nil {
		t.Fatalf("SaveFeedCache: %v", err)
	}

	// kept: aged, but still referenced via channel_videos -> must survive.
	if err := db.SaveChannelVideos(context.Background(), "ch1", []domain.Video{{ID: "kept", Title: "K", ChannelID: "ch1", UploadDate: old}}); err != nil {
		t.Fatalf("SaveChannelVideos: %v", err)
	}
	if _, err := db.sql.ExecContext(ctx,
		`INSERT INTO feed_cache (feed, video_id, position) VALUES ('recommended', 'kept', 1)`); err != nil {
		t.Fatalf("seed recommended feed_cache: %v", err)
	}

	if err := db.pruneRecommendedFeed(1); err != nil {
		t.Fatalf("pruneRecommendedFeed: %v", err)
	}

	var count int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM videos WHERE id='orphan'`).Scan(&count); err != nil {
		t.Fatalf("count orphan: %v", err)
	}
	if count != 0 {
		t.Error("orphan sweep: fully unreferenced aged video row should have been deleted")
	}

	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM videos WHERE id='kept'`).Scan(&count); err != nil {
		t.Fatalf("count kept: %v", err)
	}
	if count != 1 {
		t.Error("orphan sweep: video still referenced by channel_videos should survive")
	}
}
