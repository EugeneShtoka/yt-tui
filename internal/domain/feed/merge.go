package feed

import (
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// PreserveCursor maps a cursor position from an old video slice to the index of
// the same video (by ID) in a new slice, returning 0 if it can't be found.
func PreserveCursor(old []domain.Video, cursor int, updated []domain.Video) int {
	if cursor >= len(old) {
		return 0
	}
	prevID := old[cursor].ID
	for i, v := range updated {
		if v.ID == prevID {
			return i
		}
	}
	return 0
}

// MergeVideos merges incoming into existing by video ID; incoming wins on conflict.
func MergeVideos(existing, incoming []domain.Video) []domain.Video {
	m := make(map[string]domain.Video, len(existing)+len(incoming))
	for _, v := range existing {
		m[v.ID] = v
	}
	for _, v := range incoming {
		m[v.ID] = v
	}
	out := make([]domain.Video, 0, len(m))
	for _, v := range m {
		out = append(out, v)
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
func RemoveChannelVideos(videos []domain.Video, channelID, channelName string) []domain.Video {
	out := make([]domain.Video, 0, len(videos))
	for _, v := range videos {
		matchID := channelID != "" && v.ChannelID == channelID
		matchName := channelName != "" && strings.EqualFold(v.Channel, channelName)
		if !matchID && !matchName {
			out = append(out, v)
		}
	}
	return out
}
