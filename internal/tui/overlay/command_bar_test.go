package overlay

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/command"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
)

func commandBarKeys() keymap.KeyMap {
	return keymap.Build(config.KeyBindings{
		Close: "esc", DrillDown: "enter", Quit: "q",
	})
}

func commandBarRegistry() command.Registry {
	var r command.Registry
	r.Register(
		command.Command{Name: "quit", Aliases: []string{"q"}, Help: "quit"},
		command.Command{Name: "download", Help: "download a url"},
	)
	return r
}

// typeInto feeds each rune of s to the bar as a key press.
func typeInto(bar CommandBar, s string) CommandBar {
	for _, r := range s {
		updated, _ := bar.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		bar = updated.(CommandBar)
	}
	return bar
}

func TestCommandBarInterfaceFlags(t *testing.T) {
	bar := NewCommandBar(commandBarKeys(), commandBarRegistry(), nil)
	if !bar.InterceptsInput() {
		t.Error("command bar must intercept input (it owns a focused text field)")
	}
	if !bar.HasFocus() {
		t.Error("command bar must report focus")
	}
	if bar.WidthReduction() != 0 {
		t.Error("command bar is a bottom line, not a side panel; WidthReduction must be 0")
	}
}

func TestCommandBarEnterRunsCommand(t *testing.T) {
	bar := NewCommandBar(commandBarKeys(), commandBarRegistry(), nil)
	bar = typeInto(bar, "download foo")

	_, cmd := bar.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter produced no command")
	}
	msg, ok := cmd().(tuipkg.RunCommandMsg)
	if !ok {
		t.Fatalf("enter must emit RunCommandMsg, got %T", cmd())
	}
	if msg.Input != "download foo" {
		t.Errorf("RunCommandMsg.Input = %q, want %q", msg.Input, "download foo")
	}
}

func TestCommandBarEscCancels(t *testing.T) {
	bar := NewCommandBar(commandBarKeys(), commandBarRegistry(), nil)
	_, cmd := bar.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("esc produced no command")
	}
	if _, ok := cmd().(PopOverlayMsg); !ok {
		t.Fatalf("esc must emit PopOverlayMsg, got %T", cmd())
	}
}

func TestCommandBarEmptyEnterCancels(t *testing.T) {
	bar := NewCommandBar(commandBarKeys(), commandBarRegistry(), nil)
	_, cmd := bar.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("empty enter produced no command")
	}
	if _, ok := cmd().(PopOverlayMsg); !ok {
		t.Fatalf("empty enter must cancel via PopOverlayMsg, got %T", cmd())
	}
}

func TestCommandBarTabCompletes(t *testing.T) {
	bar := NewCommandBar(commandBarKeys(), commandBarRegistry(), nil)
	bar = typeInto(bar, "dow")
	updated, _ := bar.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	bar = updated.(CommandBar)

	out := bar.Render(rectangularBehind(80, 10), 80, 10)
	if !strings.Contains(out, "download") {
		t.Errorf("Tab did not complete 'dow' to 'download':\n%s", out)
	}
}

func TestCommandBarRenderClampsWidth(t *testing.T) {
	const width, height = 40, 12
	bar := NewCommandBar(commandBarKeys(), commandBarRegistry(), nil)
	bar = typeInto(bar, "download some-really-long-argument-that-overflows-the-line")

	out := bar.Render(rectangularBehind(width, height), width, height)
	for i, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w > width {
			t.Errorf("line %d width = %d, want <= %d", i, w, width)
		}
	}
	// The prompt must sit on the bottom line.
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[len(lines)-1], ":") {
		t.Errorf("command prompt not on bottom line:\n%s", out)
	}
}
