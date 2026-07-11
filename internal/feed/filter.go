package feed

import (
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

// FilterByMinDuration removes videos shorter than minSecs seconds.
// Videos with Duration == 0 (unknown) are kept. Pass minSecs <= 0 to skip.
func FilterByMinDuration(videos []youtube.Video, minSecs int) []youtube.Video {
	if minSecs <= 0 {
		return videos
	}
	out := make([]youtube.Video, 0, len(videos))
	for _, v := range videos {
		if v.Duration == 0 || v.Duration >= minSecs {
			out = append(out, v)
		}
	}
	return out
}

// FilterByMinViews removes videos with fewer than minViews views.
// Videos with ViewCount == 0 (unknown) are kept. Pass minViews <= 0 to skip.
func FilterByMinViews(videos []youtube.Video, minViews int) []youtube.Video {
	if minViews <= 0 {
		return videos
	}
	out := make([]youtube.Video, 0, len(videos))
	for _, v := range videos {
		if v.ViewCount == 0 || v.ViewCount >= int64(minViews) {
			out = append(out, v)
		}
	}
	return out
}

// FilterByAge removes videos whose upload date is older than maxDays.
// Videos with no date are kept.
func FilterByAge(videos []youtube.Video, maxDays int) []youtube.Video {
	if maxDays <= 0 {
		return videos
	}
	cutoff := time.Now().AddDate(0, 0, -maxDays)
	out := make([]youtube.Video, 0, len(videos))
	for _, v := range videos {
		if len(v.UploadDate) != 8 {
			out = append(out, v)
			continue
		}
		t, err := time.Parse("20060102", v.UploadDate)
		if err != nil || !t.Before(cutoff) {
			out = append(out, v)
		}
	}
	return out
}

// FilterDownloaded removes videos that are already in the local library.
func FilterDownloaded(videos []youtube.Video, local map[string]db.LocalVideo) []youtube.Video {
	out := make([]youtube.Video, 0, len(videos))
	for _, v := range videos {
		if _, ok := local[v.ID]; !ok {
			out = append(out, v)
		}
	}
	return out
}

// FilterHidden removes videos the user has explicitly hidden from recommended.
func FilterHidden(videos []youtube.Video, hidden map[string]bool) []youtube.Video {
	out := make([]youtube.Video, 0, len(videos))
	for _, v := range videos {
		if !hidden[v.ID] {
			out = append(out, v)
		}
	}
	return out
}

// FilterBlacklisted removes videos whose channel is blacklisted.
// As a side effect it enriches name-only blacklist entries with the channel ID.
func FilterBlacklisted(videos []youtube.Video, list []config.BlacklistedChannel, cfg *config.Config) []youtube.Video {
	out := make([]youtube.Video, 0, len(videos))
	for _, v := range videos {
		if bl, matched := MatchBlacklisted(v, list); matched {
			if bl >= 0 && cfg.BlacklistedChannels[bl].ID == "" && v.ChannelID != "" {
				cfg.SetBlacklistID(bl, v.ChannelID)
				cfg.SaveAsync()
			}
			continue
		}
		out = append(out, v)
	}
	return out
}

// MatchBlacklisted returns the index in list and true if the video's channel is
// blacklisted. Matches by ID first (exact), then by name (case-insensitive) for
// entries without an ID.
func MatchBlacklisted(v youtube.Video, list []config.BlacklistedChannel) (int, bool) {
	for i, bl := range list {
		if bl.ID != "" && bl.ID == v.ChannelID {
			return i, true
		}
		if bl.ID == "" && strings.EqualFold(bl.Name, v.Channel) {
			return i, true
		}
	}
	return -1, false
}

// FilterSubscribed removes videos whose channel the user is already subscribed
// to (matched by channel ID or, failing that, by lowercased channel name).
func FilterSubscribed(videos []youtube.Video, subscribed map[string]bool) []youtube.Video {
	if len(subscribed) == 0 {
		return videos
	}
	out := make([]youtube.Video, 0, len(videos))
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
