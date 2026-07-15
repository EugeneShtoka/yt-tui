package ui

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// playlistsKeys binds the multi-pane Playlists keys (drill/back/delete) that the
// shared testListKeys omits.
func playlistsKeys() keyMap {
	return buildKeyMap(config.KeyBindings{
		Up:            "up",
		Down:          "down",
		PageUp:        "pgup",
		PageDown:      "pgdown",
		Back:          "left",
		Right:         "right",
		DrillDown:     "enter",
		Close:         "esc",
		Delete:        "d",
		NewPlaylist:   "N",
		Download:      "D",
		DownloadAudio: "A",
	})
}

func playlistsCtx() viewCtx {
	return viewCtx{
		keys:     playlistsKeys(),
		pageSize: 10,
		circular: false,
		plCount:  3,
		plVideos: sampleVideos(),
	}
}

func TestPlaylistsListDownMovesCursor(t *testing.T) {
	v := playlistsView{}
	if in := v.update(tea.KeyMsg{Type: tea.KeyDown}, playlistsCtx()); in != nil {
		t.Fatalf("nav key should be handled in-view, got intent %T", in)
	}
	if v.cursor != 1 {
		t.Errorf("Down: cursor=%d, want 1", v.cursor)
	}
}

func TestPlaylistsListContextIsPlaylistList(t *testing.T) {
	if got := (playlistsView{}).context(playlistsCtx()); got != CtxPlaylistList {
		t.Errorf("pane 0 context=%v, want CtxPlaylistList", got)
	}
}

func TestPlaylistsVideoPaneContextIsVideoList(t *testing.T) {
	if got := (playlistsView{pane: 1}).context(playlistsCtx()); got != CtxVideoList {
		t.Errorf("pane 1 context=%v, want CtxVideoList", got)
	}
}

func TestPlaylistsListDrillDownForwardsIntent(t *testing.T) {
	v := playlistsView{}
	in := v.update(tea.KeyMsg{Type: tea.KeyEnter}, playlistsCtx())
	if _, ok := in.(playlistsActionIntent); !ok {
		t.Fatalf("drill-down should forward playlistsActionIntent (async load), got %T", in)
	}
	// Pane transition happens in apply, not in the view, so the view stays in pane 0.
	if v.pane != 0 {
		t.Errorf("view.update moved pane to %d, want 0 (apply owns the transition)", v.pane)
	}
}

func TestPlaylistsVideoPaneNavAndBack(t *testing.T) {
	v := playlistsView{pane: 1}
	// Down moves the video cursor, not the playlist cursor.
	v.update(tea.KeyMsg{Type: tea.KeyDown}, playlistsCtx())
	if v.vidCursor != 1 {
		t.Errorf("video pane Down: vidCursor=%d, want 1", v.vidCursor)
	}
	if v.cursor != 0 {
		t.Errorf("video pane Down moved playlist cursor to %d, want 0", v.cursor)
	}
	// Left returns to the playlist list.
	v.update(tea.KeyMsg{Type: tea.KeyLeft}, playlistsCtx())
	if v.pane != 0 {
		t.Errorf("Left: pane=%d, want 0", v.pane)
	}
}

func TestPlaylistsVideoPaneStaleCursorFallsBack(t *testing.T) {
	// cursor points past the (shrunk) playlist list — the view snaps back to pane 0.
	v := playlistsView{pane: 1, cursor: 5}
	if in := v.update(tea.KeyMsg{Type: tea.KeyDown}, playlistsCtx()); in != nil {
		t.Fatalf("stale-cursor fallback should not emit an intent, got %T", in)
	}
	if v.pane != 0 {
		t.Errorf("stale cursor: pane=%d, want 0", v.pane)
	}
}

func TestPlaylistsVideoPaneActionForwardsIntent(t *testing.T) {
	v := playlistsView{pane: 1}
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")}
	in := v.update(msg, playlistsCtx())
	got, ok := in.(playlistsActionIntent)
	if !ok {
		t.Fatalf("want playlistsActionIntent, got %T", in)
	}
	if got.msg.String() != "d" {
		t.Errorf("intent carried msg=%q, want d", got.msg.String())
	}
}

func TestPlaylistsJumpToLast(t *testing.T) {
	v := playlistsView{}
	v.jumpToLast(viewCtx{plCount: 3, plVideos: sampleVideos(), pageSize: 10})
	if v.cursor != 2 {
		t.Errorf("jumpToLast pane 0: cursor=%d, want 2", v.cursor)
	}
	v.pane = 1
	v.jumpToLast(viewCtx{plCount: 3, plVideos: make([]domain.Video, 4), pageSize: 10})
	if v.vidCursor != 3 {
		t.Errorf("jumpToLast pane 1: vidCursor=%d, want 3", v.vidCursor)
	}
}

// Guard: playlistsView satisfies the tabView interface.
var _ tabView = (*playlistsView)(nil)
