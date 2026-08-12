package overlay

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/EugeneShtoka/yt-tui/internal/tui/command"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

// CommandHelp is a scrollable, read-only overlay listing every command reachable
// from the palette (`:help`), with its aliases and help text. It mirrors the
// keyboard-shortcut Help overlay's box/scroll chrome but lists commands instead
// of key bindings. Root builds it from the registry plus the active view's
// local commands.
type CommandHelp struct {
	identity
	keys keymap.KeyMap
	cmds []command.Command
	vs   int // first visible row (scroll offset)
}

// NewCommandHelp builds the command listing from the resolved command set
// (local ∪ global, already deduped by Root).
func NewCommandHelp(keys keymap.KeyMap, cmds []command.Command) CommandHelp {
	return CommandHelp{identity: newIdentity(), keys: keys, cmds: cmds}
}

// ── overlay.Overlay interface ─────────────────────────────────────────────────

func (c CommandHelp) InterceptsInput() bool { return false }
func (c CommandHelp) WidthReduction() int   { return 0 }
func (c CommandHelp) HasFocus() bool        { return true }

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (c CommandHelp) Init() tea.Cmd  { return nil }
func (c CommandHelp) View() tea.View { return tea.NewView("") } // rendering done via Render(behind,...)

func (c CommandHelp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}
	switch {
	case key.Matches(km, c.keys.Escape), key.Matches(km, c.keys.Quit):
		return c, popCmd
	case key.Matches(km, c.keys.Down):
		c.vs++
	case key.Matches(km, c.keys.Up):
		if c.vs > 0 {
			c.vs--
		}
	case key.Matches(km, c.keys.PageDown):
		c.vs += 10
	case key.Matches(km, c.keys.PageUp):
		c.vs -= 10
		if c.vs < 0 {
			c.vs = 0
		}
	}
	return c, nil
}

// contentLines renders each command as an aligned "name  help" row, with any
// aliases appended to the name column.
func (c CommandHelp) contentLines(innerW int) []string {
	const nameCol = 20
	lines := make([]string, 0, len(c.cmds))
	for _, cmd := range c.cmds {
		name := ":" + cmd.Name
		if len(cmd.Aliases) > 0 {
			name += " (" + strings.Join(cmd.Aliases, ", ") + ")"
		}
		if lipgloss.Width(name) > nameCol-1 {
			name = render.Truncate(name, nameCol-1)
		}
		pad := nameCol - lipgloss.Width(name)
		row := styles.Bold.Render(name) + strings.Repeat(" ", pad) + render.ClampLine(cmd.Help, innerW-nameCol)
		lines = append(lines, row)
	}
	return lines
}

func (c CommandHelp) Render(behind string, width, height int) string {
	boxW, innerW := render.ModalBox(width, 6, 72)

	lines := c.contentLines(innerW)

	maxRows := height - 8 // borders, padding, title, footer
	if maxRows < 3 {
		maxRows = 3
	}
	needsScroll := len(lines) > maxRows
	maxVS := len(lines) - maxRows
	if maxVS < 0 {
		maxVS = 0
	}
	vs := c.vs
	if vs > maxVS {
		vs = maxVS
	}

	out := []string{styles.Bold.Render("Commands"), ""}
	visible := lines[vs:]
	if len(visible) > maxRows {
		visible = visible[:maxRows]
	}
	out = append(out, visible...)

	closeHint := c.keys.Escape.Help().Key + ": close"
	var left string
	if needsScroll {
		left = c.keys.Down.Help().Key + "/" + c.keys.Up.Help().Key + ": scroll"
	}
	out = append(out, "", styles.Help.Render(render.JustifyEnds(left, closeHint, innerW)))

	return placeOverlayBox(behind, strings.Join(out, "\n"), width, boxW)
}
