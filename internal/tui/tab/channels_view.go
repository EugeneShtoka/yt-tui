package tab

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

func (t Channels) View() tea.View {
	headerText := "Channels" + styles.Dim.Render(" · "+t.viewLabel())
	if t.loading {
		headerText += "  " + styles.Dim.Render(t.spinnerFrame+" loading…")
	}
	header := styles.SectionTitle.Render(headerText)
	headerH := lipgloss.Height(header)
	contentH := t.height - headerH
	return tea.NewView(t.viewContent(header, contentH))
}

func (t Channels) viewContent(header string, _ int) string {
	if !t.inDetail() {
		var body string
		switch {
		case t.loading && t.subs.Len() == 0:
			body = t.spinnerFrame + " Loading channels…"
		case len(t.sortedChs) == 0:
			body = styles.Dim.Render(t.emptyViewText())
		default:
			body = t.listNav.View()
		}
		if t.editMode != chEditNone {
			body = t.appendEditInput(body)
		}
		if t.picker.isOpen() {
			body = t.picker.view(body, t.keys.Escape.Help().Key, t.width)
		}
		return t.renderList([]string{header, body})
	}

	chName := ""
	idx := t.listNav.Index()
	if idx < len(t.sortedChs) {
		chName = t.sortedChs[idx].DisplayName()
	}
	suffix := ""
	if t.chVidLoad == srcRefreshing {
		suffix = t.spinnerFrame + " refreshing…"
	}
	body := t.vidNav.View()
	if t.chVidLoad == srcLoading {
		body = t.spinnerFrame + " Loading…"
	}
	return t.renderDetail(header, chName, suffix, body)
}
