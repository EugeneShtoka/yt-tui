package channels

import (
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// RecFeedIDs returns the set of distinct non-empty channel IDs appearing in the
// given (recommended-feed) videos. It is the membership set the Channels/Tags
// panels use to decide whether a channel is "in the recommended feed" for the
// recommended/mixed modes.
func RecFeedIDs(videos []domain.Video) map[string]bool {
	ids := make(map[string]bool)
	for i := range videos {
		if id := videos[i].ChannelID; id != "" {
			ids[id] = true
		}
	}
	return ids
}

// SynthesizeRec folds recommended-feed channels into the channel universe
// without persisting them: for every distinct channel ID that appears in videos
// but has no row in have, it returns a bare domain.Channel at State=SubNone,
// carrying the first-seen name and a canonical channel URL derived from the ID.
//
// This is the lazy half of the rec-channel model — untagged rec channels are
// derived live each load and never written to the DB; a row is materialized only
// when the user first tags/aliases the channel (see the Channels tab). Channels
// already present in have (subscribed, blocked, or previously annotated) are
// skipped so their stored annotations/state win.
func SynthesizeRec(have []domain.Channel, videos []domain.Video) []domain.Channel {
	stored := make(map[string]bool, len(have))
	for i := range have {
		if have[i].ID != "" {
			stored[have[i].ID] = true
		}
	}
	seen := make(map[string]bool)
	var out []domain.Channel
	for i := range videos {
		id := videos[i].ChannelID
		if id == "" || stored[id] || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, domain.Channel{
			ID:    id,
			Name:  videos[i].Channel,
			URL:   channelURL(id),
			State: domain.SubNone,
		})
	}
	return out
}

// channelURL builds the canonical YouTube channel URL from a channel ID so a
// synthesized rec channel is still drillable (ChannelVideos needs a URL). IDs
// that already look like a URL are returned unchanged.
func channelURL(id string) string {
	if strings.HasPrefix(id, "http://") || strings.HasPrefix(id, "https://") {
		return id
	}
	return "https://www.youtube.com/channel/" + id
}
