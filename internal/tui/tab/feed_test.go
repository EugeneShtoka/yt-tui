package tab

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

func updateFeed(f Feed, msg tea.Msg) (Feed, tea.Cmd) {
	m, cmd := f.Update(msg)
	return m.(Feed), cmd
}

// ── mode + title ──────────────────────────────────────────────────────────────

func TestFeedDefaultMode(t *testing.T) {
	cases := []struct {
		cfg  string
		want feedMode
	}{
		{"recommended", feedRecommended},
		{"subscribed", feedSubscribed},
		{"mixed", feedMixed},
		{"garbage", feedRecommended}, // invalid falls back to recommended
	}
	for _, c := range cases {
		f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{Mode: c.cfg, StaleDays: 30})
		if f.mode != c.want {
			t.Errorf("NewFeed(context.Background(), %q).mode = %d, want %d", c.cfg, f.mode, c.want)
		}
		// The mode is no longer surfaced in the tab title (it shows in the picker).
		if got := f.Title(); got != "Feed" {
			t.Errorf("NewFeed(context.Background(), %q).Title() = %q, want %q", c.cfg, got, "Feed")
		}
	}
}

// The mode picker (PanelMode) opens, intercepts input, and applying a choice
// switches the mode (and thus the tab title).
func TestFeedModePickerSwitchesMode(t *testing.T) {
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{Mode: "recommended", StaleDays: 30})
	f, _ = updateFeed(f, sized(80, 24))

	f, _ = updateFeed(f, tea.KeyPressMsg{Text: "M"}) // PanelMode → open picker
	if !f.picker.isOpen() {
		t.Fatal("PanelMode should open the mode picker")
	}
	if !f.InterceptsInput() {
		t.Error("open picker must intercept input")
	}
	if f.picker.selection() != int(feedRecommended) {
		t.Errorf("picker should start on the active mode, got %d", f.picker.selection())
	}
	// The open picker renders as a centered popup box over the list, within width.
	frame := f.View().Content
	for _, want := range []string{"Mode:", "Recommended", "Subscribed", "Mixed"} {
		if !strings.Contains(frame, want) {
			t.Errorf("picker frame missing %q:\n%s", want, frame)
		}
	}
	assertNoLineOverflows(t, frame, 80)

	f, _ = updateFeed(f, tea.KeyPressMsg{Text: "j"})     // Down → Subscribed
	f, _ = updateFeed(f, tea.KeyPressMsg{Text: "enter"}) // commit
	if f.picker.isOpen() {
		t.Error("Enter should close the picker")
	}
	if f.mode != feedSubscribed {
		t.Fatalf("mode = %d, want feedSubscribed", f.mode)
	}
	if got := f.Title(); got != "Feed" {
		t.Errorf("title after switch = %q, want Feed", got)
	}
}

func TestFeedPickerEscCancels(t *testing.T) {
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{Mode: "recommended", StaleDays: 30})
	f, _ = updateFeed(f, tea.KeyPressMsg{Text: "M"})
	f, _ = updateFeed(f, tea.KeyPressMsg{Text: "j"})   // move selection
	f, _ = updateFeed(f, tea.KeyPressMsg{Text: "esc"}) // cancel
	if f.picker.isOpen() {
		t.Error("Esc should close the picker")
	}
	if f.mode != feedRecommended {
		t.Errorf("Esc must not change the mode, got %d", f.mode)
	}
}

// ── data loading + projection ──────────────────────────────────────────────────

func TestFeedRecCacheMsgPopulates(t *testing.T) {
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{Mode: "recommended", StaleDays: 30})
	f, _ = updateFeed(f, sized(80, 24))
	f, _ = updateFeed(f, feedRecCacheMsg{videos: []domain.Video{{ID: "v1"}, {ID: "v2"}}})
	if n := f.feed.Len(); n != 2 {
		t.Fatalf("want feed len 2, got %d", n)
	}
}

func TestFeedSubscribedModePopulates(t *testing.T) {
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{Mode: "subscribed", StaleDays: 30})
	f, _ = updateFeed(f, sized(80, 24))
	f, _ = updateFeed(f, feedSubLoadedMsg{videos: []domain.Video{{ID: "v1"}, {ID: "v2"}}})
	if n := f.feed.Len(); n != 2 {
		t.Fatalf("want feed len 2, got %d", n)
	}
}

// Mixed mode shows the union of the recommended and subscribed sources,
// deduplicated by video ID.
func TestFeedMixedDedupsByID(t *testing.T) {
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{Mode: "mixed", StaleDays: 30})
	f, _ = updateFeed(f, sized(80, 24))
	f, _ = updateFeed(f, feedRecCacheMsg{videos: []domain.Video{{ID: "v1"}, {ID: "v2"}}})
	f, _ = updateFeed(f, feedSubLoadedMsg{videos: []domain.Video{{ID: "v2"}, {ID: "v3"}}})

	if n := f.feed.Len(); n != 3 {
		t.Fatalf("mixed feed len = %d, want 3 (v1,v2,v3 deduped)", n)
	}
	seen := map[string]int{}
	for _, v := range f.feed.Videos() {
		seen[v.ID]++
	}
	for _, id := range []string{"v1", "v2", "v3"} {
		if seen[id] != 1 {
			t.Errorf("video %q appears %d times, want exactly 1", id, seen[id])
		}
	}
}

// Switching from a single-source mode to mixed keeps the already-loaded source
// (no data is lost across a mode switch).
func TestFeedSwitchToMixedKeepsLoadedSource(t *testing.T) {
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{Mode: "recommended", StaleDays: 30})
	f, _ = updateFeed(f, sized(80, 24))
	f, _ = updateFeed(f, feedRecCacheMsg{videos: []domain.Video{{ID: "v1"}, {ID: "v2"}}})

	// Open picker, move to Mixed (index 2), commit.
	f, _ = updateFeed(f, tea.KeyPressMsg{Text: "M"})
	f, _ = updateFeed(f, tea.KeyPressMsg{Text: "j"})
	f, _ = updateFeed(f, tea.KeyPressMsg{Text: "j"})
	f, _ = updateFeed(f, tea.KeyPressMsg{Text: "enter"})
	if f.mode != feedMixed {
		t.Fatalf("mode = %d, want feedMixed", f.mode)
	}
	// The recommended source survives; subscribed is empty (nop backend).
	if n := f.feed.Len(); n != 2 {
		t.Fatalf("mixed feed len = %d, want 2 (rec kept)", n)
	}
}

// ── hide flow (H-6 regression, migrated from Recommended) ───────────────────────

func TestFeedHideSuccessEmitsHiddenMsg(t *testing.T) {
	var hiddenID string
	fb := &fakeBackend{hideRecVideo: func(_ context.Context, id string) error { hiddenID = id; return nil }}
	f := NewFeed(context.Background(), fb, testKeys(), false, FeedOpts{Mode: "recommended", StaleDays: 30})
	f, _ = updateFeed(f, sized(80, 24))
	f, _ = updateFeed(f, feedRecCacheMsg{videos: []domain.Video{{ID: "v1", Title: "T"}}})

	_, cmd := f.Update(tea.KeyPressMsg{Text: "b"}) // HideVideo
	msg := runCmd(cmd)
	if _, ok := msg.(feedHiddenMsg); !ok {
		t.Fatalf("want feedHiddenMsg on successful hide, got %T", msg)
	}
	if hiddenID != "v1" {
		t.Errorf("backend hidden id = %q, want v1", hiddenID)
	}
}

func TestFeedHideFailureSurfacesErrorAndKeepsRow(t *testing.T) {
	fb := &fakeBackend{hideRecVideo: func(context.Context, string) error { return errors.New("db down") }}
	f := NewFeed(context.Background(), fb, testKeys(), false, FeedOpts{Mode: "recommended", StaleDays: 30})
	f, _ = updateFeed(f, sized(80, 24))
	f, _ = updateFeed(f, feedRecCacheMsg{videos: []domain.Video{{ID: "v1", Title: "T"}}})

	before := f.feed.Len()
	_, cmd := f.Update(tea.KeyPressMsg{Text: "b"})
	sm, ok := runCmd(cmd).(tuipkg.StatusMsg)
	if !ok || !sm.IsErr {
		t.Fatalf("want error StatusMsg on failed hide")
	}
	if f.feed.Len() != before {
		t.Errorf("row removed on failed hide (before=%d, after=%d)", before, f.feed.Len())
	}
}

// feedHiddenMsg is the only path that removes a row (after the write succeeded).
func TestFeedHiddenMsgRemovesRow(t *testing.T) {
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{Mode: "recommended", StaleDays: 30})
	f, _ = updateFeed(f, sized(80, 24))
	f, _ = updateFeed(f, feedRecCacheMsg{videos: []domain.Video{{ID: "v1"}, {ID: "v2"}}})
	f, _ = updateFeed(f, feedHiddenMsg{videoID: "v1"})
	if f.feed.Len() != 1 {
		t.Fatalf("want 1 video after hide, got %d", f.feed.Len())
	}
}

// Hiding is a recommended-source action: in subscribed mode the key is a no-op.
func TestFeedHideIgnoredInSubscribedMode(t *testing.T) {
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{Mode: "subscribed", StaleDays: 30})
	f, _ = updateFeed(f, sized(80, 24))
	f, _ = updateFeed(f, feedSubLoadedMsg{videos: []domain.Video{{ID: "v1"}}})
	_, cmd := f.Update(tea.KeyPressMsg{Text: "b"})
	if runCmd(cmd) != nil {
		t.Error("HideVideo should be a no-op in subscribed mode")
	}
}

// ── unsubscribe flow (migrated from Subscriptions) ──────────────────────────────

func TestFeedUnsubscribeOptimisticAndRestoreOnError(t *testing.T) {
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{Mode: "subscribed", StaleDays: 30})
	f, _ = updateFeed(f, sized(80, 24))
	f, _ = updateFeed(f, feedSubLoadedMsg{videos: []domain.Video{
		{ID: "v1", ChannelID: "c1", Channel: "Chan"},
		{ID: "v2", ChannelID: "c2", Channel: "Other"},
	}})

	// Unsubscribe from the selected (first) video's channel.
	f, cmd := updateFeed(f, tea.KeyPressMsg{Text: "u"})
	msg := runCmd(cmd)
	um, ok := msg.(tuipkg.UnsubscribeMsg)
	if !ok || um.Channel.ID != "c1" {
		t.Fatalf("want UnsubscribeMsg for c1, got %#v", msg)
	}
	if f.feed.Len() != 1 {
		t.Fatalf("channel not optimistically removed: feed len = %d, want 1", f.feed.Len())
	}

	// A failed result restores the removed channel's videos.
	f, _ = updateFeed(f, tuipkg.UnsubscribeResultMsg{Channel: domain.Channel{ID: "c1"}, Err: errors.New("nope")})
	if f.feed.Len() != 2 {
		t.Fatalf("failed unsubscribe should restore videos: feed len = %d, want 2", f.feed.Len())
	}
}

// ── addressing ─────────────────────────────────────────────────────────────────

func TestFeedLoadCmdsAddressedToOwnerTab(t *testing.T) {
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{Mode: "mixed", StaleDays: 30})
	for name, cmd := range map[string]tea.Cmd{"sub": f.subLoadCmd(), "recFetch": f.recFetchCmd()} {
		am, ok := cmd().(tuipkg.TabAddressedMsg)
		if !ok {
			t.Fatalf("%s: message does not implement TabAddressedMsg", name)
		}
		if am.TargetTab() != tuipkg.TabFeed {
			t.Errorf("%s: TargetTab() = %v, want TabFeed", name, am.TargetTab())
		}
	}
}
