package component

import (
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// TabBar renders the row of tab labels at the top of the screen.
type TabBar struct {
	titles []string
	active int
	width  int
}

// NewTabBar returns a TabBar showing the given tab titles.
func NewTabBar(titles []string) TabBar {
	return TabBar{titles: titles}
}

// WithActive returns a copy with the given index marked as active.
func (t TabBar) WithActive(i int) TabBar {
	t.active = i
	return t
}

// WithWidth returns a copy sized to the given terminal width.
func (t TabBar) WithWidth(w int) TabBar {
	t.width = w
	return t
}

func (t TabBar) Init() tea.Cmd                       { return nil }
func (t TabBar) Update(tea.Msg) (tea.Model, tea.Cmd) { return t, nil }

func (t TabBar) View() string {
	var tabs []string
	for i, title := range t.titles {
		if i == t.active {
			tabs = append(tabs, styles.TabActive.Render(title))
		} else {
			tabs = append(tabs, styles.TabInactive.Render(title))
		}
	}
	bar := strings.Join(tabs, " ")
	return styles.TabBar.Width(t.width).Render(bar)
}
