package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderSearch draws the Search tab. The cursor/scroll values are owned by the
// searchView and passed in (see view_search.go); everything else it reads is
// router-owned state (input, spinner, result slices). It stays on Model and is
// reached through the viewCtx.renderSearch closure captured per frame.
func (m Model) renderSearch(height, cursor, vs, vidCursor, vidVS int) string {
	prompt := " " + styleInputPrompt.Render("Search: ") + m.searchInput.View()
	promptH := 1
	remaining := height - promptH - 1

	// Channel drill-down
	if m.searchChSel != nil {
		subHeader := styleSectionTitle.Render("← " + truncate(m.searchChSel.Name, m.width-4))
		subH := lipgloss.Height(subHeader)
		filterLine := m.filterBar()
		filterH := 0
		if filterLine != "" {
			filterH = 1
		}
		var body string
		if m.searchChLoading {
			body = m.spinner.View() + " Loading…"
		} else {
			vids, cur := m.contentVideos(m.searchChVideos, vidCursor)
			body = m.renderVideoRows(vids, cur, vidVS, remaining-subH-filterH)
		}
		parts := []string{prompt, subHeader}
		if filterLine != "" {
			parts = append(parts, filterLine)
		}
		return lipgloss.JoinVertical(lipgloss.Left, append(parts, body)...)
	}

	if m.searchLoading {
		return lipgloss.JoinVertical(lipgloss.Left, prompt, m.spinner.View()+" Searching…")
	}
	if len(m.searchChannels) == 0 && len(m.searchVideos) == 0 {
		if m.lastQuery != "" {
			return lipgloss.JoinVertical(lipgloss.Left, prompt, styleDim.PaddingLeft(1).Render("No results for: "+m.lastQuery))
		}
		return lipgloss.JoinVertical(lipgloss.Left, prompt, styleDim.PaddingLeft(1).Render("Type to search YouTube"))
	}

	header := styleSectionTitle.Render("Results for: " + m.lastQuery)
	headerH := lipgloss.Height(header)
	listH := remaining - headerH

	nCh := len(m.searchChannels)
	var rows []string

	// ── Channels section ──────────────────────────────────────────────────────
	if nCh > 0 {
		rows = append(rows, styleDim.PaddingLeft(1).Render("Channels"))
		nameW := m.width - colNum - 1 - 4
		selW := m.width - colNum - 1
		for i, ch := range m.searchChannels {
			name := truncate(ch.Name, nameW)
			if cursor == i {
				numStr := styleRowNum.Background(colorBgSelect).Render(fmt.Sprintf("%*d ", colNum, i+1))
				rows = append(rows, numStr+styleSelected.Width(selW).Render("▶ "+name))
			} else {
				numStr := styleRowNum.Render(fmt.Sprintf("%*d ", colNum, i+1))
				rows = append(rows, numStr+"  "+name)
			}
		}
		if len(m.searchVideos) > 0 {
			rows = append(rows, styleDim.PaddingLeft(1).Render("Videos"))
		}
	}

	// ── Videos section ────────────────────────────────────────────────────────
	if len(m.searchVideos) > 0 {
		titleW := m.videoListTitleW()
		rows = append(rows, m.renderVideoColHeader(titleW))
		usedRows := len(rows)
		start, end := scrollWindowAt(vs, len(m.searchVideos), listH-usedRows)
		for i := start; i < end && i < len(m.searchVideos); i++ {
			rows = append(rows, m.renderVideoRow(m.searchVideos[i], cursor == nCh+i, titleW, i+1))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, prompt, header, strings.Join(rows, "\n"))
}
