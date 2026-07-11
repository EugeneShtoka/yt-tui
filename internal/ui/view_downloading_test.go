package ui

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
	tea "github.com/charmbracelet/bubbletea"
)

func testListKeys() keyMap {
	return buildKeyMap(config.KeyBindings{
		Up:          "up",
		Down:        "down",
		PageUp:      "pgup",
		PageDown:    "pgdown",
		Play:        "p",
		PlayAudio:   "a",
		Delete:      "d",
		HideChannel: "h",
		CopyURL:     "y",
	})
}

func sampleItems() []downloader.Item {
	return []downloader.Item{
		{Video: youtube.Video{ID: "a", Title: "Alpha"}, Status: downloader.StatusActive},
		{Video: youtube.Video{ID: "b", Title: "Beta"}, Status: downloader.StatusComplete},
		{Video: youtube.Video{ID: "c", Title: "Gamma"}, Status: downloader.StatusPending},
	}
}

func TestDownloadingDownMovesCursor(t *testing.T) {
	v := downloadingView{}
	items := sampleItems()
	keys := testListKeys()
	if in := v.update(tea.KeyMsg{Type: tea.KeyDown}, keys, items, 10, false); in.kind != dlIntentNone {
		t.Fatalf("Down should not produce an intent, got %d", in.kind)
	}
	if v.cursor != 1 {
		t.Errorf("Down: cursor=%d, want 1", v.cursor)
	}
}

func TestDownloadingPlayReturnsItem(t *testing.T) {
	v := downloadingView{cursor: 1}
	items := sampleItems()
	keys := testListKeys()
	in := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}, keys, items, 10, false)
	if in.kind != dlIntentPlay {
		t.Fatalf("Play: kind=%d, want dlIntentPlay", in.kind)
	}
	if in.item.Video.ID != "b" {
		t.Errorf("Play: item=%q, want b", in.item.Video.ID)
	}
}

func TestDownloadingDeleteReturnsIntent(t *testing.T) {
	v := downloadingView{cursor: 2}
	items := sampleItems()
	keys := testListKeys()
	in := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")}, keys, items, 10, false)
	if in.kind != dlIntentDelete || in.item.Video.ID != "c" {
		t.Errorf("Delete: got kind=%d item=%q, want dlIntentDelete/c", in.kind, in.item.Video.ID)
	}
}

func TestDownloadingActionEmptyNoIntent(t *testing.T) {
	v := downloadingView{}
	keys := testListKeys()
	// no items → cursor 0 is out of range, play must not fire.
	if in := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}, keys, nil, 10, false); in.kind != dlIntentNone {
		t.Errorf("Play on empty queue: kind=%d, want dlIntentNone", in.kind)
	}
}

func TestDownloadingReclampAfterShrink(t *testing.T) {
	v := downloadingView{cursor: 2, vs: 0}
	v.reclamp(2, 10) // queue shrank to 2 items
	if v.cursor != 1 {
		t.Errorf("reclamp: cursor=%d, want 1", v.cursor)
	}
}

func TestDownloadingCurrentItem(t *testing.T) {
	v := downloadingView{cursor: 1}
	items := sampleItems()
	got, ok := v.currentItem(items)
	if !ok || got.Video.ID != "b" {
		t.Errorf("currentItem: got %q ok=%v, want b/true", got.Video.ID, ok)
	}
	if _, ok := (downloadingView{cursor: 9}).currentItem(items); ok {
		t.Error("currentItem out of range should be false")
	}
}
