package tab

import (
	"context"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// The recommended feed is cumulative: a fetched batch merges into the cached
// set. The min-duration / min-views / age filters must therefore apply to the
// whole accumulated list, so videos cached before a threshold was set (or that
// an older threshold let through) are dropped too — not just the incoming batch.
func TestFeedRecommendedFiltersCumulativeList(t *testing.T) {
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{
		Mode: "recommended", StaleDays: 30,
		RecMinDurationSecs: 300, RecMinViews: 1000,
	})
	// Pre-existing cache (as if fetched before the thresholds were set): two
	// violators and one keeper.
	f.recVideos = []domain.Video{
		{ID: "short", Duration: 60, ViewCount: 5000},  // too short
		{ID: "lowviews", Duration: 600, ViewCount: 5}, // too few views
		{ID: "keep-old", Duration: 600, ViewCount: 900_000},
	}

	// A fresh fetch brings one keeper and one new violator.
	model, _ := f.onRecFetched(feedRecFetchedMsg{
		TabTarget: feedTarget,
		videos: []domain.Video{
			{ID: "keep-new", Duration: 400, ViewCount: 2000},
			{ID: "short-new", Duration: 10, ViewCount: 9999}, // too short
		},
	})

	got := map[string]bool{}
	for _, v := range model.(Feed).recVideos {
		got[v.ID] = true
	}
	want := []string{"keep-old", "keep-new"}
	for _, id := range want {
		if !got[id] {
			t.Errorf("expected %q to remain in the recommended feed", id)
		}
	}
	for _, id := range []string{"short", "lowviews", "short-new"} {
		if got[id] {
			t.Errorf("expected %q to be filtered out of the recommended feed", id)
		}
	}
	if len(model.(Feed).recVideos) != 2 {
		t.Errorf("recVideos = %d entries, want 2 (only keepers)", len(model.(Feed).recVideos))
	}
}
