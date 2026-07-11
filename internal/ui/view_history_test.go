package ui

import (
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	tea "github.com/charmbracelet/bubbletea"
)

func testHistoryKeys() keyMap {
	return buildKeyMap(config.KeyBindings{
		Up:          "up",
		Down:        "down",
		PageUp:      "pgup",
		PageDown:    "pgdn",
		DrillDown:   "enter",
		Right:       "right",
		Back:        "left",
		Close:       "esc",
		Play:        "p",
		Delete:      "d",
		HideChannel: "h",
	})
}

func sampleHistory() historyView {
	return historyView{
		entries: []db.HistoryEntry{
			{VideoID: "v1", Title: "Alpha", Channel: "Chan1", ChannelID: "c1", EventType: "streamVideo"},
			{VideoID: "v2", Title: "Beta", Channel: "Chan2", ChannelID: "c2", EventType: "playVideo"},
			{Details: "golang tutorial", EventType: "search"},
		},
	}
}

func TestHistoryViewDownMovesCursor(t *testing.T) {
	v := sampleHistory()
	keys := testHistoryKeys()
	intent := v.update(tea.KeyMsg{Type: tea.KeyDown}, keys, 10, false, &fakeStore{})
	if intent.kind != histIntentNone {
		t.Fatalf("Down should return no intent, got %v", intent.kind)
	}
	if v.cursor != 1 {
		t.Errorf("Down: cursor=%d, want 1", v.cursor)
	}
}

func TestHistoryViewUpClampsAtTop(t *testing.T) {
	v := sampleHistory()
	keys := testHistoryKeys()
	v.update(tea.KeyMsg{Type: tea.KeyUp}, keys, 10, false, &fakeStore{})
	if v.cursor != 0 {
		t.Errorf("Up at top: cursor=%d, want 0", v.cursor)
	}
}

func TestHistoryViewPlayOnVideoEntry(t *testing.T) {
	v := sampleHistory()
	keys := testHistoryKeys()
	intent := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}, keys, 10, false, &fakeStore{})
	if intent.kind != histIntentPlay {
		t.Fatalf("Play on video entry: intent=%v, want histIntentPlay", intent.kind)
	}
	if intent.entry.VideoID != "v1" {
		t.Errorf("Play: entry.VideoID=%q, want v1", intent.entry.VideoID)
	}
}

func TestHistoryViewPlayOnSearchEntryDoesNothing(t *testing.T) {
	v := sampleHistory()
	v.cursor = 2 // search entry
	keys := testHistoryKeys()
	intent := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}, keys, 10, false, &fakeStore{})
	if intent.kind != histIntentNone {
		t.Errorf("Play on search entry should return no intent, got %v", intent.kind)
	}
}

func TestHistoryViewDrillDownOnSearchReturnsSearchIntent(t *testing.T) {
	v := sampleHistory()
	v.cursor = 2 // search entry
	keys := testHistoryKeys()
	intent := v.update(tea.KeyMsg{Type: tea.KeyEnter}, keys, 10, false, &fakeStore{})
	if intent.kind != histIntentDrillSearch {
		t.Fatalf("DrillDown on search: intent=%v, want histIntentDrillSearch", intent.kind)
	}
	if intent.entry.Details != "golang tutorial" {
		t.Errorf("DrillDown search: Details=%q, want 'golang tutorial'", intent.entry.Details)
	}
}

func TestHistoryViewDrillDownOnVideoOpensDetail(t *testing.T) {
	v := sampleHistory()
	keys := testHistoryKeys()
	// fakeStore.VideoHistory returns nil, so detail will be nil but detailVideoID is set.
	intent := v.update(tea.KeyMsg{Type: tea.KeyEnter}, keys, 10, false, &fakeStore{})
	if intent.kind != histIntentNone {
		t.Fatalf("DrillDown on video should return no intent, got %v", intent.kind)
	}
	if v.detailVideoID != "v1" {
		t.Errorf("DrillDown: detailVideoID=%q, want v1", v.detailVideoID)
	}
}

func TestHistoryViewEscapeClosesDetail(t *testing.T) {
	v := sampleHistory()
	v.detailVideoID = "v1"
	v.detail = []db.HistoryEntry{{VideoID: "v1", Title: "Alpha", EventType: "streamVideo", Timestamp: time.Now()}}
	keys := testHistoryKeys()
	intent := v.update(tea.KeyMsg{Type: tea.KeyEsc}, keys, 10, false, &fakeStore{})
	if intent.kind != histIntentNone {
		t.Errorf("Escape in detail: intent=%v, want none", intent.kind)
	}
	if v.detailVideoID != "" {
		t.Errorf("Escape should clear detailVideoID, got %q", v.detailVideoID)
	}
}

func TestHistoryViewLeftClosesDetail(t *testing.T) {
	v := sampleHistory()
	v.detailVideoID = "v1"
	keys := testHistoryKeys()
	v.update(tea.KeyMsg{Type: tea.KeyLeft}, keys, 10, false, &fakeStore{})
	if v.detailVideoID != "" {
		t.Errorf("Left should clear detailVideoID, got %q", v.detailVideoID)
	}
}

func TestHistoryViewDeleteVideoEntry(t *testing.T) {
	v := sampleHistory()
	keys := testHistoryKeys()
	intent := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}, keys, 10, false, &fakeStore{})
	if intent.kind != histIntentDelete {
		t.Fatalf("Delete: intent=%v, want histIntentDelete", intent.kind)
	}
	if intent.entry.VideoID != "v1" {
		t.Errorf("Delete: entry.VideoID=%q, want v1", intent.entry.VideoID)
	}
	if len(v.entries) != 2 {
		t.Errorf("Delete: entries len=%d, want 2", len(v.entries))
	}
}

func TestHistoryViewDeleteClampsCursorAtEnd(t *testing.T) {
	v := sampleHistory()
	v.cursor = 2 // last entry
	keys := testHistoryKeys()
	v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}, keys, 10, false, &fakeStore{})
	if v.cursor != 1 {
		t.Errorf("Delete last: cursor=%d, want 1", v.cursor)
	}
}

func TestHistoryViewHideChannelOnVideoEntry(t *testing.T) {
	v := sampleHistory()
	keys := testHistoryKeys()
	intent := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}, keys, 10, false, &fakeStore{})
	if intent.kind != histIntentHide {
		t.Fatalf("HideChannel: intent=%v, want histIntentHide", intent.kind)
	}
	if intent.entry.ChannelID != "c1" {
		t.Errorf("HideChannel: ChannelID=%q, want c1", intent.entry.ChannelID)
	}
}

func TestHistoryViewHideChannelOnSearchDoesNothing(t *testing.T) {
	v := sampleHistory()
	v.cursor = 2 // search entry
	keys := testHistoryKeys()
	intent := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}, keys, 10, false, &fakeStore{})
	if intent.kind != histIntentNone {
		t.Errorf("HideChannel on search: intent=%v, want none", intent.kind)
	}
}

func TestHistoryViewLoadResetsState(t *testing.T) {
	v := historyView{cursor: 5, detailVideoID: "x", loaded: true}
	v.load(&fakeStore{}, func(string) {})
	if v.cursor != 0 {
		t.Errorf("load: cursor=%d, want 0", v.cursor)
	}
	if v.detailVideoID != "" {
		t.Errorf("load: detailVideoID=%q, want empty", v.detailVideoID)
	}
	if !v.loaded {
		t.Error("load: loaded should be true")
	}
}

func TestHistoryViewClear(t *testing.T) {
	v := sampleHistory()
	v.cursor = 2
	v.loaded = true
	v.detailVideoID = "v1"
	v.clear()
	if v.cursor != 0 || len(v.entries) != 0 || v.loaded || v.detailVideoID != "" {
		t.Errorf("clear: state not zeroed: %+v", v)
	}
}

func TestHistoryViewContextVideoEntry(t *testing.T) {
	v := sampleHistory()
	if got := v.context(); got != CtxHistoryVideo {
		t.Errorf("context for video entry: %v, want CtxHistoryVideo", got)
	}
}

func TestHistoryViewContextSearchEntry(t *testing.T) {
	v := sampleHistory()
	v.cursor = 2
	if got := v.context(); got != CtxHistorySearch {
		t.Errorf("context for search entry: %v, want CtxHistorySearch", got)
	}
}

func TestHistoryViewContextDetailOpen(t *testing.T) {
	v := sampleHistory()
	v.detailVideoID = "v1"
	if got := v.context(); got != CtxHistoryVideo {
		t.Errorf("context in detail mode: %v, want CtxHistoryVideo", got)
	}
}

func TestHistoryViewJumpTo(t *testing.T) {
	v := sampleHistory()
	v.jumpTo(2, 10)
	if v.cursor != 2 {
		t.Errorf("jumpTo(2): cursor=%d, want 2", v.cursor)
	}
}

func TestHistoryViewJumpToLast(t *testing.T) {
	v := sampleHistory()
	v.jumpToLast(10)
	if v.cursor != 2 {
		t.Errorf("jumpToLast: cursor=%d, want 2", v.cursor)
	}
}
