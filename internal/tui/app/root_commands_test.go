package app

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/command"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	ovpkg "github.com/EugeneShtoka/yt-tui/internal/tui/overlay"
)

func newCommandRoot() (Root, *paletteBackend) {
	be := &paletteBackend{}
	keys := keymap.Build(config.KeyBindings{Quit: "q", Close: "esc", CommandPrompt: ":", TabChord: "t"})
	var reg command.Registry
	reg.Register(globalCommands(context.Background(), be, []string{"Feed"})...)
	r := Root{
		backend: be,
		keys:    keys,
		cmds:    reg,
		tabs:    []tuipkg.Tab{fakeTab{}},
	}
	return r, be
}

// paletteBackend records the bulk-action calls the palette should trigger.
type paletteBackend struct {
	apitest.NopBackend
	cleared     bool
	deletedAll  bool
	deleteCount int
}

func (b *paletteBackend) ClearDownloads(context.Context) error { b.cleared = true; return nil }
func (b *paletteBackend) DeleteAllLocalFiles(context.Context) (int, error) {
	b.deletedAll = true
	return b.deleteCount, nil
}

func TestCommandPromptKeyOpensBar(t *testing.T) {
	r, _ := newCommandRoot()
	r2, _ := r.handleKey(tea.KeyPressMsg{Code: ':', Text: ":"})
	if n := len(r2.overlays); n != 1 {
		t.Fatalf("expected 1 overlay after `:`, got %d", n)
	}
	if _, ok := r2.overlays[0].(ovpkg.CommandBar); !ok {
		t.Fatalf("top overlay is %T, want CommandBar", r2.overlays[0])
	}
}

func TestCommandPromptDoesNotStack(t *testing.T) {
	r, _ := newCommandRoot()
	r, _ = r.handleKey(tea.KeyPressMsg{Code: ':', Text: ":"})
	r, _ = r.handleKey(tea.KeyPressMsg{Code: ':', Text: ":"})
	if n := len(r.overlays); n != 1 {
		t.Fatalf("`:` twice must not stack; got %d overlays", n)
	}
}

func TestRunCommandQuitPopsBarAndQuits(t *testing.T) {
	r, _ := newCommandRoot()
	r.overlays = []ovpkg.Overlay{ovpkg.NewCommandBar(r.keys, r.cmds, nil)}

	r2, cmd := r.handleRunCommand(tuipkg.RunCommandMsg{Input: "q"})
	if len(r2.overlays) != 0 {
		t.Errorf("command bar not popped; %d overlays remain", len(r2.overlays))
	}
	if !quitMsg(cmd) {
		t.Errorf(":q did not resolve to tea.Quit")
	}
}

func TestRunCommandUnknownReportsError(t *testing.T) {
	r, _ := newCommandRoot()
	r.overlays = []ovpkg.Overlay{ovpkg.NewCommandBar(r.keys, r.cmds, nil)}

	r2, cmd := r.handleRunCommand(tuipkg.RunCommandMsg{Input: "bogus arg"})
	if len(r2.overlays) != 0 {
		t.Errorf("command bar must pop even on unknown command")
	}
	sm, ok := runCmd(cmd).(tuipkg.StatusMsg)
	if !ok || !sm.IsErr {
		t.Fatalf("unknown command must yield error StatusMsg, got %#v", runCmd(cmd))
	}
}

func TestClearDownloadsHandler(t *testing.T) {
	r, be := newCommandRoot()
	_, cmd := r.Update(tuipkg.ClearDownloadsMsg{})
	msgs := flatten(runCmd(cmd))
	if !be.cleared {
		t.Error("ClearDownloads not called on backend")
	}
	if !containsMsg[tuipkg.DownloadItemsChangedMsg](msgs) {
		t.Error("clear-downloads must refresh the Downloading tab")
	}
}

func TestDeleteAllLocalHandler(t *testing.T) {
	r, be := newCommandRoot()
	be.deleteCount = 3
	_, cmd := r.Update(tuipkg.DeleteAllLocalFilesMsg{})
	msgs := flatten(runCmd(cmd))
	if !be.deletedAll {
		t.Error("DeleteAllLocalFiles not called on backend")
	}
	if !containsMsg[tuipkg.LocalVideosChangedMsg](msgs) {
		t.Error("delete-all-local must reload the Local tab")
	}
	if !hasStatusContaining(msgs, "3") {
		t.Error("delete-all-local must report the deleted count")
	}
}

func TestOpenConfirmHandler(t *testing.T) {
	r, _ := newCommandRoot()
	r2, _ := r.Update(tuipkg.OpenConfirmMsg{Prompt: "sure?", OnConfirm: tuipkg.DeleteAllLocalFilesMsg{}})
	nr := r2.(Root)
	if n := len(nr.overlays); n != 1 {
		t.Fatalf("expected a confirm overlay, got %d overlays", n)
	}
	if _, ok := nr.overlays[0].(ovpkg.Confirm); !ok {
		t.Fatalf("top overlay is %T, want Confirm", nr.overlays[0])
	}
}

func TestOpenCommandHelpHandler(t *testing.T) {
	r, _ := newCommandRoot()
	r2, _ := r.Update(tuipkg.OpenCommandHelpMsg{})
	nr := r2.(Root)
	if n := len(nr.overlays); n != 1 {
		t.Fatalf("expected a command-help overlay, got %d overlays", n)
	}
	if _, ok := nr.overlays[0].(ovpkg.CommandHelp); !ok {
		t.Fatalf("top overlay is %T, want CommandHelp", nr.overlays[0])
	}
}

func containsMsg[T any](msgs []tea.Msg) bool {
	for _, m := range msgs {
		if _, ok := m.(T); ok {
			return true
		}
	}
	return false
}

func hasStatusContaining(msgs []tea.Msg, sub string) bool {
	for _, m := range msgs {
		if sm, ok := m.(tuipkg.StatusMsg); ok && strings.Contains(sm.Text, sub) {
			return true
		}
	}
	return false
}
