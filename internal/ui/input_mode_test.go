package ui

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func TestEnterExitMode(t *testing.T) {
	var m Model
	if m.mode != modeNormal {
		t.Fatalf("zero-value mode = %d, want modeNormal", m.mode)
	}
	m.enterMode(modeCommand)
	if m.mode != modeCommand {
		t.Errorf("after enterMode(command): mode = %d", m.mode)
	}
	// Entering another mode replaces the current one (the exclusivity invariant).
	m.enterMode(modeSearchInput)
	if m.mode != modeSearchInput {
		t.Errorf("after enterMode(search): mode = %d", m.mode)
	}
	m.exitMode()
	if m.mode != modeNormal {
		t.Errorf("after exitMode: mode = %d", m.mode)
	}
}

// The playlist-create flow is the one non-trivial transition: the type selector
// hands off to name entry on Enter, and backs out to normal on Esc.
func TestCreateTypeTransitions(t *testing.T) {
	enter := func() Model {
		m := Model{mode: modeCreateType, createInput: textinput.New()}
		res, _ := m.handleCreateTypeInput(tea.KeyMsg{Type: tea.KeyEnter})
		return res.(Model)
	}()
	if enter.mode != modeCreatePlaylist {
		t.Errorf("createType + enter → mode %d, want modeCreatePlaylist", enter.mode)
	}

	esc := func() Model {
		m := Model{mode: modeCreateType, createInput: textinput.New()}
		res, _ := m.handleCreateTypeInput(tea.KeyMsg{Type: tea.KeyEsc})
		return res.(Model)
	}()
	if esc.mode != modeNormal {
		t.Errorf("createType + esc → mode %d, want modeNormal", esc.mode)
	}
}

// handleKey must dispatch to the mode's input handler ahead of normal
// navigation — proving the mode switch replaces the old bool if-ladder.
func TestHandleKeyRoutesByMode(t *testing.T) {
	m := Model{mode: modeCreateType, createInput: textinput.New(), cfg: &config.Config{}}
	res, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyDown}) // "down" selects YouTube in the type dialog
	nm := res.(Model)
	if nm.createTypeSel != 1 {
		t.Errorf("handleKey did not route to handleCreateTypeInput (createTypeSel=%d, want 1)", nm.createTypeSel)
	}
	if nm.mode != modeCreateType {
		t.Errorf("mode changed unexpectedly to %d", nm.mode)
	}
}
