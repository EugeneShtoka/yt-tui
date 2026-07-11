package ui

import (
	"fmt"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/youtube"
	"github.com/charmbracelet/lipgloss"
)

// Channels tab renderers. These stay on Model (rather than moving onto
// channelsView) because they read a broad slice of router-owned state — the
// spinner, the channel/video slices, the latest-video map, and the alias/tag edit
// input. The channelsView reaches them through the viewCtx.renderChannels closure
// captured per frame (see view_channels.go / view_tab.go).

func (m Model) renderSubChannels(height int) string {
	headerText := "Channels"
	if m.subChLoading {
		headerText += "  " + styleDim.Render(m.spinner.View()+" loading…")
	}
	if m.channels.tagsMode {
		headerText += "  " + styleDim.Render("[tags]")
	}
	header := styleSectionTitle.Render(headerText)
	headerH := lipgloss.Height(header)

	if m.channels.tagsMode {
		return m.renderSubChannelsTags(header, headerH, height)
	}

	// ── Flat mode ─────────────────────────────────────────────────────────────
	if m.channels.pane == 0 {
		var body string
		if m.subChLoading && len(m.subChannels) == 0 {
			body = m.spinner.View() + " Loading channels…"
		} else if len(m.subChannels) == 0 {
			body = styleDim.Render("No channels found.")
		} else {
			body = m.renderChannelList(m.sortedChannels(), height-headerH)
		}
		if m.subChEditMode != 0 {
			body = m.appendEditInput(body)
		}
		return lipgloss.JoinVertical(lipgloss.Left, header, body)
	}

	// ── Flat mode: channel videos pane ────────────────────────────────────────
	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderSubChannelsVideoPane(header, headerH, height, m.sortedChannels(), m.channels.cursor)...)
}

func (m Model) renderSubChannelsTags(header string, headerH, height int) string {
	switch m.channels.pane {
	case 0: // tag list
		var body string
		if m.subChLoading && len(m.subChannels) == 0 {
			body = m.spinner.View() + " Loading channels…"
		} else {
			body = m.renderTagList(height - headerH)
		}
		return lipgloss.JoinVertical(lipgloss.Left, header, body)

	case 1: // video list for selected tag
		tagHeader := styleSectionTitle.Render("← " + tagDisplayName(m.channels.tagSel))
		tagH := lipgloss.Height(tagHeader)
		filterLine := m.filterBar()
		filterH := 0
		if filterLine != "" {
			filterH = 1
		}
		vids, cur := m.contentVideos(m.tagVideos(), m.channels.cursor)
		body := m.renderVideoRows(vids, cur, m.channels.vs, height-headerH-tagH-filterH)
		parts := []string{header, tagHeader}
		if filterLine != "" {
			parts = append(parts, filterLine)
		}
		return lipgloss.JoinVertical(lipgloss.Left, append(parts, body)...)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, m.renderTagList(height-headerH))
}

func (m Model) renderSubChannelsVideoPane(header string, headerH, height int, sorted []youtube.Channel, cursor int) []string {
	chName := ""
	if cursor < len(sorted) {
		chName = sorted[cursor].DisplayName()
	}
	subHeaderText := "← " + chName
	if m.subChVidRefreshing {
		subHeaderText += "  " + styleDim.Render(m.spinner.View()+" refreshing…")
	}
	subHeader := styleSectionTitle.Render(subHeaderText)
	subH := lipgloss.Height(subHeader)

	filterLine := m.filterBar()
	filterH := 0
	if filterLine != "" {
		filterH = 1
	}
	var body string
	if m.subChVidLoading {
		body = m.spinner.View() + " Loading…"
	} else {
		vids, cur := m.contentVideos(m.subChVideos, m.channels.vidCursor)
		body = m.renderVideoRows(vids, cur, m.channels.vidVS, height-headerH-subH-filterH)
	}
	parts := []string{header, subHeader}
	if filterLine != "" {
		parts = append(parts, filterLine)
	}
	return append(parts, body)
}

func (m Model) renderTagList(height int) string {
	items := m.tagListItems()
	labelW := m.width - colNum - 3

	colHeader := strings.Repeat(" ", colNum) + " " + "  " +
		styleColHeader.Width(labelW).Render("Tag")

	windowH := height - 1
	start, end := scrollWindowAt(m.channels.tagVS, len(items), windowH)
	rows := []string{colHeader}

	for i := start; i < end && i < len(items); i++ {
		tag := items[i]
		count := len(m.channelsInTag(tag))
		label := fmt.Sprintf("%s (%d)", tagDisplayName(tag), count)
		selected := i == m.channels.tagCursor

		numStyle := styleRowNum
		rowStyle := styleNormal.Width(labelW)
		indicator := "  "

		if selected {
			indicator = styleSelected.Render("▶ ")
			numStyle = numStyle.Background(colorBgSelect)
			rowStyle = styleSelected.Width(labelW)
		}

		numStr := numStyle.Render(fmt.Sprintf("%*d ", colNum, i+1))
		rows = append(rows, numStr+indicator+rowStyle.Render(label))
	}
	return strings.Join(rows, "\n")
}

// appendEditInput adds the inline channel edit input below the channel list body.
func (m Model) appendEditInput(body string) string {
	label := "Alias: "
	if m.subChEditMode == 2 {
		label = "Tags (comma-separated): "
	}
	inputLine := "\n" + styleInputPrompt.Render(label) + m.subChEditInput.View()
	return body + inputLine
}

const (
	colChName = 22 // channel name column in channels-mode list
	colSubs   = 8  // subscriber count column
	colTags   = 14 // tags column in flat channel list
)

func (m Model) renderChannelList(channels []youtube.Channel, height int) string {
	if len(channels) == 0 {
		return ""
	}

	// Row layout: num + indicator + chName + tags + subs + title(W) + dur + views + date
	titleW := m.width - colNum - 1 - 2 - colChName - 1 - colTags - 1 - colSubs - 1 - colDuration - 1 - colViews - 1 - colDate
	if titleW < 10 {
		titleW = 10
	}

	colHeader := strings.Repeat(" ", colNum) + " " + "  " +
		styleColHeader.Width(colChName).Render("Channel") + " " +
		styleColHeader.Width(colTags).Render("Tags") + " " +
		styleColHeader.Width(colSubs).Render("Subs") + " " +
		styleColHeader.Width(titleW).Render("Latest Video") + " " +
		styleColHeader.Width(colDuration).Render("Duration") + " " +
		styleColHeader.Width(colViews).Render("Views") + " " +
		styleColHeader.Width(colDate).Render("Date")

	windowH := height - 1
	start, end := scrollWindowAt(m.channels.vs, len(channels), windowH)
	rows := []string{colHeader}

	for i := start; i < end && i < len(channels); i++ {
		ch := channels[i]
		latest := m.subChLatest[ch.ID]
		selected := i == m.channels.cursor

		chName := truncate(ch.DisplayName(), colChName-2)
		tagsStr := truncate(strings.Join(ch.Tags, ", "), colTags)
		subs := fmtViews(ch.Subscribers)
		vidTitle := truncate(latest.Title, titleW)
		dur := latest.DurationStr()
		views := latest.ViewsStr()
		date := latest.DateStr()

		sep := " "
		numStyle := styleRowNum
		chStyle := styleNormal.Width(colChName)
		tagsStyle := styleDim.Width(colTags)
		subsStyle := styleDuration.Width(colSubs)
		titleStyle := styleNormal.Width(titleW)
		durStyle := styleDuration.Width(colDuration)
		viewsStyle := styleDuration.Width(colViews)
		dateStyle := styleChannel.Width(colDate)
		indicator := "  "

		if selected {
			indicator = styleSelected.Render("▶ ")
			numStyle = numStyle.Background(colorBgSelect)
			sep = lipgloss.NewStyle().Background(colorBgSelect).Render(" ")
			chStyle = chStyle.Background(colorBgSelect)
			tagsStyle = tagsStyle.Background(colorBgSelect)
			subsStyle = subsStyle.Background(colorBgSelect)
			titleStyle = styleSelected.Width(titleW)
			durStyle = durStyle.Background(colorBgSelect)
			viewsStyle = viewsStyle.Background(colorBgSelect)
			dateStyle = dateStyle.Background(colorBgSelect)
		}

		numStr := numStyle.Render(fmt.Sprintf("%*d ", colNum, i+1))
		rows = append(rows, numStr+indicator+
			chStyle.Render(chName)+sep+
			tagsStyle.Render(tagsStr)+sep+
			subsStyle.Render(subs)+sep+
			titleStyle.Render(vidTitle)+sep+
			durStyle.Render(dur)+sep+
			viewsStyle.Render(views)+sep+
			dateStyle.Render(date))
	}
	return strings.Join(rows, "\n")
}
