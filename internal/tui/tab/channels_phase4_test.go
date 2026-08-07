package tab

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// mixedChannels is the standard fixture: one YT sub, one local sub, one
// annotated-but-unsubscribed, one blocked.
func mixedChannels() []domain.Channel {
	return []domain.Channel{
		{ID: "yt", Name: "YTChan", State: domain.SubYT},
		{ID: "loc", Name: "LocChan", State: domain.SubLocal},
		{ID: "none", Name: "NoneChan", State: domain.SubNone, Tags: []string{"kept"}},
		{ID: "blk", Name: "BlkChan", State: domain.SubNone, Blocked: true},
	}
}

func chLoaded(view string, chs []domain.Channel) Channels {
	ch := NewChannels(context.Background(), &fakeBackend{}, testKeys(), false, ChannelsOpts{LatestCount: 3, RefreshMinutes: 60, View: view, HideStale: false, StaleDays: 30})
	m, _ := ch.Update(chsLoadedMsg{chans: chs, latest: map[string]domain.Video{}})
	return m.(Channels)
}

func sortedIDs(chs []domain.Channel) []string {
	ids := make([]string, len(chs))
	for i := range chs {
		ids[i] = chs[i].ID
	}
	sort.Strings(ids)
	return ids
}

func TestChannelsViewFiltering(t *testing.T) {
	cases := []struct {
		view string
		want []string
	}{
		{"subscribed", []string{"loc", "yt"}},
		{"mixed", []string{"loc", "none", "yt"}}, // every non-blocked channel
		{"all", []string{"loc", "none", "yt"}},   // legacy alias for mixed
		{"blocked", []string{"blk"}},
		{"recommended", []string{}}, // no rec-feed videos loaded → nothing
	}
	for _, c := range cases {
		got := sortedIDs(chLoaded(c.view, mixedChannels()).sortedChs)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("view %q: got %v, want %v", c.view, got, c.want)
		}
	}
}

// TestChannelsRecommendedMode folds a recommended-feed channel into the universe
// and asserts the recommended/mixed modes surface it while subscribed/blocked
// exclude it.
func TestChannelsRecommendedMode(t *testing.T) {
	recVideos := []domain.Video{
		{ID: "rv1", ChannelID: "rec", Channel: "RecChan"}, // new rec channel
		{ID: "rv2", ChannelID: "yt", Channel: "YTChan"},   // already subscribed
		{ID: "rv3", ChannelID: "blk", Channel: "BlkChan"}, // blocked → excluded everywhere but blocked
	}
	load := func(view string) Channels {
		ch := NewChannels(context.Background(), &fakeBackend{}, testKeys(), false, ChannelsOpts{LatestCount: 3, RefreshMinutes: 60, View: view, HideStale: false, StaleDays: 30})
		m, _ := ch.Update(chsLoadedMsg{chans: mixedChannels(), recVideos: recVideos, latest: map[string]domain.Video{}})
		return m.(Channels)
	}
	cases := []struct {
		view string
		want []string
	}{
		{"recommended", []string{"rec"}},                // rec-feed, not subscribed, not blocked
		{"mixed", []string{"loc", "none", "rec", "yt"}}, // union, blocked excluded
		{"subscribed", []string{"loc", "yt"}},
		{"blocked", []string{"blk"}},
	}
	for _, c := range cases {
		got := sortedIDs(load(c.view).sortedChs)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("view %q: got %v, want %v", c.view, got, c.want)
		}
	}
}

func TestChannelsLoadsFromAllChannels(t *testing.T) {
	called := false
	be := &fakeBackend{allChannels: func(context.Context) ([]domain.Channel, error) {
		called = true
		return mixedChannels(), nil
	}}
	ch := NewChannels(context.Background(), be, testKeys(), false, ChannelsOpts{LatestCount: 3, RefreshMinutes: 60, View: "all", HideStale: false, StaleDays: 30})
	msg := runCmd(ch.chsLoadCmd())
	loaded, ok := msg.(chsLoadedMsg)
	if !ok {
		t.Fatalf("want chsLoadedMsg, got %#v", msg)
	}
	if !called {
		t.Error("chsLoadCmd must call AllChannels, not GetSubscribedChannels")
	}
	if len(loaded.chans) != 4 {
		t.Fatalf("want 4 channels loaded, got %d", len(loaded.chans))
	}
}

func TestChannelsBlockTogglesAndEmitsMsg(t *testing.T) {
	c := chLoaded("subscribed", mixedChannels())
	// cursor on first subscribed row (loc or yt after sort). Force a known target
	// by moving cursor to 0; whichever it is must be a subscribed channel.
	target := c.sortedChs[0]

	m, cmd := c.Update(tea.KeyPressMsg{Text: "X"}) // Block
	got := m.(Channels)

	updated, ok := got.subs.ByID(target.ID)
	if !ok || !updated.Blocked || updated.State != domain.SubNone {
		t.Fatalf("optimistic block: got %+v, want blocked+none", updated)
	}
	// Row must disappear from the subscribed view.
	for _, ch := range got.sortedChs {
		if ch.ID == target.ID {
			t.Error("blocked channel still visible in subscribed view")
		}
	}
	bm, ok := runCmd(cmd).(tuipkg.BlockChannelMsg)
	if !ok || !bm.Block || bm.Channel.ID != target.ID {
		t.Fatalf("want BlockChannelMsg{Block:true, id:%s}, got %#v", target.ID, runCmd(cmd))
	}
	// Message must carry the pre-transition value for rollback.
	if bm.Channel.Blocked || bm.Channel.State != domain.SubYT && bm.Channel.State != domain.SubLocal {
		t.Errorf("BlockChannelMsg.Channel should be pre-transition, got %+v", bm.Channel)
	}
}

func TestChannelsUnblockInBlockedView(t *testing.T) {
	c := chLoaded("blocked", mixedChannels())
	if len(c.sortedChs) != 1 || c.sortedChs[0].ID != "blk" {
		t.Fatalf("blocked view setup: got %v", sortedIDs(c.sortedChs))
	}
	m, cmd := c.Update(tea.KeyPressMsg{Text: "X"}) // toggle → unblock
	got := m.(Channels)

	updated, _ := got.subs.ByID("blk")
	if updated.Blocked {
		t.Error("channel should be unblocked optimistically")
	}
	if len(got.sortedChs) != 0 {
		t.Error("unblocked channel should leave the blocked view")
	}
	bm := runCmd(cmd).(tuipkg.BlockChannelMsg)
	if bm.Block {
		t.Errorf("want unblock (Block=false), got %#v", bm)
	}
}

func TestChannelsBlockResultRevertsOnError(t *testing.T) {
	c := chLoaded("subscribed", mixedChannels())
	target := c.sortedChs[0]
	m, _ := c.Update(tea.KeyPressMsg{Text: "X"})
	c = m.(Channels)

	// Backend reports failure — original channel value comes back for rollback.
	m2, _ := c.Update(tuipkg.BlockChannelResultMsg{Channel: target, Block: true, Err: errors.New("boom")})
	got := m2.(Channels)

	reverted, _ := got.subs.ByID(target.ID)
	if reverted.Blocked || reverted.State == domain.SubNone {
		t.Fatalf("revert failed: got %+v, want original subscribed state", reverted)
	}
	// Row reappears in the subscribed view.
	found := false
	for _, ch := range got.sortedChs {
		if ch.ID == target.ID {
			found = true
		}
	}
	if !found {
		t.Error("reverted channel should reappear in subscribed view")
	}
}

func TestChannelsViewPickerSelects(t *testing.T) {
	c := chLoaded("subscribed", mixedChannels())

	m, _ := c.Update(tea.KeyPressMsg{Text: "M"}) // PanelMode → open picker
	c = m.(Channels)
	if !c.picker.isOpen() {
		t.Fatal("PanelMode should open the view picker")
	}
	if c.picker.selection() != modeIndex(channelModes, srcSubscribed) {
		t.Errorf("picker should start on the active view, got %d", c.picker.selection())
	}
	if !c.InterceptsInput() {
		t.Error("open picker must intercept input")
	}

	// From Subscribed (index 1) move down twice to "Blocked" (index 3) and confirm.
	c, _ = updateChannels(c, tea.KeyPressMsg{Text: "j"})
	c, _ = updateChannels(c, tea.KeyPressMsg{Text: "j"})
	c, _ = updateChannels(c, tea.KeyPressMsg{Text: "enter"})

	if c.picker.isOpen() {
		t.Error("Enter should close the picker")
	}
	if c.view != srcBlocked {
		t.Errorf("view should be blocked, got %d", c.view)
	}
	if ids := sortedIDs(c.sortedChs); !reflect.DeepEqual(ids, []string{"blk"}) {
		t.Errorf("blocked view after pick: got %v", ids)
	}
}

func TestChannelsViewPickerEscCancels(t *testing.T) {
	c := chLoaded("subscribed", mixedChannels())
	c, _ = updateChannels(c, tea.KeyPressMsg{Text: "M"})
	c, _ = updateChannels(c, tea.KeyPressMsg{Text: "j"}) // move selection
	c, _ = updateChannels(c, tea.KeyPressMsg{Text: "esc"})
	if c.picker.isOpen() {
		t.Error("Esc should close the picker")
	}
	if c.view != srcSubscribed {
		t.Error("Esc must not change the active view")
	}
}

// TestChannelsMaterializesRecChannelOnTag: tagging a rec-feed channel (no stored
// row) upserts a state=none row so the annotation persists, then sets the tags.
func TestChannelsMaterializesRecChannelOnTag(t *testing.T) {
	var added *domain.Channel
	var taggedID string
	be := &fakeBackend{
		addSubscribedChan: func(_ context.Context, ch domain.Channel) error { c := ch; added = &c; return nil },
		setChannelTags:    func(_ context.Context, id string, _ []string) error { taggedID = id; return nil },
	}
	c := NewChannels(context.Background(), be, testKeys(), false, ChannelsOpts{LatestCount: 3, RefreshMinutes: 60, View: "recommended", HideStale: false, StaleDays: 30})
	recCh := domain.Channel{ID: "rec", Name: "RecChan", URL: "https://www.youtube.com/channel/rec", State: domain.SubNone}

	runCmd(c.chSetTagsCmd(recCh, []string{"science"}))

	if added == nil || added.ID != "rec" || added.State != domain.SubNone || added.Blocked {
		t.Fatalf("rec channel should be materialized as state=none, got %+v", added)
	}
	if taggedID != "rec" {
		t.Errorf("SetChannelTags should target rec, got %q", taggedID)
	}
}

// TestChannelsDoesNotMaterializeSubscribed: a subscribed channel already has a
// row, so tagging must not re-upsert it (which would risk a state downgrade).
func TestChannelsDoesNotMaterializeSubscribed(t *testing.T) {
	materialized := false
	be := &fakeBackend{addSubscribedChan: func(context.Context, domain.Channel) error { materialized = true; return nil }}
	c := NewChannels(context.Background(), be, testKeys(), false, ChannelsOpts{LatestCount: 3, RefreshMinutes: 60, View: "subscribed", HideStale: false, StaleDays: 30})
	subCh := domain.Channel{ID: "yt", Name: "YTChan", State: domain.SubYT}

	runCmd(c.chSetAliasCmd(subCh, "alias", "Alias set"))

	if materialized {
		t.Error("subscribed channel already has a row; must not be re-materialized")
	}
}

func updateChannels(c Channels, msg tea.Msg) (Channels, tea.Cmd) {
	m, cmd := c.Update(msg)
	return m.(Channels), cmd
}
