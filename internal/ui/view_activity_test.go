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
		PageDown:  "pgdn",
		DrillDown: "enter",
		Right:     "right",
	})
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
	keys := testActivityKeys()
	if _, ok := v.update(tea.KeyMsg{Type: tea.KeyDown}, keys, 10, false); ok {
		t.Fatal("Down should not signal navigation")
	}
	if v.cursor != 1 {
		t.Errorf("Down: cursor=%d, want 1", v.cursor)
	}
}

func TestActivityViewUpClampsAtTop(t *testing.T) {
	v := sampleActivity()
	keys := testActivityKeys()
	v.update(tea.KeyMsg{Type: tea.KeyUp}, keys, 10, false)
	if v.cursor != 0 {
		t.Errorf("Up at top (non-circular): cursor=%d, want 0", v.cursor)
	}
}

func TestActivityViewDrillDownReturnsEntry(t *testing.T) {
	v := sampleActivity()
	keys := testActivityKeys()
	v.cursor = 2
	e, ok := v.update(tea.KeyMsg{Type: tea.KeyEnter}, keys, 10, false)
	if !ok {
		t.Fatal("DrillDown should signal navigation")
	}
	if e.Type != "add_to_playlist" || e.VideoTitle != "Gamma" {
		t.Errorf("DrillDown returned wrong entry: %+v", e)
	}
}

func TestActivityViewDrillDownEmptyNoNav(t *testing.T) {
	v := activityView{}
	keys := testActivityKeys()
	if _, ok := v.update(tea.KeyMsg{Type: tea.KeyEnter}, keys, 10, false); ok {
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
