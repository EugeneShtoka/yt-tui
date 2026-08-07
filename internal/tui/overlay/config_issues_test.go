package overlay

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
)

func ciKeys() keymap.KeyMap {
	return keymap.Build(config.KeyBindings{
		Quit: "q", Close: "esc", Up: "k", Down: "j", PageUp: "b", PageDown: "f",
	})
}

// TestConfigIssuesRenders confirms the overlay shows its title and every issue
// message, and upholds the ClampLine width invariant.
func TestConfigIssuesRenders(t *testing.T) {
	const width, height = 100, 30
	issues := []config.ConfigIssue{
		{Severity: config.SeverityWarning, Message: "feed_mode \"bananas\" is not a valid value; using \"recommended\""},
		{Severity: config.SeverityError, Message: "yt-dlp not found on PATH — YouTube features are unavailable"},
	}
	ci := NewConfigIssues(issues, ciKeys())

	out := ci.Render(rectangularBehind(width, height), width, height)
	if !strings.Contains(out, "Configuration issues") {
		t.Fatalf("overlay missing title:\n%s", out)
	}
	if !strings.Contains(out, "bananas") || !strings.Contains(out, "yt-dlp") {
		t.Errorf("overlay missing issue messages:\n%s", out)
	}
	for i, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w > width {
			t.Errorf("line %d width = %d, want <= %d", i, w, width)
		}
	}
}

// TestConfigIssuesCloses proves Escape dismisses the overlay via PopOverlayMsg.
func TestConfigIssuesCloses(t *testing.T) {
	ci := NewConfigIssues([]config.ConfigIssue{{Message: "x"}}, ciKeys())
	_, cmd := ci.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("escape produced no command")
	}
	if _, ok := cmd().(PopOverlayMsg); !ok {
		t.Fatalf("escape must emit PopOverlayMsg, got %T", cmd())
	}
}

// TestConfigIssuesScrolls confirms Down moves the scroll offset so a long list
// can be read past the visible window.
func TestConfigIssuesScrolls(t *testing.T) {
	var issues []config.ConfigIssue
	for i := 0; i < 50; i++ {
		issues = append(issues, config.ConfigIssue{Message: "issue number filler text"})
	}
	ci := NewConfigIssues(issues, ciKeys())
	updated, _ := ci.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if updated.(ConfigIssues).vs == 0 {
		t.Error("Down did not advance the scroll offset")
	}
}
