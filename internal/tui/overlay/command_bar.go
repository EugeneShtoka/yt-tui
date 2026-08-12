package overlay

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/command"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

// CommandBar is the bottom-line free-text command palette (`:`). It owns a
// focused text input, offers Tab completion against the command registry, and
// on Enter emits a RunCommandMsg for Root to resolve and dispatch — the bar
// never runs commands itself, keeping resolution single-sourced in Root and
// avoiding an overlay-ordering race when a command opens another overlay.
type CommandBar struct {
	identity
	keys  keymap.KeyMap
	input textinput.Model

	// reg + local are the read model for Tab completion only (a value copy of
	// the registry is safe: the global set is immutable after startup).
	reg   command.Registry
	local []command.Command

	// completion cycling state: comps is the candidate list captured on the
	// first Tab after an edit; compIdx cycles through it on each subsequent Tab.
	comps   []string
	compIdx int
}

// NewCommandBar builds a focused command bar. local are the view-local commands
// (from the active tab if it is a command.Provider); they shadow globals in
// both completion and resolution.
func NewCommandBar(keys keymap.KeyMap, reg command.Registry, local []command.Command) CommandBar {
	ti := textinput.New()
	ti.Prompt = ":"
	ti.Placeholder = "command"
	ti.Focus()
	return CommandBar{identity: newIdentity(), keys: keys, input: ti, reg: reg, local: local, compIdx: -1}
}

// ── overlay.Overlay interface ─────────────────────────────────────────────────

func (c CommandBar) InterceptsInput() bool { return true }
func (c CommandBar) WidthReduction() int   { return 0 }
func (c CommandBar) HasFocus() bool        { return true }

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (c CommandBar) Init() tea.Cmd  { return textinput.Blink }
func (c CommandBar) View() tea.View { return tea.NewView("") } // rendering done via Render(behind,...)

func (c CommandBar) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		c.input, cmd = c.input.Update(msg)
		return c, cmd
	}

	switch {
	case key.Matches(km, c.keys.Escape):
		return c, popCmd
	case key.Matches(km, c.keys.DrillDown):
		return c.run()
	case key.Matches(km, c.keys.Tab):
		return c.complete(), nil
	}

	// Any other key edits the text, invalidating an in-progress completion cycle.
	c.comps, c.compIdx = nil, -1
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return c, cmd
}

// run emits a RunCommandMsg for the typed line, or cancels on an empty line.
func (c CommandBar) run() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(c.input.Value())
	if input == "" {
		return c, popCmd
	}
	return c, func() tea.Msg { return tuipkg.RunCommandMsg{Input: input} }
}

// complete cycles the input through the completion candidates for the current
// line. The candidate list is captured on the first Tab after an edit and
// reused (cycled) on each subsequent Tab.
func (c CommandBar) complete() CommandBar {
	if c.comps == nil {
		c.comps = c.reg.Complete(c.input.Value(), c.local)
		c.compIdx = -1
	}
	if len(c.comps) == 0 {
		return c
	}
	c.compIdx = (c.compIdx + 1) % len(c.comps)
	c.input.SetValue(c.comps[c.compIdx])
	c.input.CursorEnd()
	return c
}

func (c CommandBar) Render(behind string, width, height int) string {
	line := render.ClampLine(styles.Bold.Render(c.input.View()), width)
	lines := strings.Split(behind, "\n")
	// Pin the prompt to the bottom row of the content region so it reads as a
	// command line, not a stray row after short tab content. Pad up to height
	// first (Root pads no further once content already fills the region).
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) == 0 {
		return line
	}
	lines[len(lines)-1] = line
	return strings.Join(lines, "\n")
}

// popCmd is the shared "close me" command emitted by overlays.
func popCmd() tea.Msg { return PopOverlayMsg{} }
