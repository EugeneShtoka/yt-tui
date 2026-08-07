package db

import (
	"context"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// vid builds a video with an explicit id + upload_date so ordering is testable.
func vid(id, date string) domain.Video {
	return domain.Video{ID: id, Title: "T " + id, Channel: "C", ChannelID: "", Duration: 60, ViewCount: 1, UploadDate: date, URL: "https://youtu.be/" + id}
}

func TestUpdateVideoUploadDate(t *testing.T) {
	d := newTestDB(t)
	upsertTestVideo(t, d, "v1")
	if err := d.UpdateVideoUploadDate(context.Background(), "v1", "20250214"); err != nil {
		t.Fatalf("UpdateVideoUploadDate: %v", err)
	}
	// Non-existent id is a no-op, not an error.
	if err := d.UpdateVideoUploadDate(context.Background(), "nope", "20250214"); err != nil {
		t.Fatalf("UpdateVideoUploadDate(missing): %v", err)
	}
	var got string
	if err := d.sql.QueryRowContext(context.Background(), `SELECT upload_date FROM videos WHERE id='v1'`).Scan(&got); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got != "20250214" {
		t.Fatalf("upload_date=%q, want 20250214", got)
	}
}

// seedSubscribed subscribes chID and attaches videos to it.
func seedSubscribed(t *testing.T, d *DB, chID string, videos []domain.Video) {
	t.Helper()
	if err := d.AddSubscribedChannel(context.Background(), domain.Channel{ID: chID, Name: chID, State: domain.SubYT}); err != nil {
		t.Fatalf("AddSubscribedChannel: %v", err)
	}
	if err := d.SaveChannelVideos(context.Background(), chID, videos); err != nil {
		t.Fatalf("SaveChannelVideos: %v", err)
	}
}

func TestThumbnailEligibilityNewestNPerChannel(t *testing.T) {
	d := newTestDB(t)
	// 3 videos in one subscribed channel, distinct dates.
	seedSubscribed(t, d, "UCsub", []domain.Video{
		vid("new1", "20250103"),
		vid("new2", "20250102"),
		vid("old1", "20250101"),
	})

	// perChannel=2 → the two newest are eligible, the oldest is not.
	ids, err := d.ThumbnailEligibleIDs(context.Background(), 2)
	if err != nil {
		t.Fatalf("ThumbnailEligibleIDs: %v", err)
	}
	if !ids["new1"] || !ids["new2"] {
		t.Fatalf("newest two should be eligible: %v", ids)
	}
	if ids["old1"] {
		t.Fatalf("oldest should not be eligible: %v", ids)
	}

	// The single-video predicate must agree with the set.
	for id, want := range map[string]bool{"new1": true, "new2": true, "old1": false} {
		got, err := d.ThumbnailEligible(context.Background(), id, 2)
		if err != nil {
			t.Fatalf("ThumbnailEligible(%s): %v", id, err)
		}
		if got != want {
			t.Fatalf("ThumbnailEligible(%s)=%v, want %v", id, got, want)
		}
	}
}

func TestThumbnailEligibilityIncludesRecommended(t *testing.T) {
	d := newTestDB(t)
	// A recommended video that belongs to no subscribed channel is still eligible.
	if err := d.SaveFeedCache(context.Background(), "recommended", []domain.Video{vid("rec1", "20240101")}); err != nil {
		t.Fatalf("SaveFeedCache: %v", err)
	}
	ok, err := d.ThumbnailEligible(context.Background(), "rec1", 30)
	if err != nil {
		t.Fatalf("ThumbnailEligible: %v", err)
	}
	if !ok {
		t.Fatal("recommended video should be thumbnail-eligible")
	}
	ids, err := d.ThumbnailEligibleIDs(context.Background(), 30)
	if err != nil {
		t.Fatalf("ThumbnailEligibleIDs: %v", err)
	}
	if !ids["rec1"] {
		t.Fatalf("recommended video missing from eligible set: %v", ids)
	}
}

func TestVideosWithoutDetailsExcludesCached(t *testing.T) {
	d := newTestDB(t)
	seedSubscribed(t, d, "UCsub", []domain.Video{vid("s1", "20250103"), vid("s2", "20250102")})
	if err := d.SaveFeedCache(context.Background(), "recommended", []domain.Video{vid("r1", "20250101")}); err != nil {
		t.Fatalf("SaveFeedCache: %v", err)
	}
	// Cache details for s1 and r1 — they must drop out of the "without details" sets.
	if err := d.SaveVideoDetailsCache(context.Background(), "s1", "desc", "thumb", 0); err != nil {
		t.Fatalf("SaveVideoDetailsCache: %v", err)
	}
	if err := d.SaveVideoDetailsCache(context.Background(), "r1", "desc", "thumb", 0); err != nil {
		t.Fatalf("SaveVideoDetailsCache: %v", err)
	}

	subs, err := d.SubscribedVideosWithoutDetails(context.Background(), 0)
	if err != nil {
		t.Fatalf("SubscribedVideosWithoutDetails: %v", err)
	}
	if len(subs) != 1 || subs[0].ID != "s2" {
		t.Fatalf("subscribed-without-details = %+v, want [s2]", subs)
	}

	recs, err := d.RecommendedVideosWithoutDetails(context.Background())
	if err != nil {
		t.Fatalf("RecommendedVideosWithoutDetails: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("recommended-without-details = %+v, want []", recs)
	}
}
