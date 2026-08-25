package tab

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// The block key used to route to HideRecVideo(channelID) — a channel ID written
// into the hidden-*video* table, which matched nothing and left the rows on
// screen. It must emit the guarded block transition instead, and drop the
// channel's rows on the keypress rather than on the next fetch.
func TestFeedBlockChannelKeyEmitsBlockAndRemovesRows(t *testing.T) {
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{StaleDays: 30})
	f, _ = updateFeed(f, sized(80, 24))
	f, _ = updateFeed(f, feedRecCacheMsg{videos: []domain.Video{
		{ID: "v1", ChannelID: "c1", Channel: "Chan"},
		{ID: "v2", ChannelID: "c1", Channel: "Chan"},
		{ID: "v3", ChannelID: "c2", Channel: "Other"},
	}})

	f, cmd := updateFeed(f, tea.KeyPressMsg{Text: "B"})
	msg := runCmd(cmd)
	bm, ok := msg.(tuipkg.BlockChannelMsg)
	if !ok {
		t.Fatalf("want BlockChannelMsg, got %#v", msg)
	}
	if bm.Channel.ID != "c1" || !bm.Block {
		t.Errorf("want block of c1, got %#v", bm)
	}
	if f.feed.Len() != 1 {
		t.Fatalf("channel not removed on keypress: feed len = %d, want 1", f.feed.Len())
	}
}

// A failed block puts every removed row back, in all three sources.
func TestFeedBlockRestoresRowsOnError(t *testing.T) {
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{Mode: "mixed", StaleDays: 30})
	f, _ = updateFeed(f, sized(80, 24))
	f, _ = updateFeed(f, feedRecCacheMsg{videos: []domain.Video{{ID: "v1", ChannelID: "c1", Channel: "Chan"}}})
	f, _ = updateFeed(f, feedSubLoadedMsg{videos: []domain.Video{{ID: "v2", ChannelID: "c1", Channel: "Chan"}}})
	if f.feed.Len() != 2 {
		t.Fatalf("setup: feed len = %d, want 2", f.feed.Len())
	}

	f, _ = updateFeed(f, tea.KeyPressMsg{Text: "B"})
	if f.feed.Len() != 0 {
		t.Fatalf("block should clear both sources: feed len = %d, want 0", f.feed.Len())
	}

	f, _ = updateFeed(f, tuipkg.BlockChannelResultMsg{
		Channel: domain.Channel{ID: "c1", Name: "Chan"}, Block: true, Err: errors.New("nope"),
	})
	if f.feed.Len() != 2 {
		t.Fatalf("failed block should restore rows: feed len = %d, want 2", f.feed.Len())
	}
}

// The result is broadcast to every tab, so a block started on the Channels tab
// must clear the feed too — without waiting for the next fetch.
func TestFeedBlockResultFromAnotherTabRemovesRows(t *testing.T) {
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{StaleDays: 30})
	f, _ = updateFeed(f, sized(80, 24))
	f, _ = updateFeed(f, feedRecCacheMsg{videos: []domain.Video{
		{ID: "v1", ChannelID: "c1", Channel: "Chan"},
		{ID: "v2", ChannelID: "c2", Channel: "Other"},
	}})

	// No prior keypress on this tab: nothing is escrowed in pendingBlock.
	f, _ = updateFeed(f, tuipkg.BlockChannelResultMsg{
		Channel: domain.Channel{ID: "c1", Name: "Chan"}, Block: true,
	})
	if f.feed.Len() != 1 {
		t.Fatalf("broadcast block not applied: feed len = %d, want 1", f.feed.Len())
	}
}

// An unblock broadcast must not resurrect anything or drop unrelated rows.
func TestFeedUnblockResultLeavesFeedAlone(t *testing.T) {
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{StaleDays: 30})
	f, _ = updateFeed(f, sized(80, 24))
	f, _ = updateFeed(f, feedRecCacheMsg{videos: []domain.Video{{ID: "v1", ChannelID: "c1", Channel: "Chan"}}})

	f, _ = updateFeed(f, tuipkg.BlockChannelResultMsg{Channel: domain.Channel{ID: "c1"}, Block: false})
	if f.feed.Len() != 1 {
		t.Fatalf("unblock should leave the feed untouched: feed len = %d, want 1", f.feed.Len())
	}
}

// A row with no channel ID can't be blocked: the transition would write a
// blocklist row keyed on "" that matches nothing. Report it instead.
func TestFeedBlockWithoutChannelIDReportsError(t *testing.T) {
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{StaleDays: 30})
	f, _ = updateFeed(f, sized(80, 24))
	f, _ = updateFeed(f, feedRecCacheMsg{videos: []domain.Video{{ID: "v1", Title: "No channel"}}})

	f, cmd := updateFeed(f, tea.KeyPressMsg{Text: "B"})
	sm, ok := runCmd(cmd).(tuipkg.StatusMsg)
	if !ok || !sm.IsErr {
		t.Fatalf("want an error StatusMsg, got %#v", runCmd(cmd))
	}
	if f.feed.Len() != 1 {
		t.Errorf("row should stay: feed len = %d, want 1", f.feed.Len())
	}
}
