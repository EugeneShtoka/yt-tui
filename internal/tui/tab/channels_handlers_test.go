package tab

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// chAt returns the channel at sorted index i from a loaded subscribed view.
func subscribedChannels(t *testing.T) Channels {
	t.Helper()
	c := chLoaded("subscribed", mixedChannels())
	if len(c.sortedChs) == 0 {
		t.Fatal("expected subscribed channels in fixture")
	}
	return c
}

// TestChannelsDrillIntoEntersVideoPane: Enter on a channel row switches to the
// video pane, records the active channel, and issues a fetch command.
func TestChannelsDrillIntoEntersVideoPane(t *testing.T) {
	c := subscribedChannels(t)
	target := c.sortedChs[0]

	m, cmd := c.Update(tea.KeyPressMsg{Text: "enter"}) // DrillDown
	got := m.(Channels)

	if got.pane != 1 {
		t.Errorf("drill should switch to video pane (1), got %d", got.pane)
	}
	if got.activeChID != target.ID {
		t.Errorf("activeChID = %q, want %q", got.activeChID, target.ID)
	}
	if got.chVidLoad != srcLoading {
		t.Errorf("drill into a fresh channel should set srcLoading, got %v", got.chVidLoad)
	}
	if cmd == nil {
		t.Error("drill should issue a fetch command")
	}
}

// TestChannelsDrillReusesLoadedVideos: re-opening the already-active channel
// that still has videos loaded does not re-enter the loading state.
func TestChannelsDrillReusesLoadedVideos(t *testing.T) {
	c := subscribedChannels(t)
	target := c.sortedChs[0]
	c.activeChID = target.ID
	c.chVideos = vids(3)
	c.refreshInterval = 0 // 0 = always-stale; exercise the refresh branch below

	m, _ := c.drillIntoChannel(target)
	got := m.(Channels)
	if got.chVidLoad == srcLoading {
		t.Error("re-drilling a channel with cached videos must not show the initial loading state")
	}
}

// TestChannelsBeginEditAlias enters alias-edit mode, focuses the input, and
// pre-fills it with the channel's display name.
func TestChannelsBeginEditAlias(t *testing.T) {
	c := subscribedChannels(t)
	c.listNav.GotoRow(0)
	target := c.sortedChs[0]

	m, _ := c.Update(tea.KeyPressMsg{Text: "A"}) // RenameChannel
	got := m.(Channels)

	if got.editMode != chEditAlias {
		t.Errorf("editMode = %d, want chEditAlias", got.editMode)
	}
	if !got.editInput.Focused() {
		t.Error("alias edit should focus the input")
	}
	if got.editInput.Value() != target.DisplayName() {
		t.Errorf("alias input = %q, want display name %q", got.editInput.Value(), target.DisplayName())
	}
	if !got.InterceptsInput() {
		t.Error("an open edit input must intercept input")
	}
}

// TestChannelsBeginEditTags enters tag-edit mode pre-filled with the current
// comma-joined tags.
func TestChannelsBeginEditTags(t *testing.T) {
	c := chLoaded("mixed", mixedChannels())
	// "none" carries tags:["kept"] in the fixture — find its row.
	var idx int
	for i, ch := range c.sortedChs {
		if ch.ID == "none" {
			idx = i
		}
	}
	c.listNav.GotoRow(idx)

	m, _ := c.Update(tea.KeyPressMsg{Text: "T"}) // TagChannel
	got := m.(Channels)
	if got.editMode != chEditTags {
		t.Fatalf("editMode = %d, want chEditTags", got.editMode)
	}
	if got.editInput.Value() != "kept" {
		t.Errorf("tag input = %q, want %q", got.editInput.Value(), "kept")
	}
}

// TestChannelsUnsubscribeRemovesAndEmits: 'u' on a channel removes it from the
// local set optimistically and emits an UnsubscribeMsg carrying that channel.
func TestChannelsUnsubscribeRemovesAndEmits(t *testing.T) {
	c := subscribedChannels(t)
	c.listNav.GotoRow(0)
	target := c.sortedChs[0]

	m, cmd := c.Update(tea.KeyPressMsg{Text: "u"}) // Unsubscribe
	got := m.(Channels)

	for _, ch := range got.sortedChs {
		if ch.ID == target.ID {
			t.Errorf("unsubscribed channel %q still present in the list", target.ID)
		}
	}
	um, ok := runCmd(cmd).(tuipkg.UnsubscribeMsg)
	if !ok || um.Channel.ID != target.ID {
		t.Fatalf("want UnsubscribeMsg for %q, got %#v", target.ID, runCmd(cmd))
	}
}

// TestChannelsUnsubscribeFromVideoPaneReturnsToList: unsubscribing while drilled
// into the video pane drops back to the channel list (returnToList path).
func TestChannelsUnsubscribeFromVideoPaneReturnsToList(t *testing.T) {
	c := subscribedChannels(t)
	c.listNav.GotoRow(0)
	c.pane = 1 // drilled into the video list

	m, cmd := c.handleKeyVideos(tea.KeyPressMsg{Text: "u"})
	got := m.(Channels)

	if got.pane != 0 {
		t.Errorf("unsubscribe from video pane should return to list pane (0), got %d", got.pane)
	}
	if _, ok := runCmd(cmd).(tuipkg.UnsubscribeMsg); !ok {
		t.Errorf("want UnsubscribeMsg, got %#v", runCmd(cmd))
	}
}

// TestChannelsVideoPaneRefresh: the Refresh key in the video pane triggers a
// refresh command and marks the channel's videos as fetching. With videos
// already loaded, that fetch is a refresh (data stays visible).
func TestChannelsVideoPaneRefresh(t *testing.T) {
	c := subscribedChannels(t)
	c.pane = 1
	c.activeChID = "yt"
	c.chVideos = []domain.Video{{ID: "v1", Title: "V1"}}
	c.chVidLoad = srcLoaded

	m, cmd := c.handleKeyVideos(tea.KeyPressMsg{Text: "r"}) // Refresh
	got := m.(Channels)
	if got.chVidLoad != srcRefreshing {
		t.Errorf("Refresh with videos loaded should set srcRefreshing, got %v", got.chVidLoad)
	}
	if cmd == nil {
		t.Error("Refresh should issue a refresh command")
	}
}

// TestChannelsVideoPaneRefreshNoActiveChannelNoops: with no active channel the
// refresh key does nothing (guards the activeChID=="" branch).
func TestChannelsVideoPaneRefreshNoActiveChannelNoops(t *testing.T) {
	c := NewChannels(context.Background(), &fakeBackend{}, testKeys(), false, ChannelsOpts{LatestCount: 3, RefreshMinutes: 60, View: "subscribed", HideStale: false, StaleDays: 30})
	c.pane = 1
	c.activeChID = ""
	if _, cmd := c.handleKeyVideos(tea.KeyPressMsg{Text: "r"}); cmd != nil {
		t.Error("refresh with no active channel should be a no-op")
	}
}

// TestChannelsDrillBackFromVideoPane: the Back key in the video pane returns to
// the channel list without emitting a command.
func TestChannelsDrillBackFromVideoPane(t *testing.T) {
	c := subscribedChannels(t)
	c.pane = 1
	c.activeChID = "yt"
	c.chVideos = vids(3)

	m, _ := c.handleKeyVideos(tea.KeyPressMsg{Text: "h"}) // Back
	if got := m.(Channels); got.pane != 0 {
		t.Errorf("Back should return to list pane, got pane %d", got.pane)
	}
}
