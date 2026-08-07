package tab

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/channels"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

func (t *Feed) ensureModeCmd() tea.Cmd {
	var cmds []tea.Cmd
	if t.mode.needsRec() && t.recLoad == srcUnloaded {
		t.recLoad = srcLoading
		cmds = append(cmds, t.recCacheCmd())
	}
	if t.mode.needsSub() && t.subLoad == srcUnloaded {
		t.subLoad = srcLoading
		cmds = append(cmds, t.subLoadCmd())
	}
	if t.mode.needsStale() && t.staleLoad == srcUnloaded {
		t.staleLoad = srcLoading
		cmds = append(cmds, t.staleLoadCmd())
	}
	return tea.Batch(cmds...)
}

// refreshCmd reloads the sources the active mode needs. full clears the
// recommended cache before refetching (the force-refresh path).
func (t *Feed) refreshCmd(full bool) tea.Cmd {
	var cmds []tea.Cmd
	if t.mode.needsRec() {
		t.recLoad = t.recLoad.fetching()
		if full {
			cmds = append(cmds, t.recClearAndFetchCmd())
		} else {
			cmds = append(cmds, t.recFetchCmd())
		}
	}
	if t.mode.needsSub() {
		t.subLoad = t.subLoad.fetching()
		cmds = append(cmds, t.subLoadCmd())
	}
	if t.mode.needsStale() {
		t.staleLoad = t.staleLoad.fetching()
		cmds = append(cmds, t.staleLoadCmd())
	}
	return tea.Batch(cmds...)
}

func (t Feed) recCacheCmd() tea.Cmd {
	return func() tea.Msg {
		videos, err := t.backend.GetFeedCache(t.ctx, "recommended")
		if err != nil || len(videos) == 0 {
			return t.recFetchCmd()()
		}
		return feedRecCacheMsg{TabTarget: feedTarget, videos: videos}
	}
}

func (t Feed) recFetchCmd() tea.Cmd {
	return func() tea.Msg {
		videos, err := t.backend.Recommended(t.ctx)
		return feedRecFetchedMsg{TabTarget: feedTarget, videos: videos, err: err}
	}
}

func (t Feed) recClearAndFetchCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := t.ctx
		_ = t.backend.ClearRecommended(ctx)
		videos, err := t.backend.Recommended(ctx)
		return feedRecFetchedMsg{TabTarget: feedTarget, videos: videos, err: err}
	}
}

func (t Feed) recSaveCacheCmd() tea.Cmd {
	videos := append([]domain.Video(nil), t.recVideos...)
	return func() tea.Msg {
		_ = t.backend.SaveFeedCache(t.ctx, "recommended", videos)
		return nil
	}
}

func (t Feed) subLoadCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := t.ctx
		channels, err := t.backend.GetSubscribedChannels(ctx)
		if err != nil {
			return feedSubLoadedMsg{TabTarget: feedTarget, err: err}
		}
		ids := make([]string, len(channels))
		for i := range channels {
			ids[i] = channels[i].ID
		}
		videos, err := t.backend.GetAllChannelVideos(ctx, ids)
		if err != nil {
			return feedSubLoadedMsg{TabTarget: feedTarget, err: err}
		}
		return feedSubLoadedMsg{TabTarget: feedTarget, videos: videos}
	}
}

// staleLoadCmd loads the latest videos from stale tagged channels — the
// "catch up on channels I forgot about" view. It reads the full channel
// universe, intersects it with the recommended-feed cache (a channel currently
// in the feed isn't stale), keeps the stale tagged set (channels.IsStale), and
// pulls their cached videos, newest first.
func (t Feed) staleLoadCmd() tea.Cmd {
	sf := staleFilter{days: t.staleDays}
	return func() tea.Msg {
		ctx := t.ctx
		chans, err := t.backend.AllChannels(ctx)
		if err != nil {
			return feedStaleLoadedMsg{TabTarget: feedTarget, err: err}
		}
		rec, _ := t.backend.GetFeedCache(ctx, "recommended")
		recIDs := channels.RecFeedIDs(rec)
		now := time.Now()
		var ids []string
		for i := range chans {
			if sf.isStale(chans[i], recIDs[chans[i].ID], now) {
				ids = append(ids, chans[i].ID)
			}
		}
		videos, err := t.backend.GetAllChannelVideos(ctx, ids)
		if err != nil {
			return feedStaleLoadedMsg{TabTarget: feedTarget, err: err}
		}
		feed.SortVideos(videos, feed.SortDate)
		return feedStaleLoadedMsg{TabTarget: feedTarget, videos: videos}
	}
}

func (t Feed) hideVideoCmd(v domain.Video) tea.Cmd {
	return func() tea.Msg {
		if err := t.backend.HideRecVideo(t.ctx, v.ID); err != nil {
			return tuipkg.StatusMsg{Text: "hide: " + err.Error(), IsErr: true}
		}
		return feedHiddenMsg{TabTarget: feedTarget, videoID: v.ID}
	}
}

// channelVideos returns the subset of videos belonging to ch, using the same
// ID-or-name match as feed.RemoveChannelVideos.
func channelVideos(videos []domain.Video, ch domain.Channel) []domain.Video {
	var out []domain.Video
	for _, v := range videos {
		matchID := ch.ID != "" && v.ChannelID == ch.ID
		matchName := ch.Name != "" && strings.EqualFold(v.Channel, ch.Name)
		if matchID || matchName {
			out = append(out, v)
		}
	}
	return out
}
