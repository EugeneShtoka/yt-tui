package ui

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// channelsKeys binds the multi-pane Channels keys (drill/back/toggle) that the
// shared testListKeys omits.
func channelsKeys() keyMap {
	return buildKeyMap(config.KeyBindings{
		Up:         "up",
		Down:       "down",
		PageUp:     "pgup",
		PageDown:   "pgdown",
		Back:       "left",
		Right:      "right",
		DrillDown:  "enter",
		Close:      "esc",
		ToggleMode: "t",
		Play:       "p",
		CopyURL:    "y",
	})
}

func channelsCtx() viewCtx {
	return viewCtx{
		keys:        channelsKeys(),
		pageSize:    10,
		circular:    false,
		chSorted:    sampleChannels(),
		chTagItems:  []string{"news", "music"},
		chTagVideos: sampleVideos(),
		subChVideos: sampleVideos(),
	}
}

func TestChannelsFlatDownMovesCursor(t *testing.T) {
	v := channelsView{}
	if in := v.update(tea.KeyMsg{Type: tea.KeyDown}, channelsCtx()); in != nil {
		t.Fatalf("nav key should be handled in-view, got intent %T", in)
	}
	if v.cursor != 1 {
		t.Errorf("Down: cursor=%d, want 1", v.cursor)
	}
}

func TestChannelsFlatContextIsChannelList(t *testing.T) {
	if got := (channelsView{}).context(channelsCtx()); got != CtxChannelList {
		t.Errorf("flat pane 0 context=%v, want CtxChannelList", got)
	}
}

func TestChannelsToggleModeSwitchesToTags(t *testing.T) {
	v := channelsView{cursor: 3, pane: 1, tagCursor: 2}
	in := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")}, channelsCtx())
	if in != nil {
		t.Fatalf("ToggleMode is pure view state, got intent %T", in)
	}
	if !v.tagsMode {
		t.Error("ToggleMode did not enable tagsMode")
	}
	if v.pane != 0 || v.tagCursor != 0 {
		t.Errorf("ToggleMode: pane=%d tagCursor=%d, want 0/0", v.pane, v.tagCursor)
	}
}

func TestChannelsTagsDrillDownSelectsTag(t *testing.T) {
	v := channelsView{tagsMode: true, tagCursor: 1}
	in := v.update(tea.KeyMsg{Type: tea.KeyRight}, channelsCtx())
	if in != nil {
		t.Fatalf("tag drill-in is pure view state, got intent %T", in)
	}
	if v.pane != 1 {
		t.Errorf("drill-in: pane=%d, want 1", v.pane)
	}
	if v.tagSel != "music" {
		t.Errorf("drill-in: tagSel=%q, want music", v.tagSel)
	}
}

func TestChannelsTagsContexts(t *testing.T) {
	ctx := channelsCtx()
	if got := (channelsView{tagsMode: true, pane: 0}).context(ctx); got != CtxTagList {
		t.Errorf("tags pane 0 context=%v, want CtxTagList", got)
	}
	if got := (channelsView{tagsMode: true, pane: 1}).context(ctx); got != CtxVideoList {
		t.Errorf("tags pane 1 context=%v, want CtxVideoList", got)
	}
}

func TestChannelsVideoPaneNavAndBack(t *testing.T) {
	v := channelsView{pane: 1}
	// Down moves the video cursor (its own vidCursor, not the channel cursor).
	v.update(tea.KeyMsg{Type: tea.KeyDown}, channelsCtx())
	if v.vidCursor != 1 {
		t.Errorf("video pane Down: vidCursor=%d, want 1", v.vidCursor)
	}
	if v.cursor != 0 {
		t.Errorf("video pane Down moved channel cursor to %d, want 0", v.cursor)
	}
	// Left returns to the channel list.
	v.update(tea.KeyMsg{Type: tea.KeyLeft}, channelsCtx())
	if v.pane != 0 {
		t.Errorf("Left: pane=%d, want 0", v.pane)
	}
}

func TestChannelsActionIntentCarriesKey(t *testing.T) {
	v := channelsView{}
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}
	in := v.update(msg, channelsCtx())
	got, ok := in.(channelsActionIntent)
	if !ok {
		t.Fatalf("want channelsActionIntent, got %T", in)
	}
	if got.msg.String() != "p" {
		t.Errorf("intent carried msg=%q, want p", got.msg.String())
	}
	if v.cursor != 0 {
		t.Errorf("action key moved cursor to %d, want 0", v.cursor)
	}
}

// Guard: channelsView satisfies the tabView interface.
var _ tabView = (*channelsView)(nil)

// Guard: sampleChannels returns domain.Channel values (used by channelsCtx).
var _ = []domain.Channel(sampleChannels())
