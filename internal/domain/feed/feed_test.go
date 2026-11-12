package feed

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

func TestFeedAtAndMutations(t *testing.T) {
	var f Feed
	f.SetVideos([]domain.Video{{ID: "a"}, {ID: "b"}, {ID: "c"}})
	if v, ok := f.At(1); !ok || v.ID != "b" {
		t.Errorf("At(1) = %v,%v, want b,true", v.ID, ok)
	}
	if _, ok := f.At(9); ok {
		t.Errorf("At(9) ok=true, want false")
	}
	f.Clear()
	if f.Len() != 0 {
		t.Errorf("after Clear: len=%d", f.Len())
	}
}

func TestFeedSort(t *testing.T) {
	var f Feed
	f.SetVideos([]domain.Video{
		{ID: "a", ViewCount: 100},
		{ID: "b", ViewCount: 500},
		{ID: "c", ViewCount: 200},
	})
	f.Sort(SortViews)
	if ids(f.videos) != "b c a" {
		t.Errorf("Sort(views) = %q, want 'b c a'", ids(f.videos))
	}
}
