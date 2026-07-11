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

func dlCtx(items []downloader.Item) viewCtx {
	return viewCtx{keys: testListKeys(), pageSize: 10, circular: false, dlItems: items}
}

func TestDownloadingDownMovesCursor(t *testing.T) {
	v := downloadingView{}
	if in := v.update(tea.KeyMsg{Type: tea.KeyDown}, dlCtx(sampleItems())); in != nil {
		t.Fatalf("Down should not produce an intent, got %T", in)
	}
	if v.cursor != 1 {
		t.Errorf("Down: cursor=%d, want 1", v.cursor)
	}
}

func TestDownloadingPlayReturnsItem(t *testing.T) {
	v := downloadingView{cursor: 1}
	in := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}, dlCtx(sampleItems()))
	got, ok := in.(downloadingIntent)
	if !ok || got.kind != dlIntentPlay {
		t.Fatalf("Play: got %T kind=%v, want downloadingIntent/dlIntentPlay", in, got.kind)
	}
	if got.item.Video.ID != "b" {
		t.Errorf("Play: item=%q, want b", got.item.Video.ID)
	}
}

func TestDownloadingDeleteReturnsIntent(t *testing.T) {
	v := downloadingView{cursor: 2}
	in := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")}, dlCtx(sampleItems()))
	got, ok := in.(downloadingIntent)
	if !ok || got.kind != dlIntentDelete || got.item.Video.ID != "c" {
		t.Errorf("Delete: got %T kind=%v item=%q, want downloadingIntent/dlIntentDelete/c", in, got.kind, got.item.Video.ID)
	}
}

func TestDownloadingActionEmptyNoIntent(t *testing.T) {
	v := downloadingView{}
	// no items → cursor 0 is out of range, play must not fire.
	if in := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}, dlCtx(nil)); in != nil {
		t.Errorf("Play on empty queue: got %T, want nil", in)
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

func TestDownloadingContext(t *testing.T) {
	if got := (downloadingView{}).context(); got != CtxDownloading {
		t.Errorf("context=%v, want CtxDownloading", got)
	}
}
