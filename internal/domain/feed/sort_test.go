package feed

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// SortParity: SortVideos and SortLocalVideos must produce the same relative
// order on equivalent input for every mode.
func TestSortParityAllModes(t *testing.T) {
	videos := []domain.Video{
		{ID: "a", Title: "Zebra", Channel: "Beta", ViewCount: 100, UploadDate: "20230101", Duration: 300},
		{ID: "b", Title: "apple", Channel: "alpha", ViewCount: 500, UploadDate: "20240601", Duration: 60},
		{ID: "c", Title: "Mango", Channel: "Gamma", ViewCount: 200, UploadDate: "20230601", Duration: 600},
	}
	locals := []domain.LocalVideo{
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
		vs := append([]domain.Video(nil), videos...)
		ls := append([]domain.LocalVideo(nil), locals...)
		SortVideos(vs, tc.mode)
		SortLocalVideos(ls, tc.mode)
		for i := range vs {
			if vs[i].ID != ls[i].ID {
				t.Errorf("mode=%s pos=%d: SortVideos id=%s, SortLocalVideos id=%s", tc.name, i, vs[i].ID, ls[i].ID)
			}
		}
	}
}

// SortSize is a local-video-only mode: it orders local videos by bytes on disk
// (largest first) and is a stable no-op for entities without a file size.
func TestSortLocalVideosBySize(t *testing.T) {
	locals := []domain.LocalVideo{
		{ID: "a", FileSize: 300},
		{ID: "b", FileSize: 1000},
		{ID: "c", FileSize: 50},
	}
	SortLocalVideos(locals, SortSize)
	got := ""
	for _, lv := range locals {
		got += lv.ID
	}
	if got != "bac" {
		t.Errorf("SortSize order = %q, want %q (1000 > 300 > 50)", got, "bac")
	}

	// Videos carry no size projection, so SortSize must not reorder them.
	videos := []domain.Video{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	SortVideos(videos, SortSize)
	if videos[0].ID != "a" || videos[1].ID != "b" || videos[2].ID != "c" {
		t.Errorf("SortSize reordered size-less videos: %v", videos)
	}
}

// SortChannels shares the video sort keys. The two alphabetical modes must stay
// distinct across every tab: SortName ("video name") orders by the latest
// video's title, SortChannel ("channel name") by the channel's own name.
func TestSortChannelsExpectedOrders(t *testing.T) {
	chans := func() []domain.Channel {
		return []domain.Channel{
			{ID: "a", Name: "Beta", Subscribers: 100, Tags: []string{"news"}},
			{ID: "b", Name: "alpha", Subscribers: 500, Tags: []string{"arts"}},
			{ID: "c", Name: "Gamma", Subscribers: 200}, // no tags
		}
	}
	latest := map[string]domain.Video{
		"a": {Title: "Zebra", ViewCount: 100, UploadDate: "20230101", Duration: 300},
		"b": {Title: "apple", ViewCount: 500, UploadDate: "20240601", Duration: 60},
		"c": {Title: "Mango", ViewCount: 200, UploadDate: "20230601", Duration: 600},
	}
	cases := []struct {
		mode  int
		order string
		desc  string
	}{
		{SortViews, "b c a", "latest video views desc"},
		{SortDate, "b c a", "latest video date desc"},
		{SortName, "b c a", "latest video title asc (apple<Mango<Zebra)"},
		{SortChannel, "b a c", "channel name asc (alpha<Beta<Gamma)"},
		{SortDuration, "c a b", "latest video duration desc"},
		{SortSubscribers, "b c a", "subscribers desc"},
		{SortTags, "b a c", "first tag asc, untagged last (arts<news<∅)"},
	}
	for _, tc := range cases {
		s := chans()
		SortChannels(s, tc.mode, latest)
		got := s[0].ID + " " + s[1].ID + " " + s[2].ID
		if got != tc.order {
			t.Errorf("mode=%d (%s): got order %q, want %q", tc.mode, tc.desc, got, tc.order)
		}
	}
}

func TestSortNoneIsNoOp(t *testing.T) {
	videos := []domain.Video{
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
	vids := func() []domain.Video {
		return []domain.Video{
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
