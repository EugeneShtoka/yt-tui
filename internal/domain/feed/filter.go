package feed

import (
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// FilterRecommended applies the recommended-feed property filters in a single
// pass: max age (days), min duration (seconds), and min views. Each is disabled
// when its threshold is <= 0; when all three are, the input is returned as-is.
//
// A video with an unknown value for an enabled filter — zero duration/views, or
// an absent/unparseable upload date — is kept: missing metadata shouldn't hide a
// video. This is the single source of truth for the recommended feed's property
// filtering, shared by the fetch path (FeedService.Recommended) and the
// cumulative display path (the Feed tab), so the rule lives in one place instead
// of three separate passes duplicated at each call site.
func FilterRecommended(videos []domain.Video, maxDays, minSecs, minViews int) []domain.Video {
	if maxDays <= 0 && minSecs <= 0 && minViews <= 0 {
		return videos
	}
	var cutoff time.Time
	if maxDays > 0 {
		cutoff = time.Now().AddDate(0, 0, -maxDays)
	}
	out := make([]domain.Video, 0, len(videos))
	for _, v := range videos {
		if minSecs > 0 && v.Duration != 0 && v.Duration < minSecs {
			continue
		}
		if minViews > 0 && v.ViewCount != 0 && v.ViewCount < int64(minViews) {
			continue
		}
		if maxDays > 0 && olderThan(v.UploadDate, cutoff) {
			continue
		}
		out = append(out, v)
	}
	return out
}

// olderThan reports whether a "20060102" upload date is strictly before cutoff.
// An absent or unparseable date counts as not-older (the video is kept).
func olderThan(uploadDate string, cutoff time.Time) bool {
	if len(uploadDate) != 8 {
		return false
	}
	t, err := time.Parse("20060102", uploadDate)
	if err != nil {
		return false
	}
	return t.Before(cutoff)
}

// FilterDownloaded removes videos that are already in the local library.
func FilterDownloaded(videos []domain.Video, local map[string]domain.LocalVideo) []domain.Video {
	out := make([]domain.Video, 0, len(videos))
	for _, v := range videos {
		if _, ok := local[v.ID]; !ok {
			out = append(out, v)
		}
	}
	return out
}

// FilterHidden removes videos the user has explicitly hidden from recommended.
func FilterHidden(videos []domain.Video, hidden map[string]bool) []domain.Video {
	out := make([]domain.Video, 0, len(videos))
	for _, v := range videos {
		if !hidden[v.ID] {
			out = append(out, v)
		}
	}
	return out
}

// Blocklist is the set of blocked channels used to filter the recommended feed.
// It is a pure projection derived by the service layer from the DB (channel rows
// flagged blocked=1), keeping this package free of any storage dependency. IDs
// are matched exactly.
type Blocklist struct {
	IDs map[string]bool // blocked channel IDs
}

// NewBlocklist builds a Blocklist from a slice of blocked channel IDs.
func NewBlocklist(ids []string) Blocklist {
	bl := Blocklist{IDs: make(map[string]bool, len(ids))}
	for _, id := range ids {
		if id != "" {
			bl.IDs[id] = true
		}
	}
	return bl
}

// Match reports whether the video's channel is blocked, by channel ID (exact).
func (b Blocklist) Match(v domain.Video) bool {
	return v.ChannelID != "" && b.IDs[v.ChannelID]
}

// FilterBlacklisted removes videos whose channel is blocked. It is pure.
func FilterBlacklisted(videos []domain.Video, bl Blocklist) []domain.Video {
	out := make([]domain.Video, 0, len(videos))
	for _, v := range videos {
		if bl.Match(v) {
			continue
		}
		out = append(out, v)
	}
	return out
}

// FilterSubscribed removes videos whose channel the user is already subscribed
// to (matched by channel ID or, failing that, by lowercased channel name).
func FilterSubscribed(videos []domain.Video, subscribed map[string]bool) []domain.Video {
	if len(subscribed) == 0 {
		return videos
	}
	out := make([]domain.Video, 0, len(videos))
	for _, v := range videos {
		if subscribed[v.ChannelID] {
			continue
		}
		if v.Channel != "" && subscribed["name:"+strings.ToLower(v.Channel)] {
			continue
		}
		out = append(out, v)
	}
	return out
}
