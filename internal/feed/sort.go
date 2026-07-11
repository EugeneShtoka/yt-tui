// Package feed holds the pure video feed pipeline: filtering, merging, cursor
// preservation, and sorting for the recommended/subscription/local video lists.
// Everything here is UI-free and side-effect-free (except FilterBlacklisted,
// which enriches the passed *config.Config), so it is cheap to unit-test.
//
// This package is the seed of the P5 data-owner: item #5 grows it into a
// stateful Feed that owns the slices these functions currently operate on.
package feed

import (
	"sort"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

// Video list sort modes (used by each tab view's sort field). These are the
// canonical definitions; the ui package aliases them as vidSort* for its views.
const (
	SortViews    = 0 // view count desc (default for recommended)
	SortDate     = 1 // upload date desc
	SortName     = 2 // title alphabetical asc
	SortNone     = 3 // no re-sort — keep fetch/API order
	SortChannel  = 4 // channel name alphabetical asc
	SortDuration = 5 // duration desc (longest first)
)

// sortKey is the comparable projection of a video used by sortByMode, so the
// same ordering logic serves both youtube.Video and db.LocalVideo.
type sortKey struct {
	viewCount  int64
	uploadDate string
	title      string
	channel    string
	duration   int
}

func sortByMode[T any](s []T, mode int, extract func(T) sortKey) {
	switch mode {
	case SortViews:
		sort.SliceStable(s, func(i, j int) bool { return extract(s[i]).viewCount > extract(s[j]).viewCount })
	case SortDate:
		sort.SliceStable(s, func(i, j int) bool { return extract(s[i]).uploadDate > extract(s[j]).uploadDate })
	case SortName:
		sort.SliceStable(s, func(i, j int) bool {
			return strings.ToLower(extract(s[i]).title) < strings.ToLower(extract(s[j]).title)
		})
	case SortChannel:
		sort.SliceStable(s, func(i, j int) bool {
			return strings.ToLower(extract(s[i]).channel) < strings.ToLower(extract(s[j]).channel)
		})
	case SortDuration:
		sort.SliceStable(s, func(i, j int) bool { return extract(s[i]).duration > extract(s[j]).duration })
		// SortNone: no-op — keep current order
	}
}

// SortVideos sorts videos in place by the given mode.
func SortVideos(videos []youtube.Video, mode int) {
	sortByMode(videos, mode, func(v youtube.Video) sortKey {
		return sortKey{v.ViewCount, v.UploadDate, v.Title, v.Channel, v.Duration}
	})
}

// SortLocalVideos sorts local videos in place by the given mode.
func SortLocalVideos(videos []db.LocalVideo, mode int) {
	sortByMode(videos, mode, func(v db.LocalVideo) sortKey {
		return sortKey{v.ViewCount, v.UploadDate, v.Title, v.Channel, v.Duration}
	})
}
