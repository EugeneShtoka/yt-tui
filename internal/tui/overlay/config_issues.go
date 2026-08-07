package overlay

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

// ConfigIssues is a scrollable, read-only overlay that lists the non-fatal
// problems found while loading configuration and probing the environment
// (invalid enums reset to defaults, dropped/unreachable panels, missing yt-dlp,
// an unusable cookie source). It is opened once on startup when any issue
// exists and dismissed with Escape/Quit — the app runs regardless. It mirrors
// the Help overlay's scroll/render pattern to stay consistent.
type ConfigIssues struct {
	identity
	issues []config.ConfigIssue
	keys   keymap.KeyMap
	vs     int // first visible row (scroll offset)
}

// NewConfigIssues builds the overlay for the given issue list.
func NewConfigIssues(issues []config.ConfigIssue, keys keymap.KeyMap) ConfigIssues {
	return ConfigIssues{identity: newIdentity(), issues: issues, keys: keys}
}

// ── overlay.Overlay interface ─────────────────────────────────────────────────

func (c ConfigIssues) InterceptsInput() bool { return false }
func (c ConfigIssues) WidthReduction() int   { return 0 }
func (c ConfigIssues) HasFocus() bool        { return true }

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (c ConfigIssues) Init() tea.Cmd  { return nil }
func (c ConfigIssues) View() tea.View { return tea.NewView("") } // rendering done via Render(behind,...)

func (c ConfigIssues) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}
	switch {
	case key.Matches(km, c.keys.Escape), key.Matches(km, c.keys.Quit):
		return c, func() tea.Msg { return PopOverlayMsg{} }
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

// contentLines renders each issue as a severity-marked, word-wrapped block:
// the first physical line carries a colored marker, continuation lines are
// indented under the text so multi-line messages stay legible.
func (c ConfigIssues) contentLines(innerW int) []string {
	const marker = 2 // "● " / "▲ "
	var lines []string
	for _, iss := range c.issues {
		mark, style := "▲", styles.Warning
		if iss.Severity == config.SeverityError {
			mark, style = "●", styles.Error
		}
		wrapped := render.WordWrap(iss.Message, innerW-marker)
		for i, w := range wrapped {
			if i == 0 {
				lines = append(lines, style.Render(mark)+" "+w)
				continue
			}
			lines = append(lines, strings.Repeat(" ", marker)+w)
		}
	}
	return lines
}

func (c ConfigIssues) Render(behind string, width, height int) string {
	boxW, innerW := render.ModalBox(width, 7, 84)

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

	out := []string{styles.Bold.Render("Configuration issues"), ""}
	visible := lines[vs:]
	if len(visible) > maxRows {
		visible = visible[:maxRows]
	}
	out = append(out, visible...)

	closeHint := c.keys.Escape.Help().Key + ": dismiss"
	var left string
	if needsScroll {
		left = c.keys.Down.Help().Key + "/" + c.keys.Up.Help().Key + ": scroll"
	}
	out = append(out, "", styles.Help.Render(render.JustifyEnds(left, closeHint, innerW)))

	return placeOverlayBox(behind, strings.Join(out, "\n"), width, boxW)
}
