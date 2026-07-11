package ui

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/db"
	tea "github.com/charmbracelet/bubbletea"
)

func sampleLocal() []db.LocalVideo {
	return []db.LocalVideo{
		{ID: "a", Title: "Alpha"},
		{ID: "b", Title: "Beta"},
		{ID: "c", Title: "Gamma"},
	}
}

func TestLocalDownMovesCursor(t *testing.T) {
	v := localView{sort: vidSortNone}
	keys := testListKeys()
	if in := v.update(tea.KeyMsg{Type: tea.KeyDown}, keys, sampleLocal(), 10, false); in.kind != localIntentNone {
		t.Fatalf("Down should not produce an intent, got %d", in.kind)
	}
	if v.cursor != 1 {
		t.Errorf("Down: cursor=%d, want 1", v.cursor)
	}
}

func TestLocalPageDownMovesCursor(t *testing.T) {
	v := localView{}
	keys := testListKeys()
	vids := make([]db.LocalVideo, 10)
	for i := range vids {
		vids[i] = db.LocalVideo{ID: string(rune('a' + i))}
	}
	// One page (size 2) down from the top advances the viewport by 2 rows.
	v.update(tea.KeyMsg{Type: tea.KeyPgDown}, keys, vids, 2, false)
	if v.cursor != 2 {
		t.Errorf("PageDown: cursor=%d, want 2", v.cursor)
	}
}

func TestLocalPlayReturnsVideo(t *testing.T) {
	v := localView{cursor: 2}
	keys := testListKeys()
	in := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}, keys, sampleLocal(), 10, false)
	if in.kind != localIntentPlay || in.video.ID != "c" {
		t.Errorf("Play: got kind=%d id=%q, want localIntentPlay/c", in.kind, in.video.ID)
	}
}

func TestLocalDeleteReturnsIntent(t *testing.T) {
	v := localView{cursor: 0}
	keys := testListKeys()
	in := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")}, keys, sampleLocal(), 10, false)
	if in.kind != localIntentDelete || in.video.ID != "a" {
		t.Errorf("Delete: got kind=%d id=%q, want localIntentDelete/a", in.kind, in.video.ID)
	}
}

func TestLocalActionEmptyNoIntent(t *testing.T) {
	v := localView{}
	keys := testListKeys()
	if in := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}, keys, nil, 10, false); in.kind != localIntentNone {
		t.Errorf("Play on empty library: kind=%d, want localIntentNone", in.kind)
	}
}

func TestLocalCurrentVideoConvertsEntry(t *testing.T) {
	v := localView{cursor: 1}
	got, ok := v.currentVideo(sampleLocal())
	if !ok {
		t.Fatal("currentVideo should return ok")
	}
	if got.ID != "b" || got.URL != "https://www.youtube.com/watch?v=b" {
		t.Errorf("currentVideo: got %+v", got)
	}
	if _, ok := (localView{cursor: 9}).currentVideo(sampleLocal()); ok {
		t.Error("currentVideo out of range should be false")
	}
}
