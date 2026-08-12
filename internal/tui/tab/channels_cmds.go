package tab

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

func (t Channels) chsLoadCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := t.ctx
		// Load the full channel universe (subscribed, annotated-none, blocked) so
		// the view picker can switch between them client-side without refetching.
		chans, err := t.backend.AllChannels(ctx)
		if err != nil {
			return chsLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabChannels}, err: err}
		}
		latest, err := t.backend.GetChannelLatestAll(ctx)
		if err != nil {
			latest = make(map[string]domain.Video)
		}
		// The recommended-feed cache backs the recommended/mixed modes (a discovery
		// surface for tagging channels we don't subscribe to). Best-effort: an error
		// or empty cache just means no rec channels this load.
		recVideos, _ := t.backend.GetFeedCache(ctx, "recommended")
		return chsLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabChannels}, chans: chans, recVideos: recVideos, latest: latest}
	}
}

// mergeRecLatest returns latest with a best-effort "latest video" filled in for
// rec-feed channels that have no DB-derived entry (they aren't in channel_videos),
// so the recommended mode shows the video that surfaced each channel. The DB map
// wins for channels present in both. recVideos is scanned in feed order; the
// newest by upload date per channel is kept.
func mergeRecLatest(latest map[string]domain.Video, recVideos []domain.Video) map[string]domain.Video {
	out := make(map[string]domain.Video, len(latest)+len(recVideos))
	for id, v := range latest {
		out[id] = v
	}
	for i := range recVideos {
		v := recVideos[i]
		if v.ChannelID == "" {
			continue
		}
		if _, fromDB := latest[v.ChannelID]; fromDB {
			continue
		}
		if cur, ok := out[v.ChannelID]; !ok || v.UploadDate > cur.UploadDate {
			out[v.ChannelID] = v
		}
	}
	return out
}

// chPollFetchCmd reads the channel's currently-cached videos from the DB (no
// network) so the open list reflects whatever a background crawl has persisted.
func (t Channels) chPollFetchCmd(channelID string) tea.Cmd {
	return func() tea.Msg {
		vids, err := t.backend.GetChannelVideos(t.ctx, channelID)
		if err != nil {
			// Treat a read error as "no new videos"; the idle counter will retire
			// the loop if it persists.
			return chVideosPolledMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabChannels}, channelID: channelID}
		}
		return chVideosPolledMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabChannels}, channelID: channelID, videos: vids}
	}
}

func (t Channels) chDrilldownCmd(ch domain.Channel) tea.Cmd {
	n := t.channelLatestCount
	return func() tea.Msg {
		ctx := t.ctx
		cached, err := t.backend.GetChannelVideos(ctx, ch.ID)
		if err == nil && len(cached) > 0 {
			return chVideosCachedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabChannels}, channelID: ch.ID, videos: cached}
		}
		// First load pulls only the latest N (one bounded yt-dlp call) rather than
		// crawling the whole back-catalog: a full paginated pull of a large channel
		// runs dozens of sequential yt-dlp pages, routinely trips YouTube's 429 rate
		// limit, and leaves the drill-in spinner hanging for minutes. The full list
		// comes from the background backfill or an explicit force-refresh (R). Falls
		// back to a full fetch only when N <= 0 (latest-N disabled).
		var videos []domain.Video
		if n > 0 {
			videos, err = t.backend.ChannelLatestN(ctx, ch.URL, ch.ID, n)
		} else {
			videos, err = t.backend.ChannelVideos(ctx, ch.URL, ch.ID)
		}
		if err != nil {
			return chVideosFetchedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabChannels}, channelID: ch.ID, err: err}
		}
		return chVideosFetchedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabChannels}, channelID: ch.ID, videos: videos}
	}
}

// channelStale reports whether a channel's videos should be auto-refreshed:
// true if throttling is disabled, the channel was never fetched this session,
// or the last fetch is older than refreshInterval.
func (t Channels) channelStale(channelID string) bool {
	if t.refreshInterval <= 0 {
		return true
	}
	last, ok := t.lastRefresh[channelID]
	return !ok || time.Since(last) >= t.refreshInterval
}

// chRefreshCmd fetches the active channel's videos. When full is true it pulls
// the entire (paginated) video list; otherwise it pulls the latest N (per
// channelLatestCount, falling back to a full fetch when N <= 0).
func (t Channels) chRefreshCmd(full bool) tea.Cmd {
	chID, chURL, n := t.activeChID, t.activeChURL, t.channelLatestCount
	return func() tea.Msg {
		ctx := t.ctx
		var videos []domain.Video
		var err error
		if full || n <= 0 {
			videos, err = t.backend.ChannelVideos(ctx, chURL, chID)
		} else {
			videos, err = t.backend.ChannelLatestN(ctx, chURL, chID, n)
		}
		if err != nil {
			return chVideosFetchedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabChannels}, channelID: chID, err: err}
		}
		return chVideosFetchedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabChannels}, channelID: chID, videos: videos}
	}
}

func (t Channels) chSetAliasCmd(ch domain.Channel, alias, status string) tea.Cmd {
	return func() tea.Msg {
		ctx := t.ctx
		if err := t.materialize(ctx, ch); err != nil {
			return tuipkg.StatusMsg{Text: "alias: " + err.Error(), IsErr: true}
		}
		if err := t.backend.SetChannelAlias(ctx, ch.ID, alias); err != nil {
			return tuipkg.StatusMsg{Text: "alias: " + err.Error(), IsErr: true}
		}
		return tuipkg.StatusMsg{Text: status}
	}
}

func (t Channels) chSetTagsCmd(ch domain.Channel, tags []string) tea.Cmd {
	return func() tea.Msg {
		ctx := t.ctx
		if err := t.materialize(ctx, ch); err != nil {
			return tuipkg.StatusMsg{Text: "tags: " + err.Error(), IsErr: true}
		}
		if err := t.backend.SetChannelTags(ctx, ch.ID, tags); err != nil {
			return tuipkg.StatusMsg{Text: "tags: " + err.Error(), IsErr: true}
		}
		return tuipkg.StatusMsg{Text: "Tags updated"}
	}
}

// materialize persists a recommended-feed channel that has no stored row yet, so
// an alias/tags annotation survives the channel leaving the feed. It upserts a
// state=none row (via AddSubscribedChannel, which preserves any existing
// alias/tags and enforces the block invariant) only for unsubscribed, unblocked
// channels — subscribed and blocked channels already have a row, and re-running
// it on an existing state=none row is a harmless no-op that keeps its annotations.
func (t Channels) materialize(ctx context.Context, ch domain.Channel) error {
	if ch.IsSubscribed() || ch.Blocked {
		return nil
	}
	bare := domain.Channel{ID: ch.ID, Name: ch.Name, URL: ch.URL, Subscribers: ch.Subscribers, State: domain.SubNone}
	if err := t.backend.AddSubscribedChannel(ctx, bare); err != nil {
		return fmt.Errorf("materialize channel: %w", err)
	}
	return nil
}

func parseTags(val string) []string {
	parts := strings.Split(val, ",")
	var tags []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}
