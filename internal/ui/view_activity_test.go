package ui

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	tea "github.com/charmbracelet/bubbletea"
)

func testActivityKeys() keyMap {
	return buildKeyMap(config.KeyBindings{
		Up:        "up",
		Down:      "down",
		PageUp:    "pgup",
		PageDown:  "pgdown",
		DrillDown: "enter",
		Right:     "right",
	})
}

func testActivityCtx() viewCtx {
	return viewCtx{keys: testActivityKeys(), pageSize: 10, circular: false}
}

func sampleActivity() activityView {
	return activityView{
		entries: []db.ActivityEntry{
			{Type: "subscribe", ChannelName: "Alpha"},
			{Type: "create_playlist", PlaylistName: "Beta"},
			{Type: "add_to_playlist", VideoTitle: "Gamma", PlaylistName: "Beta"},
		},
	}
}

func TestActivityViewDownMovesCursor(t *testing.T) {
	v := sampleActivity()
	if in := v.update(tea.KeyMsg{Type: tea.KeyDown}, testActivityCtx()); in != nil {
		t.Fatal("Down should not signal navigation")
	}
	if v.cursor != 1 {
		t.Errorf("Down: cursor=%d, want 1", v.cursor)
	}
}

func TestActivityViewUpClampsAtTop(t *testing.T) {
	v := sampleActivity()
	v.update(tea.KeyMsg{Type: tea.KeyUp}, testActivityCtx())
	if v.cursor != 0 {
		t.Errorf("Up at top (non-circular): cursor=%d, want 0", v.cursor)
	}
}

func TestActivityViewDrillDownReturnsEntry(t *testing.T) {
	v := sampleActivity()
	v.cursor = 2
	in := v.update(tea.KeyMsg{Type: tea.KeyEnter}, testActivityCtx())
	nav, ok := in.(activityNavIntent)
	if !ok {
		t.Fatalf("DrillDown should return activityNavIntent, got %T", in)
	}
	if nav.entry.Type != "add_to_playlist" || nav.entry.VideoTitle != "Gamma" {
		t.Errorf("DrillDown returned wrong entry: %+v", nav.entry)
	}
}

func TestActivityViewDrillDownEmptyNoNav(t *testing.T) {
	v := activityView{}
	if in := v.update(tea.KeyMsg{Type: tea.KeyEnter}, testActivityCtx()); in != nil {
		t.Error("DrillDown on empty list should not signal navigation")
	}
}

func TestActivityViewLoadClampsCursor(t *testing.T) {
	v := activityView{cursor: 5}
	v.load(&fakeStore{}, func(string) {})
	// fakeStore returns nil entries → cursor must clamp to 0.
	if v.cursor != 0 {
		t.Errorf("load with empty result: cursor=%d, want 0", v.cursor)
	}
}
