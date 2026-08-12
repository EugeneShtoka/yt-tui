package overlay

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
)

func confirmKeys() keymap.KeyMap {
	return keymap.Build(config.KeyBindings{
		Close: "esc", DrillDown: "enter", Back: "h", Right: "l", Up: "k", Down: "j",
	})
}

type confirmSentinelMsg struct{}

func confirmMsgsFrom(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			if c != nil {
				out = append(out, c())
			}
		}
		return out
	}
	return []tea.Msg{msg}
}

func hasMsg[T any](msgs []tea.Msg) bool {
	for _, m := range msgs {
		if _, ok := m.(T); ok {
			return true
		}
	}
	return false
}

func TestConfirmDefaultsToNo(t *testing.T) {
	c := NewConfirm(confirmKeys(), "Delete everything?", confirmSentinelMsg{})
	// Enter on the default selection (No) must cancel, not confirm.
	msgs := confirmMsgsFrom(func() (cmd tea.Cmd) { _, cmd = c.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); return }())
	if hasMsg[confirmSentinelMsg](msgs) {
		t.Error("default selection must be No — Enter must not confirm")
	}
	if !hasMsg[PopOverlayMsg](msgs) {
		t.Error("Enter on No must close the overlay")
	}
}

func TestConfirmYesKeyConfirms(t *testing.T) {
	c := NewConfirm(confirmKeys(), "Delete everything?", confirmSentinelMsg{})
	_, cmd := c.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	msgs := confirmMsgsFrom(cmd)
	if !hasMsg[confirmSentinelMsg](msgs) {
		t.Error("'y' must dispatch the onConfirm message")
	}
	if !hasMsg[PopOverlayMsg](msgs) {
		t.Error("'y' must also close the overlay")
	}
}

func TestConfirmSelectYesThenEnter(t *testing.T) {
	c := NewConfirm(confirmKeys(), "Delete everything?", confirmSentinelMsg{})
	// Move selection to Yes, then Enter.
	m, _ := c.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	c = m.(Confirm)
	_, cmd := c.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !hasMsg[confirmSentinelMsg](confirmMsgsFrom(cmd)) {
		t.Error("selecting Yes then Enter must confirm")
	}
}

func TestConfirmEscCancels(t *testing.T) {
	c := NewConfirm(confirmKeys(), "Delete everything?", confirmSentinelMsg{})
	_, cmd := c.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	msgs := confirmMsgsFrom(cmd)
	if hasMsg[confirmSentinelMsg](msgs) {
		t.Error("Esc must not confirm")
	}
	if !hasMsg[PopOverlayMsg](msgs) {
		t.Error("Esc must close the overlay")
	}
}

func TestConfirmRenders(t *testing.T) {
	c := NewConfirm(confirmKeys(), "Delete everything?", confirmSentinelMsg{})
	out := c.Render(rectangularBehind(80, 20), 80, 20)
	if !strings.Contains(out, "Delete everything?") {
		t.Errorf("confirm overlay missing prompt:\n%s", out)
	}
	if !strings.Contains(out, "Yes") || !strings.Contains(out, "No") {
		t.Errorf("confirm overlay missing Yes/No choices:\n%s", out)
	}
	if c.InterceptsInput() {
		t.Error("confirm overlay owns no text input; InterceptsInput must be false")
	}
	if !c.HasFocus() {
		t.Error("confirm overlay must capture keys")
	}
}
