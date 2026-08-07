package tab

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

func (t Feed) View() tea.View {
	headerText := "Feed · " + feedModes[t.mode].label
	if t.refreshingActive() && t.spinnerFrame != "" {
		headerText += "  " + styles.Dim.Render(t.spinnerFrame+" refreshing…")
	}
	header := styles.SectionTitle.Render(headerText)

	var body string
	switch {
	case t.Loading() && t.feed.Len() == 0:
		body = " " + t.spinnerFrame + " Loading…"
	case t.feed.Len() == 0:
		body = styles.Dim.PaddingLeft(1).Render("No videos. Press r to refresh.")
	default:
		body = t.nav.View()
	}
	// The mode picker can be opened over any state (loading, empty, or list).
	if t.picker.isOpen() {
		body = t.picker.view(body, t.keys.Escape.Help().Key, t.width)
	}
	parts := []string{header, body}
	if s := t.nav.NumBufView(); s != "" {
		parts = append(parts, s)
	}
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

// refreshingActive reports whether a background fetch is in flight while content
// is already on screen (drives the header "refreshing…" spinner vs "Loading…").
func (t Feed) refreshingActive() bool { return t.Loading() && t.feed.Len() > 0 }
