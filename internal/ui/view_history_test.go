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
		PageDown:    "pgdown",
		DrillDown:   "enter",
		Right:       "right",
		Back:        "left",
		Close:       "esc",
		Play:        "p",
		Delete:      "d",
		HideChannel: "h",
	})
}

func testHistoryCtx() viewCtx {
	return viewCtx{keys: testHistoryKeys(), pageSize: 10, circular: false, db: &fakeStore{}}
}

// histKind decodes a viewIntent into its history kind/entry (nil → None).
func histKind(in viewIntent) (histIntentKind, db.HistoryEntry) {
	if hi, ok := in.(historyIntent); ok {
		return hi.kind, hi.entry
	}
	return histIntentNone, db.HistoryEntry{}
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
	kind, _ := histKind(v.update(tea.KeyMsg{Type: tea.KeyDown}, testHistoryCtx()))
	if kind != histIntentNone {
		t.Fatalf("Down should return no intent, got %v", kind)
	}
	if v.cursor != 1 {
		t.Errorf("Down: cursor=%d, want 1", v.cursor)
	}
}

func TestHistoryViewUpClampsAtTop(t *testing.T) {
	v := sampleHistory()
	v.update(tea.KeyMsg{Type: tea.KeyUp}, testHistoryCtx())
	if v.cursor != 0 {
		t.Errorf("Up at top: cursor=%d, want 0", v.cursor)
	}
}

func TestHistoryViewPlayOnVideoEntry(t *testing.T) {
	v := sampleHistory()
	kind, entry := histKind(v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}, testHistoryCtx()))
	if kind != histIntentPlay {
		t.Fatalf("Play on video entry: intent=%v, want histIntentPlay", kind)
	}
	if entry.VideoID != "v1" {
		t.Errorf("Play: entry.VideoID=%q, want v1", entry.VideoID)
	}
}

func TestHistoryViewPlayOnSearchEntryDoesNothing(t *testing.T) {
	v := sampleHistory()
	v.cursor = 2 // search entry
	kind, _ := histKind(v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}, testHistoryCtx()))
	if kind != histIntentNone {
		t.Errorf("Play on search entry should return no intent, got %v", kind)
	}
}

func TestHistoryViewDrillDownOnSearchReturnsSearchIntent(t *testing.T) {
	v := sampleHistory()
	v.cursor = 2 // search entry
	kind, entry := histKind(v.update(tea.KeyMsg{Type: tea.KeyEnter}, testHistoryCtx()))
	if kind != histIntentDrillSearch {
		t.Fatalf("DrillDown on search: intent=%v, want histIntentDrillSearch", kind)
	}
	if entry.Details != "golang tutorial" {
		t.Errorf("DrillDown search: Details=%q, want 'golang tutorial'", entry.Details)
	}
}

func TestHistoryViewDrillDownOnVideoOpensDetail(t *testing.T) {
	v := sampleHistory()
	// fakeStore.VideoHistory returns nil, so detail will be nil but detailVideoID is set.
	kind, _ := histKind(v.update(tea.KeyMsg{Type: tea.KeyEnter}, testHistoryCtx()))
	if kind != histIntentNone {
		t.Fatalf("DrillDown on video should return no intent, got %v", kind)
	}
	if v.detailVideoID != "v1" {
		t.Errorf("DrillDown: detailVideoID=%q, want v1", v.detailVideoID)
	}
}

func TestHistoryViewEscapeClosesDetail(t *testing.T) {
	v := sampleHistory()
	v.detailVideoID = "v1"
	v.detail = []db.HistoryEntry{{VideoID: "v1", Title: "Alpha", EventType: "streamVideo", Timestamp: time.Now()}}
	kind, _ := histKind(v.update(tea.KeyMsg{Type: tea.KeyEsc}, testHistoryCtx()))
	if kind != histIntentNone {
		t.Errorf("Escape in detail: intent=%v, want none", kind)
	}
	if v.detailVideoID != "" {
		t.Errorf("Escape should clear detailVideoID, got %q", v.detailVideoID)
	}
}

func TestHistoryViewLeftClosesDetail(t *testing.T) {
	v := sampleHistory()
	v.detailVideoID = "v1"
	v.update(tea.KeyMsg{Type: tea.KeyLeft}, testHistoryCtx())
	if v.detailVideoID != "" {
		t.Errorf("Left should clear detailVideoID, got %q", v.detailVideoID)
	}
}

func TestHistoryViewDeleteVideoEntry(t *testing.T) {
	v := sampleHistory()
	kind, entry := histKind(v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}, testHistoryCtx()))
	if kind != histIntentDelete {
		t.Fatalf("Delete: intent=%v, want histIntentDelete", kind)
	}
	if entry.VideoID != "v1" {
		t.Errorf("Delete: entry.VideoID=%q, want v1", entry.VideoID)
	}
	if len(v.entries) != 2 {
		t.Errorf("Delete: entries len=%d, want 2", len(v.entries))
	}
}

func TestHistoryViewDeleteClampsCursorAtEnd(t *testing.T) {
	v := sampleHistory()
	v.cursor = 2 // last entry
	v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}, testHistoryCtx())
	if v.cursor != 1 {
		t.Errorf("Delete last: cursor=%d, want 1", v.cursor)
	}
}

func TestHistoryViewHideChannelOnVideoEntry(t *testing.T) {
	v := sampleHistory()
	kind, entry := histKind(v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}, testHistoryCtx()))
	if kind != histIntentHide {
		t.Fatalf("HideChannel: intent=%v, want histIntentHide", kind)
	}
	if entry.ChannelID != "c1" {
		t.Errorf("HideChannel: ChannelID=%q, want c1", entry.ChannelID)
	}
}

func TestHistoryViewHideChannelOnSearchDoesNothing(t *testing.T) {
	v := sampleHistory()
	v.cursor = 2 // search entry
	kind, _ := histKind(v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}, testHistoryCtx()))
	if kind != histIntentNone {
		t.Errorf("HideChannel on search: intent=%v, want none", kind)
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
	if got := v.context(viewCtx{}); got != CtxHistoryVideo {
		t.Errorf("context for video entry: %v, want CtxHistoryVideo", got)
	}
}

func TestHistoryViewContextSearchEntry(t *testing.T) {
	v := sampleHistory()
	v.cursor = 2
	if got := v.context(viewCtx{}); got != CtxHistorySearch {
		t.Errorf("context for search entry: %v, want CtxHistorySearch", got)
	}
}

func TestHistoryViewContextDetailOpen(t *testing.T) {
	v := sampleHistory()
	v.detailVideoID = "v1"
	if got := v.context(viewCtx{}); got != CtxHistoryVideo {
		t.Errorf("context in detail mode: %v, want CtxHistoryVideo", got)
	}
}

func TestHistoryViewJumpTo(t *testing.T) {
	v := sampleHistory()
	v.jumpTo(2, viewCtx{pageSize: 10})
	if v.cursor != 2 {
		t.Errorf("jumpTo(2): cursor=%d, want 2", v.cursor)
	}
}

func TestHistoryViewJumpToLast(t *testing.T) {
	v := sampleHistory()
	v.jumpToLast(viewCtx{pageSize: 10})
	if v.cursor != 2 {
		t.Errorf("jumpToLast: cursor=%d, want 2", v.cursor)
	}
}
