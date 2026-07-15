package feed

import (
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// Feed owns a video list together with its fetch lifecycle (loading / refreshing
// / loaded flags + a page counter) and the merge/filter pipeline that produces
// it. It is the data-owner the P5 review calls for (docs/ARCH_REVIEW_PLAN.md #5):
// state that previously sat as loose fields on the UI Model, consolidated behind
// a tested API so views read through it instead of the Model owning raw slices.
//
// The zero value is a valid empty, unloaded feed. A Feed is held by value on the
// Model and mutated through pointer methods on the addressable field, so changes
// persist across Bubble Tea's value-copy of the Model (the same trick P4 used for
// the per-tab view structs).
type Feed struct {
	videos     []domain.Video
	loading    bool
	loaded     bool
	refreshing bool
	page       int
}

// NewStarting builds a Feed seeded with cached videos and immediately marked as
// fetching — the startup state (show cache now, refresh in the background).
func NewStarting(cache []domain.Video) Feed {
	f := Feed{videos: cache, loaded: len(cache) > 0}
	f.StartRefresh()
	return f
}

// New builds a Feed holding videos with no fetch in flight. Used for feeds whose
// loading state is derived externally (e.g. Subscriptions, rebuilt from the
// channel data rather than owning its own fetch lifecycle).
func New(videos []domain.Video) Feed {
	return Feed{videos: videos, loaded: len(videos) > 0}
}

// ── Reads ─────────────────────────────────────────────────────────────────────

func (f *Feed) Videos() []domain.Video { return f.videos }
func (f *Feed) Len() int               { return len(f.videos) }
func (f *Feed) Loading() bool          { return f.loading }
func (f *Feed) Loaded() bool           { return f.loaded }
func (f *Feed) Refreshing() bool       { return f.refreshing }
func (f *Feed) Page() int              { return f.page }

// At returns the video at i, or false if i is out of range.
func (f *Feed) At(i int) (domain.Video, bool) {
	if i >= 0 && i < len(f.videos) {
		return f.videos[i], true
	}
	return domain.Video{}, false
}

// ── Lifecycle ─────────────────────────────────────────────────────────────────

// StartRefresh begins a page-0 fetch. refreshing is true when there is already
// cached content being refreshed underneath.
func (f *Feed) StartRefresh() {
	f.page = 0
	f.loading = true
	f.refreshing = f.loaded
}

// ContinueFetch begins another page of the same fetch (too few results so far),
// keeping the page counter.
func (f *Feed) ContinueFetch() {
	f.loading = true
	f.refreshing = true
}

// FinishFetch marks the in-flight fetch complete and advances the page counter.
func (f *Feed) FinishFetch() {
	f.loading = false
	f.refreshing = false
	f.loaded = true
	f.page++
}

// ── Mutations ─────────────────────────────────────────────────────────────────

// SetVideos replaces the video list (e.g. after an out-of-band filter pass).
func (f *Feed) SetVideos(v []domain.Video) { f.videos = v }

// Clear empties the feed and marks it unloaded (leaving the fetch flags alone).
func (f *Feed) Clear() {
	f.videos = nil
	f.loaded = false
}

// Sort orders the feed in place by the given mode.
func (f *Feed) Sort(mode int) { SortVideos(f.videos, mode) }

// RemoveVideo drops the video with the given ID.
func (f *Feed) RemoveVideo(id string) { f.videos = RemoveVideoByID(f.videos, id) }

// RemoveChannel drops all of a channel's videos (matched by ID or name).
func (f *Feed) RemoveChannel(ch domain.Channel) {
	f.videos = RemoveChannelVideos(f.videos, ch)
}

// MergeOpts carries the inputs to the recommended-feed filter pipeline.
type MergeOpts struct {
	MaxAgeDays      int
	MinDurationSecs int
	MinViews        int
	Downloaded      map[string]domain.LocalVideo
	Hidden          map[string]bool
	Blacklist       []config.BlacklistedChannel
	Cfg             *config.Config // receives blacklist-ID enrichment as a side effect
	Subscribed      map[string]bool
	Sort            int
}

// Merge folds incoming videos into the feed, runs the full age / duration /
// views / downloaded / hidden / blacklist / subscribed filter chain, sorts, and
// stores the result. It returns the cursor remapped from oldCursor so the
// caller's selection follows its video across the merge.
func (f *Feed) Merge(incoming []domain.Video, oldCursor int, o MergeOpts) int {
	merged := MergeVideos(f.videos, incoming)
	filtered := FilterByAge(merged, o.MaxAgeDays)
	filtered = FilterByMinDuration(filtered, o.MinDurationSecs)
	filtered = FilterByMinViews(filtered, o.MinViews)
	filtered = FilterDownloaded(filtered, o.Downloaded)
	filtered = FilterHidden(filtered, o.Hidden)
	filtered = FilterBlacklisted(filtered, o.Blacklist, o.Cfg)
	filtered = FilterSubscribed(filtered, o.Subscribed)
	SortVideos(filtered, o.Sort)
	newCursor := PreserveCursor(f.videos, oldCursor, filtered)
	f.videos = filtered
	return newCursor
}
