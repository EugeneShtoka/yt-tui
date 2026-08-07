// Package component provides reusable Bubble Tea view fragments shared across
// tabs — the status/hint bar and the tab bar — that render from supplied state
// and hold no application logic of their own.
package component

import (
	"time"

	tea "charm.land/bubbletea/v2"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

const statusExpiry = 5 * time.Second

// StatusBar renders the single-line status row at the bottom of the screen.
type StatusBar struct {
	text  string
	isErr bool
	gen   int
	right string // static right-side help text, e.g. "?: help  q: quit"
	hints string // tab-contextual shortcut hints shown on the left when idle
	width int
}

// NewStatusBar returns a StatusBar with the given static right-side text.
func NewStatusBar(right string) StatusBar {
	return StatusBar{right: right}
}

// WithWidth returns a copy sized to the given terminal width.
func (s StatusBar) WithWidth(w int) StatusBar {
	s.width = w
	return s
}

// WithHints returns a copy with updated tab-contextual shortcut hints.
func (s StatusBar) WithHints(hints string) StatusBar {
	s.hints = hints
	return s
}

func (s StatusBar) Init() tea.Cmd { return nil }

func (s StatusBar) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tuipkg.StatusMsg:
		s.text = m.Text
		s.isErr = m.IsErr
		s.gen++
		gen := s.gen
		return s, tea.Tick(statusExpiry, func(time.Time) tea.Msg { return tuipkg.StatusExpireMsg{Gen: gen} })
	case tuipkg.StatusExpireMsg:
		if m.Gen == s.gen {
			s.text = ""
		}
	}
	return s, nil
}

func (s StatusBar) Render() string {
	right := styles.Help.Render(s.right)

	var left string
	if s.text != "" {
		if s.isErr {
			left = styles.Error.Render("✗ " + s.text)
		} else {
			left = styles.Success.Render("✓ " + s.text)
		}
	} else if s.hints != "" {
		left = styles.Help.Render(s.hints)
	}

	return render.JustifyEnds(left, right, s.width)
}

func (s StatusBar) View() tea.View { return tea.NewView(s.Render()) }
