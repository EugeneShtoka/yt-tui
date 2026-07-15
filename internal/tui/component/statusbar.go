package component

import (
	"strings"
	"time"

	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StatusBar renders the single-line status row at the bottom of the screen.
type StatusBar struct {
	text     string
	isErr    bool
	statusAt time.Time
	right    string // static right-side help text, e.g. "?: help  q: quit"
	width    int
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

func (s StatusBar) Init() tea.Cmd { return nil }

func (s StatusBar) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m, ok := msg.(tuipkg.StatusMsg); ok {
		s.text = m.Text
		s.isErr = m.IsErr
		s.statusAt = time.Now()
	}
	return s, nil
}

func (s StatusBar) View() string {
	right := styles.Help.Render(s.right)

	var left string
	if s.text != "" && time.Since(s.statusAt) < 5*time.Second {
		if s.isErr {
			left = styles.Error.Render("✗ " + s.text)
		} else {
			left = styles.Success.Render("✓ " + s.text)
		}
	}

	space := s.width - lipgloss.Width(left) - lipgloss.Width(right)
	if space < 1 {
		space = 1
	}
	return left + strings.Repeat(" ", space) + right
}
