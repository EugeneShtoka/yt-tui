package overlay

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

// Confirm is a generic yes/no modal gating an irreversible action behind an
// explicit acknowledgement. It defaults to No (a stray Enter cancels), emits
// onConfirm only when the user actively chooses Yes, and closes either way.
type Confirm struct {
	identity
	keys      keymap.KeyMap
	prompt    string
	onConfirm tea.Msg
	yes       bool // false = No (default), true = Yes
}

// NewConfirm builds a confirmation overlay for prompt. onConfirm is dispatched
// (as a tea.Cmd result) when the user confirms; canceling dispatches nothing.
func NewConfirm(keys keymap.KeyMap, prompt string, onConfirm tea.Msg) Confirm {
	return Confirm{identity: newIdentity(), keys: keys, prompt: prompt, onConfirm: onConfirm}
}

// ── overlay.Overlay interface ─────────────────────────────────────────────────

func (c Confirm) InterceptsInput() bool { return false }
func (c Confirm) WidthReduction() int   { return 0 }
func (c Confirm) HasFocus() bool        { return true }

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (c Confirm) Init() tea.Cmd  { return nil }
func (c Confirm) View() tea.View { return tea.NewView("") } // rendering done via Render(behind,...)

func (c Confirm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}
	switch {
	case key.Matches(km, c.keys.Escape):
		return c, popCmd
	case km.Text == "y" || km.Text == "Y":
		return c, c.confirmCmd()
	case km.Text == "n" || km.Text == "N":
		return c, popCmd
	case key.Matches(km, c.keys.Left):
		c.yes = false
	case key.Matches(km, c.keys.Right):
		c.yes = true
	case key.Matches(km, c.keys.Up), key.Matches(km, c.keys.Down):
		c.yes = !c.yes
	case key.Matches(km, c.keys.DrillDown):
		if c.yes {
			return c, c.confirmCmd()
		}
		return c, popCmd
	}
	return c, nil
}

// confirmCmd dispatches the onConfirm message and closes the overlay. The
// action never opens another overlay, so batching the close with it is safe
// (no top-of-stack race).
func (c Confirm) confirmCmd() tea.Cmd {
	onConfirm := c.onConfirm
	return tea.Batch(func() tea.Msg { return onConfirm }, popCmd)
}

func (c Confirm) Render(behind string, width, _ int) string {
	boxW := width / 2
	if boxW > 60 {
		boxW = 60
	}
	if boxW < 24 {
		boxW = 24
	}

	no, yes := " No ", " Yes "
	if c.yes {
		yes = styles.Selected.Render(yes)
		no = styles.Dim.Render(no)
	} else {
		no = styles.Selected.Render(no)
		yes = styles.Dim.Render(yes)
	}
	buttons := lipgloss.JoinHorizontal(lipgloss.Top, no, "    ", yes)

	out := []string{
		styles.Bold.Render(c.prompt),
		"",
		buttons,
		"",
		styles.Help.Render("←/→ select · y/n · enter: confirm · esc: cancel"),
	}
	return placeOverlayBox(behind, strings.Join(out, "\n"), width, boxW)
}
