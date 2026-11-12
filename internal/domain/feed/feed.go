package feed

import "github.com/EugeneShtoka/yt-tui/internal/domain"

// Feed owns a video list. It is the data-owner the P5 review calls for
// (docs/ARCH_REVIEW_PLAN.md #5): state that previously sat as loose fields on
// the UI Model, consolidated behind a tested API so views read through it
// instead of the Model owning raw slices.
//
// The zero value is a valid empty feed. A Feed is held by value on the
// Model and mutated through pointer methods on the addressable field, so changes
// persist across Bubble Tea's value-copy of the Model (the same trick P4 used for
// the per-tab view structs).
type Feed struct {
	videos []domain.Video
}

// New builds a Feed holding videos.
func New(videos []domain.Video) Feed {
	return Feed{videos: videos}
}

// ── Reads ─────────────────────────────────────────────────────────────────────

func (f *Feed) Videos() []domain.Video { return f.videos }
func (f *Feed) Len() int               { return len(f.videos) }

// At returns the video at i, or false if i is out of range.
func (f *Feed) At(i int) (domain.Video, bool) {
	if i >= 0 && i < len(f.videos) {
		return f.videos[i], true
	}
	return domain.Video{}, false
}

// ── Mutations ─────────────────────────────────────────────────────────────────

// SetVideos replaces the video list (e.g. after an out-of-band filter pass).
func (f *Feed) SetVideos(v []domain.Video) { f.videos = v }

// Clear empties the feed.
func (f *Feed) Clear() { f.videos = nil }

// Sort orders the feed in place by the given mode.
func (f *Feed) Sort(mode int) { SortVideos(f.videos, mode) }
