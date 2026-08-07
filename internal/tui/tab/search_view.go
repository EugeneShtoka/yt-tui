package tab

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

func (t Search) View() tea.View {
	header := styles.SectionTitle.Render("Search")
	headerH := lipgloss.Height(header)
	label := styles.Warning.Render("Search:")
	prompt := " " + label + " " + t.input.View()
	remaining := t.height - headerH - 1

	var body string
	if t.drill.ch != nil {
		body = t.viewDrillDown(prompt, remaining)
	} else {
		body = t.viewResults(prompt, remaining)
	}
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, body))
}

func (t Search) viewDrillDown(prompt string, _ int) string {
	header := drillSubHeader(t.drill.ch.Name, t.width, "")
	if t.drill.loading {
		return lipgloss.JoinVertical(lipgloss.Left, prompt, header, t.spinnerFrame+" Loading…")
	}
	if len(t.drill.videos) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, prompt, header, styles.Dim.Render("No videos found."))
	}
	parts := []string{prompt, header, t.drill.nav.View()}
	if s := t.drill.nav.NumBufView(); s != "" {
		parts = append(parts, s)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (t Search) viewResults(prompt string, remaining int) string {
	showRecent := (t.input.Focused() || t.recentMode) && t.lastQuery == "" && len(t.recent.queries) > 0

	if t.loading {
		return lipgloss.JoinVertical(lipgloss.Left, prompt, t.spinnerFrame+" Searching…")
	}
	if len(t.channels) == 0 && len(t.videos) == 0 {
		if showRecent {
			return lipgloss.JoinVertical(lipgloss.Left, prompt, t.viewRecentSearches(remaining-1))
		}
		var hint string
		if t.lastQuery != "" {
			hint = "No results for: " + t.lastQuery
		} else {
			hint = "Type to search YouTube  (" + t.keys.Filter.Help().Key + " to focus)"
		}
		return lipgloss.JoinVertical(lipgloss.Left, prompt, styles.Dim.PaddingLeft(1).Render(hint))
	}

	resultsHeader := styles.SectionTitle.Render("Results for: " + t.lastQuery)
	parts := []string{prompt, resultsHeader}

	if len(t.channels) > 0 {
		parts = append(parts, t.srchPaneLabel("Channels", !t.onVideos), t.chNav.View())
	}
	if len(t.videos) > 0 {
		parts = append(parts, t.srchPaneLabel("Videos", t.onVideos), t.vidNav.View())
	}
	if s := t.chNav.NumBufView(); s != "" && !t.onVideos {
		parts = append(parts, s)
	}
	if s := t.vidNav.NumBufView(); s != "" && t.onVideos {
		parts = append(parts, s)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (t Search) srchPaneLabel(name string, focused bool) string {
	indicator := "  "
	style := styles.Dim
	if focused {
		indicator = "▶ "
		style = styles.Bold
	}
	return style.Render(indicator + name)
}

func (t Search) viewRecentSearches(height int) string {
	pageH := t.srchRecentPageHeight()
	if pageH > height-1 {
		pageH = height - 1
	}
	start, end := t.recent.window(pageH)

	highlighted := -1
	if t.recentMode {
		highlighted = t.recent.cursor
	} else if t.histIdx >= 0 {
		highlighted = t.histIdx
	}

	nameW := t.width - render.ColNum - 1 - 2
	if nameW < 10 {
		nameW = 10
	}

	rows := make([]string, 0, end-start+2)
	rows = append(rows, styles.Dim.PaddingLeft(render.ColNum+3).Render("Recent searches"))
	for i := start; i < end; i++ {
		q := t.recent.queries[i]
		numStyle := styles.RowNum
		indicator := "  "
		sep := " "
		nameStyle := styles.Normal.Width(nameW)
		if i == highlighted {
			indicator = styles.Selected.Render("▶ ")
			numStyle = numStyle.Background(styles.ColorBgSelect)
			sep = lipgloss.NewStyle().Background(styles.ColorBgSelect).Render(" ")
			nameStyle = styles.Selected.Width(nameW)
		}
		numStr := numStyle.Render(fmt.Sprintf("%*d", render.ColNum, i+1))
		rows = append(rows, numStr+sep+indicator+nameStyle.Render(render.Truncate(q, nameW)))
	}
	return strings.Join(rows, "\n")
}
