package feed

import (
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// MergeVideos merges incoming into existing by video ID; incoming wins on conflict.
//
// The result order is deterministic: each video keeps the position of its first
// occurrence (existing first, then new IDs from incoming), and a conflicting
// incoming video replaces the earlier one in place. Callers sort the result with
// a stable sort on coarse keys (upload date has day precision), so ties keep
// this input order; building the result from map iteration instead reshuffled
// equal-date videos on every merge, e.g. on each poll tick in the Mixed feed.
func MergeVideos(existing, incoming []domain.Video) []domain.Video {
	pos := make(map[string]int, len(existing)+len(incoming))
	out := make([]domain.Video, 0, len(existing)+len(incoming))
	add := func(v domain.Video) {
		if i, ok := pos[v.ID]; ok {
			out[i] = v
			return
		}
		pos[v.ID] = len(out)
		out = append(out, v)
	}
	for _, v := range existing {
		add(v)
	}
	for _, v := range incoming {
		add(v)
	}
	return out
}

// RemoveVideoByID returns a new slice with the given video ID removed.
func RemoveVideoByID(videos []domain.Video, id string) []domain.Video {
	out := make([]domain.Video, 0, len(videos))
	for _, v := range videos {
		if v.ID != id {
			out = append(out, v)
		}
	}
	return out
}

// RemoveChannelVideos returns a new slice with all of a channel's videos removed,
// matching by channel ID or (case-insensitive) channel name.
func RemoveChannelVideos(videos []domain.Video, ch domain.Channel) []domain.Video {
	out := make([]domain.Video, 0, len(videos))
	for _, v := range videos {
		matchID := ch.ID != "" && v.ChannelID == ch.ID
		matchName := ch.Name != "" && strings.EqualFold(v.Channel, ch.Name)
		if !matchID && !matchName {
			out = append(out, v)
		}
	}
	return out
}
