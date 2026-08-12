package db

import (
	"context"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

func TestVideoDuration(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	upsertTestVideo(t, db, "v1") // upsertTestVideo uses duration 300

	if got, err := db.VideoDuration(ctx, "v1"); err != nil || got != 300 {
		t.Fatalf("VideoDuration(v1) = %d (err %v), want 300", got, err)
	}
	// Absent row is (0, nil), not an error — callers treat 0 as "unknown".
	if got, err := db.VideoDuration(ctx, "missing"); err != nil || got != 0 {
		t.Fatalf("VideoDuration(missing) = %d (err %v), want 0", got, err)
	}
}

func TestInWatchLater_LocalStore(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	upsertTestVideo(t, db, "v1")
	upsertTestVideo(t, db, "v2")

	id, err := db.CreatePlaylist(ctx, domain.WatchLaterPlaylistName)
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	if err = db.AddToPlaylist(ctx, id, "v1"); err != nil {
		t.Fatalf("AddToPlaylist: %v", err)
	}

	in, err := db.InWatchLater(ctx, "v1", domain.WatchLaterYTID, domain.WatchLaterPlaylistName)
	if err != nil || !in {
		t.Fatalf("InWatchLater(v1) = %v (err %v), want true", in, err)
	}
	in, err = db.InWatchLater(ctx, "v2", domain.WatchLaterYTID, domain.WatchLaterPlaylistName)
	if err != nil || in {
		t.Fatalf("InWatchLater(v2) = %v (err %v), want false", in, err)
	}
}

func TestInWatchLater_YTStore(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// SaveYTPlaylistVideos upserts the video rows and links them under "WL".
	if err := db.SaveYTPlaylistVideos(ctx, domain.WatchLaterYTID, []domain.Video{{ID: "v1", Title: "V1", URL: "u"}}); err != nil {
		t.Fatalf("SaveYTPlaylistVideos: %v", err)
	}
	in, err := db.InWatchLater(ctx, "v1", domain.WatchLaterYTID, domain.WatchLaterPlaylistName)
	if err != nil || !in {
		t.Fatalf("InWatchLater(v1) = %v (err %v), want true", in, err)
	}
}

func TestRemoveYTPlaylistVideo(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	if err := db.SaveYTPlaylistVideos(ctx, domain.WatchLaterYTID, []domain.Video{
		{ID: "v1", Title: "V1", URL: "u1"},
		{ID: "v2", Title: "V2", URL: "u2"},
	}); err != nil {
		t.Fatalf("SaveYTPlaylistVideos: %v", err)
	}

	if err := db.RemoveYTPlaylistVideo(ctx, domain.WatchLaterYTID, "v1"); err != nil {
		t.Fatalf("RemoveYTPlaylistVideo: %v", err)
	}

	got, err := db.GetYTPlaylistVideos(ctx, domain.WatchLaterYTID)
	if err != nil {
		t.Fatalf("GetYTPlaylistVideos: %v", err)
	}
	if len(got) != 1 || got[0].ID != "v2" {
		t.Fatalf("after remove = %v, want only [v2]", got)
	}
	// Removing an absent video is a harmless no-op.
	if err := db.RemoveYTPlaylistVideo(ctx, domain.WatchLaterYTID, "nope"); err != nil {
		t.Fatalf("RemoveYTPlaylistVideo(absent): %v", err)
	}
}
