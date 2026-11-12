package feed

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ── MergeVideos ───────────────────────────────────────────────────────────────

func TestMergeVideosEmpty(t *testing.T) {
	result := MergeVideos(nil, nil)
	if len(result) != 0 {
		t.Errorf("merging empty slices: got len=%d, want 0", len(result))
	}
}

func TestMergeVideosNoConflict(t *testing.T) {
	existing := []domain.Video{{ID: "a"}, {ID: "b"}}
	incoming := []domain.Video{{ID: "c"}, {ID: "d"}}
	result := MergeVideos(existing, incoming)
	if len(result) != 4 {
		t.Errorf("no conflict merge: got len=%d, want 4", len(result))
	}
}

func TestMergeVideosIncomingWins(t *testing.T) {
	existing := []domain.Video{{ID: "a", Title: "old"}}
	incoming := []domain.Video{{ID: "a", Title: "new"}}
	result := MergeVideos(existing, incoming)
	if len(result) != 1 {
		t.Errorf("merge conflict: got len=%d, want 1", len(result))
	}
	for _, v := range result {
		if v.ID == "a" && v.Title != "new" {
			t.Errorf("merge conflict: incoming didn't win, title=%s", v.Title)
		}
	}
}

// MergeVideos must return a deterministic order: first occurrence wins the
// position (existing first, then new incoming IDs) and a conflict replaces in
// place. The Mixed feed re-merges on every poll tick and stable-sorts by
// day-precision upload date, so a map-iteration order here reshuffles all
// same-day videos every few seconds. Repeated runs catch a regression to map
// order, which Go randomizes per iteration.
func TestMergeVideosDeterministicOrder(t *testing.T) {
	existing := []domain.Video{{ID: "a"}, {ID: "b", Title: "old"}, {ID: "c"}}
	incoming := []domain.Video{{ID: "d"}, {ID: "b", Title: "new"}, {ID: "e"}}
	want := []string{"a", "b", "c", "d", "e"}
	for run := range 50 {
		got := MergeVideos(existing, incoming)
		if len(got) != len(want) {
			t.Fatalf("run %d: len=%d, want %d", run, len(got), len(want))
		}
		for i, id := range want {
			if got[i].ID != id {
				t.Fatalf("run %d: order = %s, want %v", run, ids(got), want)
			}
		}
		if got[1].Title != "new" {
			t.Fatalf("run %d: conflict not replaced in place, b.Title=%q", run, got[1].Title)
		}
	}
}

// ── RemoveVideoByID ───────────────────────────────────────────────────────────

func TestRemoveVideoByIDEmpty(t *testing.T) {
	result := RemoveVideoByID(nil, "any")
	if len(result) != 0 {
		t.Errorf("remove from nil: got len=%d, want 0", len(result))
	}
}

func TestRemoveVideoByIDNotFound(t *testing.T) {
	videos := []domain.Video{{ID: "a"}, {ID: "b"}}
	result := RemoveVideoByID(videos, "c")
	if len(result) != 2 {
		t.Errorf("remove not found: got len=%d, want 2", len(result))
	}
}

func TestRemoveVideoByIDFound(t *testing.T) {
	videos := []domain.Video{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	result := RemoveVideoByID(videos, "b")
	if len(result) != 2 {
		t.Errorf("remove found: got len=%d, want 2", len(result))
	}
	for _, v := range result {
		if v.ID == "b" {
			t.Errorf("remove found: 'b' was not removed")
		}
	}
}

// ── RemoveChannelVideos ───────────────────────────────────────────────────────

func TestRemoveChannelVideos(t *testing.T) {
	videos := []domain.Video{
		{ID: "a", ChannelID: "ch1"},
		{ID: "b", ChannelID: "ch2", Channel: "Two"},
		{ID: "c", Channel: "Two"},
	}
	// by ID
	if got := ids(RemoveChannelVideos(videos, domain.Channel{ID: "ch1"})); got != "b c" {
		t.Errorf("remove by channel ID: got %q, want 'b c'", got)
	}
	// by name (case-insensitive) removes both b and c
	if got := ids(RemoveChannelVideos(videos, domain.Channel{Name: "two"})); got != "a" {
		t.Errorf("remove by channel name: got %q, want 'a'", got)
	}
}
