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

// ── PreserveCursor ────────────────────────────────────────────────────────────

func TestPreserveCursor(t *testing.T) {
	old := []domain.Video{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	updated := []domain.Video{{ID: "x"}, {ID: "b"}, {ID: "c"}}
	// cursor on "b" (index 1) should follow it to its new index (1).
	if got := PreserveCursor(old, 1, updated); got != 1 {
		t.Errorf("PreserveCursor followed to %d, want 1", got)
	}
	// cursor on a video absent from the new slice resets to 0.
	if got := PreserveCursor(old, 0, updated); got != 0 {
		t.Errorf("PreserveCursor for missing video = %d, want 0", got)
	}
	// out-of-range cursor resets to 0.
	if got := PreserveCursor(old, 99, updated); got != 0 {
		t.Errorf("PreserveCursor out-of-range = %d, want 0", got)
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
	if got := ids(RemoveChannelVideos(videos, "ch1", "")); got != "b c" {
		t.Errorf("remove by channel ID: got %q, want 'b c'", got)
	}
	// by name (case-insensitive) removes both b and c
	if got := ids(RemoveChannelVideos(videos, "", "two")); got != "a" {
		t.Errorf("remove by channel name: got %q, want 'a'", got)
	}
}
