package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Playlists tab renderers. These stay on Model (rather than moving onto
// playlistsView) because they read router-owned state — the create/type text
// inputs, the spinner, the playlist slices, and the per-playlist video cache. The
// playlistsView reaches them through the viewCtx.renderPlaylists closure captured
// per frame (see view_playlists.go / view_tab.go).

func (m Model) renderPlaylists(height int) string {
	header := styleSectionTitle.Render("Playlists")
	headerH := lipgloss.Height(header)

	if m.createTypeMode {
		opt0 := "  Local playlist"
		opt1 := "  YouTube playlist"
		if m.createTypeSel == 0 {
			opt0 = styleSelected.Render("▶ Local playlist")
		} else {
			opt1 = styleSelected.Render("▶ YouTube playlist")
		}
		prompt := styleInputPrompt.Render("New playlist: ") + "\n" + opt0 + "\n" + opt1
		body := m.renderPlaylistRows(height-headerH-3) + "\n\n\n"
		return lipgloss.JoinVertical(lipgloss.Left, header, body+prompt)
	}

	if m.createMode {
		label := "New local playlist: "
		if m.createModeYT {
			label = "New YouTube playlist: "
		}
		prompt := styleInputPrompt.Render(label) + m.createInput.View()
		body := m.renderPlaylistRows(height-headerH-2) + "\n\n"
		return lipgloss.JoinVertical(lipgloss.Left, header, body+prompt)
	}

	if m.playlist.pane == 1 && m.playlist.cursor < m.playlistCount() {
		plName := m.selectedPlaylistName()
		subHeader := styleSectionTitle.Render("← " + plName)
		subH := lipgloss.Height(subHeader)

		plKey := m.selectedPlaylistKey()
		vids := m.playlistVidCache[plKey]
		body := ""
		switch {
		case len(vids) > 0:
			body = m.renderVideoRows(vids, m.playlist.vidCursor, m.playlist.vidVS, height-headerH-subH)
		case m.playlistVidLoading:
			body = m.spinner.View() + " Loading from YouTube…"
		default:
			body = styleDim.Render("Empty playlist. Add videos with 'a' from other tabs.")
		}
		return lipgloss.JoinVertical(lipgloss.Left, header, subHeader, body)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, m.renderPlaylistRows(height-headerH))
}

func (m Model) renderPlaylistRows(height int) string {
	n := m.playlistCount()
	start, end := scrollWindowAt(m.playlist.vs, n, height)
	labelW := m.width - colNum - 1 - 4
	selW := m.width - colNum - 1
	var rows []string
	for i := start; i < end && i < n; i++ {
		var label string
		if m.ytPlLoaded && i < len(m.ytPlaylists) {
			label = m.ytPlaylists[i].Title
		} else {
			localIdx := i
			if m.ytPlLoaded {
				localIdx -= len(m.ytPlaylists)
			}
			if localIdx >= 0 && localIdx < len(m.playlists) {
				label = m.playlists[localIdx].Name
			}
		}
		label = truncate(label, labelW)
		if i == m.playlist.cursor {
			numStr := styleRowNum.Background(colorBgSelect).Render(fmt.Sprintf("%*d ", colNum, i+1))
			rows = append(rows, numStr+styleSelected.Width(selW).Render("▶ "+label))
		} else {
			numStr := styleRowNum.Render(fmt.Sprintf("%*d ", colNum, i+1))
			rows = append(rows, numStr+"  "+label)
		}
	}
	if m.ytPlLoading {
		rows = append(rows, styleDim.Render("  "+m.spinner.View()+" syncing playlists…"))
	}
	return strings.Join(rows, "\n")
}
