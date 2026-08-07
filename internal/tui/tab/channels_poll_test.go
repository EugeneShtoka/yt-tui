package tab

import (
	"context"
	"testing"

	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// newDrilledChannels returns a Channels sitting in the video pane (pane 1) for
// channel c1 showing `shown` videos — the state the poll operates on.
func newDrilledChannels(shown int) Channels {
	c := NewChannels(context.Background(), &fakeBackend{}, testKeys(), false, ChannelsOpts{LatestCount: 3, RefreshMinutes: 60, View: "subscribed", HideStale: false, StaleDays: 30})
	c.pane = 1
	c.activeChID = "c1"
	c.chVideos = vids(shown)
	return c
}

func vids(n int) []domain.Video {
	out := make([]domain.Video, n)
	for i := range out {
		out[i] = domain.Video{ID: string(rune('a' + i))}
	}
	return out
}

// TestChannelsPollGrowsList: a poll that surfaces more videos than are shown
// grows the open list in place.
func TestChannelsPollGrowsList(t *testing.T) {
	c := newDrilledChannels(1)
	updated, _ := c.onChVideosPolled(chVideosPolledMsg{channelID: "c1", videos: vids(5)})
	got := updated.(Channels)
	if len(got.chVideos) != 5 {
		t.Errorf("chVideos = %d, want 5 (list should grow)", len(got.chVideos))
	}
}

// TestChannelsPollNoGrowthLeavesListUntouched: a poll returning the same count
// (crawl between pages) must not shrink or churn the list.
func TestChannelsPollNoGrowthLeavesListUntouched(t *testing.T) {
	c := newDrilledChannels(3)
	updated, _ := c.onChVideosPolled(chVideosPolledMsg{channelID: "c1", videos: vids(3)})
	if got := updated.(Channels); len(got.chVideos) != 3 {
		t.Errorf("chVideos = %d, want 3 (unchanged)", len(got.chVideos))
	}
}

// TestChannelsPollIgnoresStaleChannel: a poll for a channel that is no longer the
// active one (user navigated away) is dropped without mutating the list.
func TestChannelsPollIgnoresStaleChannel(t *testing.T) {
	c := newDrilledChannels(3)
	updated, _ := c.onChVideosPolled(chVideosPolledMsg{channelID: "other", videos: vids(9)})
	if got := updated.(Channels); len(got.chVideos) != 3 {
		t.Errorf("chVideos = %d, want 3 (stale poll must not mutate list)", len(got.chVideos))
	}
}

// TestChannelsPollTickInVideoPaneFetches: a PollTickMsg while drilled into a
// channel issues a DB read (non-nil cmd) so the open list can grow.
func TestChannelsPollTickInVideoPaneFetches(t *testing.T) {
	c := newDrilledChannels(3)
	_, cmd := c.Update(tuipkg.PollTickMsg{})
	if cmd == nil {
		t.Error("expected a poll fetch cmd while drilled into a channel")
	}
}

// TestChannelsPollTickInListPaneRefreshes: a PollTickMsg in the channel list
// pane reloads the universe (non-nil cmd) so latest-video columns update.
func TestChannelsPollTickInListPaneRefreshes(t *testing.T) {
	c := NewChannels(context.Background(), &fakeBackend{}, testKeys(), false, ChannelsOpts{LatestCount: 3, RefreshMinutes: 60, View: "subscribed", HideStale: false, StaleDays: 30})
	c.pane = 0
	_, cmd := c.Update(tuipkg.PollTickMsg{})
	if cmd == nil {
		t.Error("expected a list-refresh cmd in the channel list pane")
	}
}
