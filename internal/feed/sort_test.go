package feed

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

// SortParity: SortVideos and SortLocalVideos must produce the same relative
// order on equivalent input for every mode.
func TestSortParityAllModes(t *testing.T) {
	videos := []youtube.Video{
		{ID: "a", Title: "Zebra", Channel: "Beta", ViewCount: 100, UploadDate: "20230101", Duration: 300},
		{ID: "b", Title: "apple", Channel: "alpha", ViewCount: 500, UploadDate: "20240601", Duration: 60},
		{ID: "c", Title: "Mango", Channel: "Gamma", ViewCount: 200, UploadDate: "20230601", Duration: 600},
	}
	locals := []db.LocalVideo{
		{ID: "a", Title: "Zebra", Channel: "Beta", ViewCount: 100, UploadDate: "20230101", Duration: 300},
		{ID: "b", Title: "apple", Channel: "alpha", ViewCount: 500, UploadDate: "20240601", Duration: 60},
		{ID: "c", Title: "Mango", Channel: "Gamma", ViewCount: 200, UploadDate: "20230601", Duration: 600},
	}
	modes := []struct {
		mode int
		name string
	}{
		{SortViews, "views"},
		{SortDate, "date"},
		{SortName, "name"},
		{SortChannel, "channel"},
		{SortDuration, "duration"},
		{SortNone, "none"},
	}
	for _, tc := range modes {
		vs := append([]youtube.Video(nil), videos...)
		ls := append([]db.LocalVideo(nil), locals...)
		SortVideos(vs, tc.mode)
		SortLocalVideos(ls, tc.mode)
		for i := range vs {
			if vs[i].ID != ls[i].ID {
				t.Errorf("mode=%s pos=%d: SortVideos id=%s, SortLocalVideos id=%s", tc.name, i, vs[i].ID, ls[i].ID)
			}
		}
	}
}

func TestSortNoneIsNoOp(t *testing.T) {
	videos := []youtube.Video{
		{ID: "c"}, {ID: "a"}, {ID: "b"},
	}
	orig := make([]string, len(videos))
	for i, v := range videos {
		orig[i] = v.ID
	}
	SortVideos(videos, SortNone)
	for i, v := range videos {
		if v.ID != orig[i] {
			t.Errorf("SortNone changed order at pos %d: got %s, want %s", i, v.ID, orig[i])
		}
	}
}

func TestSortExpectedOrders(t *testing.T) {
	vids := func() []youtube.Video {
		return []youtube.Video{
			{ID: "a", Title: "Zebra", Channel: "Beta", ViewCount: 100, UploadDate: "20230101", Duration: 300},
			{ID: "b", Title: "apple", Channel: "alpha", ViewCount: 500, UploadDate: "20240601", Duration: 60},
			{ID: "c", Title: "Mango", Channel: "Gamma", ViewCount: 200, UploadDate: "20230601", Duration: 600},
		}
	}
	cases := []struct {
		mode  int
		order string // expected IDs joined
	}{
		{SortViews, "b c a"},    // 500 > 200 > 100
		{SortDate, "b c a"},     // 20240601 > 20230601 > 20230101
		{SortName, "b c a"},     // apple < Mango < Zebra (case-insensitive)
		{SortChannel, "b a c"},  // alpha < Beta < Gamma (case-insensitive)
		{SortDuration, "c a b"}, // 600 > 300 > 60
	}
	for _, tc := range cases {
		s := vids()
		SortVideos(s, tc.mode)
		got := s[0].ID + " " + s[1].ID + " " + s[2].ID
		if got != tc.order {
			t.Errorf("mode=%d: got order %q, want %q", tc.mode, got, tc.order)
		}
	}
}
