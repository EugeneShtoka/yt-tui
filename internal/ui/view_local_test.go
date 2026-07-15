package ui

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/library"
	tea "github.com/charmbracelet/bubbletea"
)

func sampleLocal() []domain.LocalVideo {
	return []domain.LocalVideo{
		{ID: "a", Title: "Alpha"},
		{ID: "b", Title: "Beta"},
		{ID: "c", Title: "Gamma"},
	}
}

func localCtx(videos []domain.LocalVideo, pageSize int) viewCtx {
	lib := library.New(videos)
	return viewCtx{keys: testListKeys(), pageSize: pageSize, circular: false, library: &lib}
}

func TestLocalDownMovesCursor(t *testing.T) {
	v := localView{sort: vidSortNone}
	if in := v.update(tea.KeyMsg{Type: tea.KeyDown}, localCtx(sampleLocal(), 10)); in != nil {
		t.Fatalf("Down should not produce an intent, got %T", in)
	}
	if v.cursor != 1 {
		t.Errorf("Down: cursor=%d, want 1", v.cursor)
	}
}

func TestLocalPageDownMovesCursor(t *testing.T) {
	v := localView{}
	vids := make([]domain.LocalVideo, 10)
	for i := range vids {
		vids[i] = domain.LocalVideo{ID: string(rune('a' + i))}
	}
	// One page (size 2) down from the top advances the viewport by 2 rows.
	v.update(tea.KeyMsg{Type: tea.KeyPgDown}, localCtx(vids, 2))
	if v.cursor != 2 {
		t.Errorf("PageDown: cursor=%d, want 2", v.cursor)
	}
}

func TestLocalPlayReturnsVideo(t *testing.T) {
	v := localView{cursor: 2}
	in := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}, localCtx(sampleLocal(), 10))
	got, ok := in.(localIntent)
	if !ok || got.kind != localIntentPlay || got.video.ID != "c" {
		t.Errorf("Play: got %T kind=%v id=%q, want localIntent/localIntentPlay/c", in, got.kind, got.video.ID)
	}
}

func TestLocalDeleteReturnsIntent(t *testing.T) {
	v := localView{cursor: 0}
	in := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")}, localCtx(sampleLocal(), 10))
	got, ok := in.(localIntent)
	if !ok || got.kind != localIntentDelete || got.video.ID != "a" {
		t.Errorf("Delete: got %T kind=%v id=%q, want localIntent/localIntentDelete/a", in, got.kind, got.video.ID)
	}
}

func TestLocalActionEmptyNoIntent(t *testing.T) {
	v := localView{}
	if in := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}, localCtx(nil, 10)); in != nil {
		t.Errorf("Play on empty library: got %T, want nil", in)
	}
}

func TestLocalCurrentVideoConvertsEntry(t *testing.T) {
	v := localView{cursor: 1}
	lib := library.New(sampleLocal())
	got, ok := v.currentVideo(localCtx(sampleLocal(), 10))
	if !ok {
		t.Fatal("currentVideo should return ok")
	}
	if got.ID != "b" || got.URL != "https://www.youtube.com/watch?v=b" {
		t.Errorf("currentVideo: got %+v", got)
	}
	if _, ok := (localView{cursor: 9}).currentVideo(viewCtx{library: &lib}); ok {
		t.Error("currentVideo out of range should be false")
	}
}

func TestLocalContext(t *testing.T) {
	if got := (localView{}).context(viewCtx{}); got != CtxLocal {
		t.Errorf("context=%v, want CtxLocal", got)
	}
}
