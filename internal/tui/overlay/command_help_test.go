package overlay

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/tui/command"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
)

func commandHelpKeys() keymap.KeyMap {
	return keymap.Build(config.KeyBindings{Close: "esc", Quit: "q", Up: "k", Down: "j"})
}

func TestCommandHelpListsCommands(t *testing.T) {
	const width, height = 100, 30
	cmds := []command.Command{
		{Name: "download", Aliases: []string{"dl"}, Help: "enqueue a video for download"},
		{Name: "clear-downloads", Help: "dismiss the whole download queue"},
	}
	ch := NewCommandHelp(commandHelpKeys(), cmds)

	out := ch.Render(rectangularBehind(width, height), width, height)
	for _, want := range []string{"download", "clear-downloads", "enqueue a video for download", "dl"} {
		if !strings.Contains(out, want) {
			t.Errorf("command help missing %q:\n%s", want, out)
		}
	}
	for i, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w > width {
			t.Errorf("line %d width = %d, want <= %d", i, w, width)
		}
	}
}

func TestCommandHelpClosesOnEscape(t *testing.T) {
	ch := NewCommandHelp(commandHelpKeys(), nil)
	_, cmd := ch.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("escape produced no command")
	}
	if _, ok := cmd().(PopOverlayMsg); !ok {
		t.Fatalf("escape must emit PopOverlayMsg, got %T", cmd())
	}
}
