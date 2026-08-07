package service

import (
	"context"
	"fmt"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
)

// resolveState maps an incoming subscription_state to what it becomes locally.
// A YouTube subscription isn't portable across accounts (locked decision #1): it
// converts to a local subscription when the caller opts in, else drops to none
// (annotations are still kept because they live on the same row). Local and none
// import as-is; any unknown value is treated as none.
func resolveState(state string, opts portability.ImportOptions) domain.SubscriptionState {
	switch domain.SubscriptionState(state) {
	case domain.SubYT:
		if opts.ConvertYTToLocal {
			return domain.SubLocal
		}
		return domain.SubNone
	case domain.SubLocal:
		return domain.SubLocal
	default:
		return domain.SubNone
	}
}

// mergeTags unions two tag lists preserving existing order then appending
// incoming tags not already present (exact match). Returns nil when both empty.
func mergeTags(existing, incoming []string) []string {
	seen := make(map[string]bool, len(existing)+len(incoming))
	var out []string
	for _, t := range existing {
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for _, t := range incoming {
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// resolveChannel applies the bundle merge policy for one channel against the
// current row (if any): tags union, alias/name/url/subscribers incoming-wins
// when non-empty (never clobbered with a blank), blocked is sticky (incoming
// blocked=1 wins and import never unblocks), and a blocked result forces state
// none. The returned channel is the exact row to upsert.
func resolveChannel(existing map[string]domain.Channel, in portability.ChannelExport, opts portability.ImportOptions) domain.Channel {
	cur := existing[in.ChannelID] // zero value when absent
	out := domain.Channel{
		ID:   in.ChannelID,
		Name: preferString(in.Name, cur.Name),
		URL:  preferString(in.URL, cur.URL),
		// subscriber count isn't carried in the bundle; keep whatever we have.
		Subscribers: cur.Subscribers,
		Alias:       preferString(in.Alias, cur.Alias),
		Tags:        mergeTags(cur.Tags, in.Tags),
		Blocked:     cur.Blocked || in.Blocked,
	}
	if out.Blocked {
		out.State = domain.SubNone
	} else {
		out.State = resolveState(in.SubscriptionState, opts)
	}
	return out
}

// preferString returns incoming when non-empty, else the existing value.
func preferString(incoming, existing string) string {
	if incoming != "" {
		return incoming
	}
	return existing
}

// historyKey identifies a history event for dedup: same video, type, and second
// is treated as the same event (timestamps are stored at second granularity).
func historyKey(videoID, eventType string, unixTS int64) string {
	return fmt.Sprintf("%s\x00%s\x00%d", videoID, eventType, unixTS)
}

// toSet builds a lookup set from a string slice.
func toSet(vs []string) map[string]bool {
	m := make(map[string]bool, len(vs))
	for _, v := range vs {
		m[v] = true
	}
	return m
}

// ── read helpers (shared by preview + apply) ────────────────────────────────

func (s *PortabilityService) channelsByID(ctx context.Context) (map[string]domain.Channel, error) {
	all, err := s.repo.AllChannels(ctx)
	if err != nil {
		return nil, fmt.Errorf("channelsByID: %w", err)
	}
	m := make(map[string]domain.Channel, len(all))
	for i := range all {
		m[all[i].ID] = all[i]
	}
	return m, nil
}

// playlistVideoSets maps each existing local playlist name to the set of video
// ids it already contains, so merge-by-name can dedup additions.
func (s *PortabilityService) playlistVideoSets(ctx context.Context) (map[string]map[string]bool, error) {
	pls, err := s.repo.Playlists(ctx)
	if err != nil {
		return nil, fmt.Errorf("playlistVideoSets: %w", err)
	}
	out := make(map[string]map[string]bool, len(pls))
	for i := range pls {
		vids, err := s.repo.PlaylistVideos(ctx, pls[i].ID)
		if err != nil {
			return nil, fmt.Errorf("playlistVideoSets %q: %w", pls[i].Name, err)
		}
		set := make(map[string]bool, len(vids))
		for j := range vids {
			set[vids[j].ID] = true
		}
		out[pls[i].Name] = set
	}
	return out, nil
}

// historyKeys builds the dedup set of existing history events.
func (s *PortabilityService) historyKeys(ctx context.Context) (map[string]bool, error) {
	hist, err := s.repo.History(ctx, unlimited)
	if err != nil {
		return nil, fmt.Errorf("historyKeys: %w", err)
	}
	set := make(map[string]bool, len(hist))
	for i := range hist {
		set[historyKey(hist[i].VideoID, hist[i].EventType, hist[i].Timestamp.Unix())] = true
	}
	return set, nil
}
