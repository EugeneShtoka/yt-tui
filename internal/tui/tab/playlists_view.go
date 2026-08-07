package tab

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

func (t Playlists) View() tea.View {
	header := styles.SectionTitle.Render("Playlists")

	switch t.createStage {
	case plCreateTypeSelect:
		return tea.NewView(t.viewCreateTypeSelect(header))
	case plCreateNameInput:
		return tea.NewView(t.viewCreateNameInput(header))
	}

	cursor := t.plNav.Index()
	if t.pane == 1 && cursor < t.plCount() {
		return tea.NewView(t.viewVideoPane(header))
	}
	return tea.NewView(t.viewListPane(header))
}

// viewCreateTypeSelect renders the local/YouTube playlist type picker overlay.
func (t Playlists) viewCreateTypeSelect(header string) string {
	opt0, opt1 := "  Local playlist", "  YouTube playlist"
	if t.createTypeSel == 0 {
		opt0 = styles.Selected.Render("▶ Local playlist")
	} else {
		opt1 = styles.Selected.Render("▶ YouTube playlist")
	}
	prompt := styles.Bold.Render("New playlist: ") + "\n" + opt0 + "\n" + opt1
	return lipgloss.JoinVertical(lipgloss.Left, header, t.plNav.View()+"\n\n\n"+prompt)
}

// viewCreateNameInput renders the new-playlist name entry overlay.
func (t Playlists) viewCreateNameInput(header string) string {
	label := "New local playlist: "
	if t.createModeYT {
		label = "New YouTube playlist: "
	}
	prompt := styles.Bold.Render(label) + t.createInput.View()
	return lipgloss.JoinVertical(lipgloss.Left, header, t.plNav.View()+"\n\n"+prompt)
}

// viewVideoPane renders pane 1: the selected playlist's video list.
func (t Playlists) viewVideoPane(header string) string {
	suffix := ""
	if t.vidLoad == srcRefreshing {
		suffix = t.spinnerFrame + " refreshing…"
	}
	subHeader := drillSubHeader(t.selectedPlaylistName(), t.width, suffix)
	vids := t.vidCache[t.selectedPlaylistKey()]
	var body string
	switch {
	case len(vids) > 0:
		body = t.vidNav.View()
	case t.vidLoad == srcLoading:
		body = t.spinnerFrame + " Loading from YouTube…"
	default:
		body = styles.Dim.Render("Empty playlist.")
	}
	parts := []string{header, subHeader, body}
	if s := t.vidNav.NumBufView(); s != "" {
		parts = append(parts, s)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// viewListPane renders pane 0: the playlist list.
func (t Playlists) viewListPane(header string) string {
	body := t.plNav.View()
	if t.ytPlLoad.inFlight() {
		body += "\n" + styles.Dim.Render("  "+t.spinnerFrame+" syncing playlists…")
	}
	parts := []string{header, body}
	if s := t.plNav.NumBufView(); s != "" {
		parts = append(parts, s)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
