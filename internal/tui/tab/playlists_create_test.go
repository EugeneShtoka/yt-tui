package tab

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// With no YouTube playlists loaded, starting the create flow goes straight to
// the name prompt; confirming a name issues a local-playlist create command.
func TestPlaylistsCreateLocalFlow(t *testing.T) {
	pl := NewPlaylists(context.Background(), &fakeBackend{}, testKeys(), false, "")
	pl, _ = updatePlaylists(pl, sized(80, 24))
	pl, _ = updatePlaylists(pl, plLocalLoadedMsg{playlists: []domain.Playlist{{ID: "local:1", Name: "A"}}})

	m, _ := pl.beginCreatePlaylist()
	pl = m.(Playlists)
	if pl.createStage != plCreateNameInput {
		t.Fatalf("local-only create should open the name prompt, stage=%v", pl.createStage)
	}

	pl.createInput.SetValue("Mixtape")
	m, cmd := pl.handleNameInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	pl = m.(Playlists)
	if pl.createStage != plCreateNone {
		t.Errorf("confirming should exit the create flow, stage=%v", pl.createStage)
	}
	if _, ok := runCmd(cmd).(plLocalCreatedMsg); !ok {
		t.Fatalf("confirm should create a local playlist, got %#v", runCmd(cmd))
	}
}

// With YouTube playlists loaded, the create flow opens the local/YT type picker;
// selecting the YouTube option lands on the name prompt in YT mode.
func TestPlaylistsCreateTypeSelectYT(t *testing.T) {
	pl := NewPlaylists(context.Background(), &fakeBackend{}, testKeys(), false, "")
	pl, _ = updatePlaylists(pl, sized(80, 24))
	pl, _ = updatePlaylists(pl, plYTLoadedMsg{playlists: []domain.YTPlaylist{{ID: "PL1", Title: "YT"}}})

	m, _ := pl.beginCreatePlaylist()
	pl = m.(Playlists)
	if pl.createStage != plCreateTypeSelect {
		t.Fatalf("YT present should open the type picker, stage=%v", pl.createStage)
	}

	m, _ = pl.handleTypeSelect(tea.KeyPressMsg{Text: "j"}) // Down → toggle to YouTube option
	pl = m.(Playlists)
	m, _ = pl.handleTypeSelect(tea.KeyPressMsg{Code: tea.KeyEnter})
	pl = m.(Playlists)
	if pl.createStage != plCreateNameInput {
		t.Fatalf("confirming a type should open the name prompt, stage=%v", pl.createStage)
	}
	if !pl.createModeYT {
		t.Error("selecting the YouTube option should set createModeYT")
	}
}

// Escape cancels the create flow from the name prompt.
func TestPlaylistsCreateCancel(t *testing.T) {
	pl := NewPlaylists(context.Background(), &fakeBackend{}, testKeys(), false, "")
	pl, _ = updatePlaylists(pl, sized(80, 24))
	pl.createStage = plCreateNameInput

	m, _ := pl.handleNameInput(tea.KeyPressMsg{Code: tea.KeyEscape})
	pl = m.(Playlists)
	if pl.createStage != plCreateNone {
		t.Errorf("Escape should cancel the create flow, stage=%v", pl.createStage)
	}
}

// Smoke test: accessors + Init loaders keep the tab's simple surface covered.
func TestPlaylists_AccessorsAndInit(t *testing.T) {
	pl := NewPlaylists(context.Background(), &fakeBackend{}, testKeys(), false, "")
	_ = pl.ID()
	_ = pl.Title()
	_ = pl.ShortHelp()
	_ = pl.InterceptsInput()
	_ = pl.Loading()
	if _, ok := pl.SelectedVideo(); ok {
		t.Error("a fresh Playlists should have no selected video")
	}
	if batch, ok := runCmd(pl.Init()).(tea.BatchMsg); ok {
		for _, c := range batch {
			runCmd(c)
		}
	}
}
