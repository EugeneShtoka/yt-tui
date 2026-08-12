// Package feed holds the pure video feed pipeline: filtering, merging, cursor
// preservation, and sorting for the recommended/subscription/local video lists.
// Everything here is UI-free and side-effect-free, so it is cheap to unit-test.
//
// This package is the seed of the P5 data-owner: item #5 grows it into a
// stateful Feed that owns the slices these functions currently operate on.
package feed

import (
	"sort"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// Sort modes shared by every tab's sort chord. These are the canonical
// definitions; each entity supplies only the projection fields it can sort by
// (see sortKey), so a mode whose field is zero for an entity is a stable no-op
// for it — "skip what you don't support", by data rather than by interface.
const (
	SortViews       = 0 // view count desc (default for recommended)
	SortDate        = 1 // upload date desc
	SortName        = 2 // title/name alphabetical asc
	SortNone        = 3 // no re-sort — keep fetch/API order
	SortChannel     = 4 // channel name alphabetical asc
	SortDuration    = 5 // duration desc (longest first)
	SortSubscribers = 6 // subscriber count desc (channels)
	SortTags        = 7 // first tag alphabetical asc, name as tiebreak (channels)
	SortSize        = 8 // file size desc (local videos)
)

// ParseSortName maps a config sort name (as written in a panel's `sort` field
// or the sort-key bindings) to its Sort* mode. ok is false for an empty or
// unrecognized name, letting callers fall back to a per-tab default. The names
// match the SortKeys config fields so the two surfaces stay in sync.
func ParseSortName(s string) (mode int, ok bool) {
	switch s {
	case "views":
		return SortViews, true
	case "date":
		return SortDate, true
	case "name":
		return SortName, true
	case "none":
		return SortNone, true
	case "channel":
		return SortChannel, true
	case "duration":
		return SortDuration, true
	case "subscribers":
		return SortSubscribers, true
	case "tags":
		return SortTags, true
	case "size":
		return SortSize, true
	}
	return 0, false
}

// sortKey is the comparable projection of a sortable item used by sortByMode,
// so one ordering switch serves domain.Video, domain.LocalVideo,
// domain.HistoryEntry and domain.Channel alike. Each SortX wrapper differs only
// in the extract closure that fills the fields its entity actually has.
type sortKey struct {
	viewCount   int64
	uploadDate  string
	title       string
	channel     string
	duration    int
	subscribers int64
	tags        string
	fileSize    int64
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
	case SortSubscribers:
		sort.SliceStable(s, func(i, j int) bool { return extract(s[i]).subscribers > extract(s[j]).subscribers })
	case SortSize:
		sort.SliceStable(s, func(i, j int) bool { return extract(s[i]).fileSize > extract(s[j]).fileSize })
	case SortTags:
		sort.SliceStable(s, func(i, j int) bool {
			ki, kj := extract(s[i]), extract(s[j])
			if ki.tags != kj.tags {
				return ki.tags < kj.tags
			}
			return strings.ToLower(ki.channel) < strings.ToLower(kj.channel)
		})
		// SortNone: no-op — keep current order
	}
}

// SortVideos sorts videos in place by the given mode.
func SortVideos(videos []domain.Video, mode int) {
	sortByMode(videos, mode, func(v domain.Video) sortKey {
		return sortKey{viewCount: v.ViewCount, uploadDate: v.UploadDate, title: v.Title, channel: v.Channel, duration: v.Duration}
	})
}

// SortLocalVideos sorts local videos in place by the given mode.
func SortLocalVideos(videos []domain.LocalVideo, mode int) {
	sortByMode(videos, mode, func(v domain.LocalVideo) sortKey {
		return sortKey{viewCount: v.ViewCount, uploadDate: v.UploadDate, title: v.Title, channel: v.Channel, duration: v.Duration, fileSize: v.FileSize}
	})
}

// SortHistoryEntries sorts history entries in place by the given mode.
func SortHistoryEntries(entries []domain.HistoryEntry, mode int) {
	sortByMode(entries, mode, func(e domain.HistoryEntry) sortKey {
		return sortKey{viewCount: e.ViewCount, uploadDate: e.UploadDate, title: e.Title, channel: e.Channel, duration: e.Duration}
	})
}

// SortChannels sorts channels in place by the given mode. Sort keys are unified
// with the video tabs: SortName ("video name") orders by each channel's latest
// video title, and SortChannel ("channel name") orders by the channel's own
// display name — both looked up per row (latest is keyed by channel ID). The
// date/views/duration modes likewise order by the latest video, matching how
// the Channels tab presents each row; subscribers/tags use the channel's own
// fields, with the channel name as the tag tiebreak.
func SortChannels(chs []domain.Channel, mode int, latest map[string]domain.Video) {
	sortByMode(chs, mode, func(c domain.Channel) sortKey {
		lv := latest[c.ID]
		return sortKey{
			viewCount:   lv.ViewCount,
			uploadDate:  lv.UploadDate,
			duration:    lv.Duration,
			title:       lv.Title,
			channel:     c.DisplayName(),
			subscribers: c.Subscribers,
			tags:        firstTag(c.Tags),
		}
	})
}

// firstTag returns the lowercased first tag, or a high sentinel so untagged
// channels sort last under SortTags.
func firstTag(tags []string) string {
	if len(tags) == 0 {
		return "\xff"
	}
	return strings.ToLower(tags[0])
}
