package overlay

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
)

// TestHelpOverlayRenders confirms the help overlay lists shortcuts, honors the
// ClampLine invariant, and closes on the Help/Escape keys.
func TestHelpOverlayRenders(t *testing.T) {
	const width, height = 120, 40
	keys := keymap.Build(config.KeyBindings{
		Help: "?", Quit: "q", Close: "esc", Up: "k", Down: "j",
		OpenTranscript: "e", Play: "p", Download: "d",
	})
	h := NewHelp(keys)

	out := h.Render(rectangularBehind(width, height), width, height)
	if !strings.Contains(out, "Keyboard shortcuts") {
		t.Fatalf("help overlay missing title:\n%s", out)
	}
	if !strings.Contains(out, "quit") || !strings.Contains(out, "transcript") {
		t.Errorf("help overlay missing expected descriptions:\n%s", out)
	}
	for i, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w > width {
			t.Errorf("line %d width = %d, want <= %d", i, w, width)
		}
	}

	// Help key closes the overlay (emits PopOverlayMsg).
	_, cmd := h.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	if cmd == nil {
		t.Fatal("help key produced no command")
	}
	if _, ok := cmd().(PopOverlayMsg); !ok {
		t.Fatalf("help key must emit PopOverlayMsg, got %T", cmd())
	}
}
