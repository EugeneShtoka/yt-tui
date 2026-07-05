package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	tabBar := m.renderTabBar()
	status := m.renderStatusBar()
	contentH := m.height - lipgloss.Height(tabBar) - lipgloss.Height(status)

	var content string
	var kittyOverlay string
	if m.showHelp {
		content = m.renderHelp(contentH)
	} else if m.vidDetailOverlay {
		thumbW, thumbH := m.thumbDimensions()

		m.width -= vidDetailPanelW
		left := m.renderContent(contentH)
		if m.addOverlay {
			left = m.renderAddOverlay(left)
		}
		m.width += vidDetailPanelW
		panel := m.renderVideoDetailPanel(vidDetailPanelW, contentH, thumbH)
		content = lipgloss.JoinHorizontal(lipgloss.Top, left, panel)

		if kittyCapable() && m.vidDetailThumb != nil {
			tabBarH := lipgloss.Height(tabBar)
			thumbRow := tabBarH + 2                    // 1-indexed: past tabBar rows + top border
			thumbCol := m.width - vidDetailPanelW + 2  // 1-indexed: past left panel + left border
			kittyOverlay = kittyImageOverlay(m.vidDetailThumb, thumbRow, thumbCol, thumbW, thumbH)
		}
	} else {
		content = m.renderContent(contentH)
		if m.addOverlay {
			content = m.renderAddOverlay(content)
		}
		// Delete the Kitty image if the panel just closed. BubbleTea's differential
		// renderer only writes this once (on the frame it appears), then skips it.
		if kittyCapable() {
			kittyOverlay = kittyDeleteOverlay()
		}
	}

	// Pad content so the status bar is always pinned to the bottom.
	if actual := lipgloss.Height(content); actual < contentH {
		content += strings.Repeat("\n", contentH-actual)
	}

	// kittyOverlay is appended after the full frame so BubbleTea writes it last.
	// DECSC/DECRC inside the overlay keeps BubbleTea's cursor tracking intact.
	return lipgloss.JoinVertical(lipgloss.Left, tabBar, content, status) + kittyOverlay
}

// ── Tab bar ───────────────────────────────────────────────────────────────────

func (m Model) renderTabBar() string {
	var tabs []string
	for _, id := range m.tabs {
		label := tabNames[id]
		if id == m.activeTab {
			tabs = append(tabs, styleTabActive.Render(label))
		} else {
			tabs = append(tabs, styleTabInactive.Render(label))
		}
	}
	bar := strings.Join(tabs, " ")
	return styleTabBar.Width(m.width).Render(bar)
}

// ── Status bar ────────────────────────────────────────────────────────────────

func (m Model) renderStatusBar() string {
	kh := m.keys.Help.Help().Key
	kq := m.keys.Quit.Help().Key

	// Non-context-help states (chords, status messages) always render single-row.
	kb := m.cfg.Keybindings
	var fixed string
	switch {
	case m.pendingChord != "":
		fixed = styleWarning.Render(m.chordHint())
	case m.gPending:
		fixed = styleWarning.Render(kb.GotoPrefix + " → " + kb.GotoPrefix + ": top  " + kb.GotoBottom + ": bottom")
	case m.numPrefix != "":
		fixed = styleWarning.Render(m.numPrefix + "G: jump to row")
	case m.status != "" && time.Since(m.statusAt) < 5*time.Second:
		if m.statusErr {
			fixed = styleError.Render("✗ " + m.status)
		} else {
			fixed = styleSuccess.Render("✓ " + m.status)
		}
	}
	if fixed != "" {
		right := styleHelp.Render(kh + ": help  " + kq + ": quit")
		space := m.width - lipgloss.Width(fixed) - lipgloss.Width(right)
		if space < 1 {
			space = 1
		}
		return fixed + strings.Repeat(" ", space) + right
	}

	// Context help: try single row; wrap to two rows if too wide.
	helpRaw := m.contextHelpRaw()
	r1 := styleHelp.Render(kh + ": help")
	r2 := styleHelp.Render(kq + ": quit")
	rightW := lipgloss.Width(r1)
	if lipgloss.Width(r2) > rightW {
		rightW = lipgloss.Width(r2)
	}
	rightSingle := styleHelp.Render(kh + ": help  " + kq + ": quit")

	if lipgloss.Width(styleHelp.Render(helpRaw))+1+lipgloss.Width(rightSingle) <= m.width {
		left := styleHelp.Render(helpRaw)
		space := m.width - lipgloss.Width(left) - lipgloss.Width(rightSingle)
		if space < 1 {
			space = 1
		}
		return left + strings.Repeat(" ", space) + rightSingle
	}

	// Two-row layout: split hints greedily at available width.
	availW := m.width - rightW - 1
	line1raw, line2raw := splitStatusHints(helpRaw, availW)
	l1 := styleHelp.Render(line1raw)
	l2 := styleHelp.Render(line2raw)
	p1 := m.width - lipgloss.Width(l1) - lipgloss.Width(r1)
	p2 := m.width - lipgloss.Width(l2) - lipgloss.Width(r2)
	if p1 < 1 {
		p1 = 1
	}
	if p2 < 1 {
		p2 = 1
	}
	return l1 + strings.Repeat(" ", p1) + r1 + "\n" +
		l2 + strings.Repeat(" ", p2) + r2
}

// splitStatusHints splits a double-space-separated hint string into two rows,
// greedily filling the first row up to maxW characters.
func splitStatusHints(text string, maxW int) (string, string) {
	parts := strings.Split(text, "  ")
	var row1 []string
	w1 := 0
	for i, p := range parts {
		pw := len(p)
		if w1 == 0 {
			row1 = append(row1, p)
			w1 = pw
		} else if w1+2+pw <= maxW {
			row1 = append(row1, p)
			w1 += 2 + pw
		} else {
			return strings.Join(row1, "  "), strings.Join(parts[i:], "  ")
		}
	}
	return text, ""
}

// chordHint returns the completion hint shown while a chord is pending.
// Driven entirely by the chord registry — no per-chord special cases.
func (m Model) chordHint() string {
	ctx := m.currentContext()
	for _, chord := range m.chordDefs() {
		if chord.trigger != m.pendingChord {
			continue
		}
		valid := validActions(chord.actions, ctx)
		var parts []string
		for _, a := range valid {
			parts = append(parts, a.key+": "+a.label)
		}
		if len(parts) == 0 {
			return chord.trigger + " → (nothing available)"
		}
		return chord.trigger + " → " + strings.Join(parts, "  ")
	}
	return ""
}

func (m Model) contextHelp() string {
	return styleHelp.Render(m.contextHelpRaw())
}

func (m Model) contextHelpRaw() string {
	switch m.cfg.HintMode {
	case "none":
		return ""
	case "minimal":
		return m.minimalHintRaw()
	default:
		return m.fullHintRaw()
	}
}

func (m Model) minimalHintRaw() string {
	kb := m.cfg.Keybindings
	play := m.keys.Play.Help().Key
	return fmt.Sprintf("j/k: move  %s: tab  %s: play", kb.TabChord, play)
}

func (m Model) fullHintRaw() string {
	kb := m.keys
	cfg := m.cfg.Keybindings
	dl := kb.Download.Help().Key
	dlA := kb.DownloadAudio.Help().Key
	cp := kb.CopyURL.Help().Key
	sub := kb.Subscribe.Help().Key
	unsub := kb.Unsubscribe.Help().Key
	mode := kb.ToggleMode.Help().Key
	play := kb.Play.Help().Key
	playA := kb.PlayAudio.Help().Key
	del := kb.Delete.Help().Key
	hide := kb.HideVideo.Help().Key
	hideCh := kb.HideChannel.Help().Key
	wl := kb.WatchLater.Help().Key
	addPl := kb.AddList.Help().Key
	newPl := kb.NewList.Help().Key
	ref := kb.Refresh.Help().Key
	drill := kb.DrillDown.Help().Key
	yt := m.ytClient != nil

	// Build chord trigger hints from the registry, filtered to current context.
	ctx := m.currentContext()
	var chordParts []string
	for _, chord := range m.chordDefs() {
		if len(validActions(chord.actions, ctx)) > 0 {
			chordParts = append(chordParts, chord.trigger+": "+chord.name)
		}
	}
	chords := strings.Join(chordParts, "  ")

	info := kb.VideoInfo.Help().Key

	switch m.activeTab {
	case tabRecommended:
		h := fmt.Sprintf("j/k: move  %s  %s: play  %s: play audio  %s: download  %s: dl audio  %s: copy url  %s: info  %s: hide video  %s: block channel", chords, play, playA, dl, dlA, cp, info, hide, hideCh)
		if yt {
			h += fmt.Sprintf("  %s: subscribe  %s: watch later  %s: add to playlist", sub, wl, addPl)
		}
		return h
	case tabSearch:
		if m.searchChSel != nil {
			return fmt.Sprintf("j/k: move  %s: info  %s: download  %s: dl audio  %s: copy url  %s: filter  h/esc: back", info, dl, dlA, cp, cfg.Filter)
		}
		h := fmt.Sprintf("j/k: move  %s  %s: play  %s: play audio  %s: open channel  %s: info  %s: download  %s: dl audio  %s: copy url", chords, play, playA, drill, info, dl, dlA, cp)
		if yt {
			h += fmt.Sprintf("  %s: subscribe", sub)
		}
		return h
	case tabSubscriptions:
		if m.subMode == subModeChannels && m.subChPane == 0 {
			return fmt.Sprintf("j/k: move  %s: open  %s: all videos  %s  %s: unsubscribe", drill, mode, chords, unsub)
		}
		if m.subMode == subModeChannels && m.subChPane == 1 {
			return fmt.Sprintf("j/k: move  %s: info  %s: download  %s: dl audio  %s: copy url  h/esc: back", info, dl, dlA, cp)
		}
		return fmt.Sprintf("j/k: move  %s  %s: play  %s: play audio  %s: channels  %s: info  %s: download  %s: dl audio  %s: copy url  %s: unsubscribe", chords, play, playA, mode, info, dl, dlA, cp, unsub)
	case tabPlaylists:
		if m.playlistPane == 1 {
			return fmt.Sprintf("j/k: move  %s: info  %s: open  %s: new playlist  %s: delete", info, drill, newPl, del)
		}
		return fmt.Sprintf("j/k: move  %s: open  %s: new playlist  %s: delete", drill, newPl, del)
	case tabDownloading:
		return fmt.Sprintf("j/k: move  %s: play  %s: play audio  %s: info  %s: block channel  %s: delete", play, playA, info, hideCh, del)
	case tabLocal:
		return fmt.Sprintf("j/k: move  %s  %s: play  %s: delete", chords, play, del)
	case tabHistory:
		if m.histDetailVideoID != "" {
			return "esc/h: back"
		}
		isSearchEntry := m.histCursor < len(m.histEntries) && m.histEntries[m.histCursor].EventType == "search"
		if !isSearchEntry {
			if yt {
				return fmt.Sprintf("j/k: move  %s: play  %s: details  %s: block channel  %s: subscribe  %s: delete  %s: refresh", play, drill, hideCh, sub, del, ref)
			}
			return fmt.Sprintf("j/k: move  %s: play  %s: details  %s: block channel  %s: delete  %s: refresh", play, drill, hideCh, del, ref)
		}
		return fmt.Sprintf("j/k: move  %s: search  %s: delete  %s: refresh", drill, del, ref)
	}
	return ""
}

// ── Content router ────────────────────────────────────────────────────────────

func (m Model) contentVideos(raw []youtube.Video, cursor int) ([]youtube.Video, int) {
	if m.localFilter != "" {
		return filterText(raw, m.localFilter), m.localFilterCursor
	}
	return raw, cursor
}

func (m Model) renderContent(height int) string {
	switch m.activeTab {
	case tabRecommended:
		vids, cur := m.contentVideos(m.recVideos, m.recCursor)
		return m.renderVideoList(vids, cur, m.recVS, m.recLoading, m.recRefreshing, height, "Recommended for you")
	case tabSubscriptions:
		return m.renderSubscriptions(height)
	case tabPlaylists:
		return m.renderPlaylists(height)
	case tabSearch:
		return m.renderSearch(height)
	case tabDownloading:
		return m.renderDownloading(height)
	case tabLocal:
		return m.renderLocal(height)
	case tabHistory:
		return m.renderHistory(height)
	}
	return ""
}

// ── Generic video list ────────────────────────────────────────────────────────

const (
	colNum      = 4
	colChannel  = 22
	colDuration = 13
	colViews    = 8
	colDate     = 11
)

func (m Model) filterBar() string {
	if m.localFilterFocused {
		return styleInputPrompt.Render("/ ") + m.localFilterInput.View()
	}
	if m.localFilter != "" {
		return styleInputPrompt.Render("/ ") + m.localFilter + styleDim.Render("  (esc to clear)")
	}
	return ""
}

func (m Model) renderVideoList(
	videos []youtube.Video,
	cursor, vs int,
	loading bool,
	refreshing bool,
	height int,
	title string,
) string {
	headerText := title
	if refreshing {
		headerText += "  " + styleDim.Render(m.spinner.View()+" refreshing…")
	}
	header := styleSectionTitle.Render(headerText)
	headerH := lipgloss.Height(header)

	filterLine := m.filterBar()
	filterH := 0
	if filterLine != "" {
		filterH = 1
	}
	listH := height - headerH - filterH

	if loading && !refreshing {
		body := m.spinner.View() + " Loading…"
		return lipgloss.JoinVertical(lipgloss.Left, header, body)
	}
	if len(videos) == 0 {
		msg := "No videos. Press r to refresh."
		if m.localFilter != "" {
			msg = "No matches for: " + m.localFilter
		}
		parts := []string{header}
		if filterLine != "" {
			parts = append(parts, filterLine)
		}
		return lipgloss.JoinVertical(lipgloss.Left, append(parts, styleDim.Render(msg))...)
	}

	body := m.renderVideoRows(videos, cursor, vs, listH)
	parts := []string{header}
	if filterLine != "" {
		parts = append(parts, filterLine)
	}
	parts = append(parts, body)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) videoListTitleW() int {
	// indicator(2) + seps between cols; channel col adds itself + 1 sep when visible
	w := m.width - colNum - 1 - colDuration - colViews - colDate - 5
	if m.videoShowChannel() {
		w -= colChannel + 1
	}
	if w < 20 {
		w = 20
	}
	return w
}

func (m Model) renderVideoRows(videos []youtube.Video, cursor, vs, height int) string {
	if height <= 0 {
		height = 10
	}

	titleW := m.videoListTitleW()
	colHeader := m.renderVideoColHeader(titleW)
	windowH := height - 1
	if ps := m.pageSize(); ps < windowH {
		windowH = ps
	}
	start, end := scrollWindowAt(vs, len(videos), windowH)

	var rows []string
	rows = append(rows, colHeader)
	for i := start; i < end && i < len(videos); i++ {
		rows = append(rows, m.renderVideoRow(videos[i], i == cursor, titleW, i+1))
	}
	return strings.Join(rows, "\n")
}

func (m Model) renderVideoColHeader(titleW int) string {
	dateLabel := "Date"
	h := strings.Repeat(" ", colNum) + " " + "  " +
		styleColHeader.Width(titleW).Render("Title") + " "
	if m.videoShowChannel() {
		h += styleColHeader.Width(colChannel).Render("Channel") + " "
	}
	return h +
		styleColHeader.Width(colDuration).Render("Duration") + " " +
		styleColHeader.Width(colViews).Render("Views") + " " +
		styleColHeader.Width(colDate).Render(dateLabel)
}

func (m Model) renderVideoRow(v youtube.Video, selected bool, titleW, num int) string {
	lv, hasLocal := m.localVideoIDs[v.ID]

	title := truncate(v.Title, titleW)
	channel := truncate(v.Channel, colChannel-2)
	dur := v.DurationStr()
	if hasLocal && lv.Status == db.StatusStarted && lv.LastPositionMs > 0 {
		dur = fmtDurWithPos(lv.LastPositionMs, v.Duration)
	}
	views := v.ViewsStr()
	date := v.DateStr()

	indicator := "  "
	sep := " "
	numStyle := styleRowNum
	chStyle := styleChannel.Width(colChannel)
	durStyle := styleDuration.Width(colDuration)
	viewsStyle := styleDuration.Width(colViews)
	dateStyle := styleChannel.Width(colDate)
	var titleStyle lipgloss.Style

	switch {
	case selected:
		titleStyle = styleSelected.Width(titleW)
		indicator = styleSelected.Render("▶ ")
		numStyle = numStyle.Background(colorBgSelect)
		sep = lipgloss.NewStyle().Background(colorBgSelect).Render(" ")
		chStyle = chStyle.Background(colorBgSelect)
		durStyle = durStyle.Background(colorBgSelect)
		viewsStyle = viewsStyle.Background(colorBgSelect)
		dateStyle = dateStyle.Background(colorBgSelect)
	case hasLocal && lv.Status == db.StatusNew:
		titleStyle = styleBold.Width(titleW)
		indicator = styleSuccess.Render("● ")
	case hasLocal && (lv.Status == db.StatusStarted || lv.Status == db.StatusWatched):
		titleStyle = styleDim.Width(titleW)
		indicator = styleDim.Render("○ ")
	default:
		titleStyle = styleNormal.Width(titleW)
	}

	numStr := numStyle.Render(fmt.Sprintf("%*d", colNum, num))
	row := numStr + sep + indicator + titleStyle.Render(title) + sep
	if m.videoShowChannel() {
		row += chStyle.Render(channel) + sep
	}
	return row +
		durStyle.Render(dur) + sep +
		viewsStyle.Render(views) + sep +
		dateStyle.Render(date)
}

// ── Subscriptions ─────────────────────────────────────────────────────────────

func (m Model) renderSubscriptions(height int) string {
	if m.subMode == subModeAll {
		return m.renderVideoList(m.subVideos, m.subCursor, m.subVS, false, m.subChLoading && len(m.subVideos) == 0, height, "Subscriptions · All Videos")
	}

	headerText := "Subscriptions · Channels"
	if m.subChLoading {
		headerText += "  " + styleDim.Render(m.spinner.View()+" loading…")
	}
	header := styleSectionTitle.Render(headerText)
	headerH := lipgloss.Height(header)

	// Channel list pane
	if m.subChPane == 0 {
		var body string
		if m.subChLoading && len(m.subChannels) == 0 {
			body = m.spinner.View() + " Loading channels…"
		} else if len(m.subChannels) == 0 {
			body = styleDim.Render("No channels found.")
		} else {
			body = m.renderChannelList(height - headerH)
		}
		return lipgloss.JoinVertical(lipgloss.Left, header, body)
	}

	// Channel videos pane
	sorted := m.sortedChannels()
	chName := ""
	if m.subChCursor < len(sorted) {
		chName = sorted[m.subChCursor].Name
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
		vids, cur := m.contentVideos(m.subChVideos, m.subChVidCursor)
		body = m.renderVideoRows(vids, cur, m.subChVidVS, height-headerH-subH-filterH)
	}
	parts := []string{header, subHeader}
	if filterLine != "" {
		parts = append(parts, filterLine)
	}
	return lipgloss.JoinVertical(lipgloss.Left, append(parts, body)...)
}

const (
	colChName = 22 // channel name column in channels-mode list
	colSubs   = 8  // subscriber count column
)

func (m Model) renderChannelList(height int) string {
	channels := m.sortedChannels()
	if len(channels) == 0 {
		return ""
	}

	// Row layout: numStr(colNum+1) + indicator(2) + chName(colChName) + sep + subs(colSubs) + sep + title(W) + sep + dur(colDuration) + sep + views(colViews) + sep + date(colDate)
	titleW := m.width - colNum - 1 - 2 - colChName - 1 - colSubs - 1 - colDuration - 1 - colViews - 1 - colDate
	if titleW < 10 {
		titleW = 10
	}

	colHeader := strings.Repeat(" ", colNum) + " " + "  " +
		styleColHeader.Width(colChName).Render("Channel") + " " +
		styleColHeader.Width(colSubs).Render("Subs") + " " +
		styleColHeader.Width(titleW).Render("Latest Video") + " " +
		styleColHeader.Width(colDuration).Render("Duration") + " " +
		styleColHeader.Width(colViews).Render("Views") + " " +
		styleColHeader.Width(colDate).Render("Date")

	windowH := height - 1
	if ps := m.pageSize(); ps < windowH {
		windowH = ps
	}
	start, end := scrollWindowAt(m.subChVS, len(channels), windowH)
	rows := []string{colHeader}

	for i := start; i < end && i < len(channels); i++ {
		ch := channels[i]
		latest := m.subChLatest[ch.ID]
		selected := i == m.subChCursor

		chName := truncate(ch.Name, colChName-2)
		subs := fmtViews(ch.Subscribers)
		vidTitle := truncate(latest.Title, titleW)
		dur := latest.DurationStr()
		views := latest.ViewsStr()
		date := latest.DateStr()

		sep := " "
		numStyle := styleRowNum
		chStyle := styleNormal.Width(colChName)
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
			subsStyle = subsStyle.Background(colorBgSelect)
			titleStyle = styleSelected.Width(titleW)
			durStyle = durStyle.Background(colorBgSelect)
			viewsStyle = viewsStyle.Background(colorBgSelect)
			dateStyle = dateStyle.Background(colorBgSelect)
		}

		numStr := numStyle.Render(fmt.Sprintf("%*d ", colNum, i+1))
		rows = append(rows, numStr+indicator+
			chStyle.Render(chName)+sep+
			subsStyle.Render(subs)+sep+
			titleStyle.Render(vidTitle)+sep+
			durStyle.Render(dur)+sep+
			viewsStyle.Render(views)+sep+
			dateStyle.Render(date))
	}
	return strings.Join(rows, "\n")
}

// ── Playlists ─────────────────────────────────────────────────────────────────

func (m Model) renderPlaylists(height int) string {
	header := styleSectionTitle.Render("Playlists")
	headerH := lipgloss.Height(header)

	if m.createMode {
		prompt := styleInputPrompt.Render("New playlist name: ") + m.createInput.View()
		body := m.renderPlaylistRows(height-headerH-2) + "\n\n"
		return lipgloss.JoinVertical(lipgloss.Left, header, body+prompt)
	}

	if m.playlistPane == 1 && m.playlistCursor < m.playlistCount() {
		plName := m.selectedPlaylistName()
		subHeader := styleSectionTitle.Render("← " + plName)
		subH := lipgloss.Height(subHeader)

		plKey := m.selectedPlaylistKey()
		vids := m.playlistVidCache[plKey]
		body := ""
		switch {
		case len(vids) > 0:
			body = m.renderVideoRows(vids, m.playlistVidCursor, m.playlistVidVS, height-headerH-subH)
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
	start, end := scrollWindowAt(m.playlistVS, n, height)
	labelW := m.width - colNum - 1 - 4
	selW := m.width - colNum - 1
	var rows []string
	for i := start; i < end && i < n; i++ {
		var label string
		if m.ytPlLoaded && i < len(m.ytPlaylists) {
			label = m.ytPlaylists[i].Title
		} else if i < len(m.playlists) {
			label = m.playlists[i].Name
		}
		label = truncate(label, labelW)
		if i == m.playlistCursor {
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

// ── Search ────────────────────────────────────────────────────────────────────

func (m Model) renderSearch(height int) string {
	prompt := styleInputPrompt.Render("Search: ") + m.searchInput.View()
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
			vids, cur := m.contentVideos(m.searchChVideos, m.searchChVidCursor)
			body = m.renderVideoRows(vids, cur, m.searchChVidVS, remaining-subH-filterH)
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
			return lipgloss.JoinVertical(lipgloss.Left, prompt, styleDim.Render("No results for: "+m.lastQuery))
		}
		return lipgloss.JoinVertical(lipgloss.Left, prompt, styleDim.Render("Type to search YouTube"))
	}

	header := styleSectionTitle.Render("Results for: " + m.lastQuery)
	headerH := lipgloss.Height(header)
	listH := remaining - headerH

	cursor := m.searchCursor
	nCh := len(m.searchChannels)
	var rows []string

	// ── Channels section ──────────────────────────────────────────────────────
	if nCh > 0 {
		rows = append(rows, styleDim.Render("Channels"))
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
			rows = append(rows, styleDim.Render("Videos"))
		}
	}

	// ── Videos section ────────────────────────────────────────────────────────
	if len(m.searchVideos) > 0 {
		titleW := m.videoListTitleW()
		rows = append(rows, m.renderVideoColHeader(titleW))
		usedRows := len(rows)
		start, end := scrollWindowAt(m.searchVS, len(m.searchVideos), listH-usedRows)
		for i := start; i < end && i < len(m.searchVideos); i++ {
			rows = append(rows, m.renderVideoRow(m.searchVideos[i], cursor == nCh+i, titleW, i+1))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, prompt, header, strings.Join(rows, "\n"))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── Downloading ───────────────────────────────────────────────────────────────

func (m Model) renderDownloading(height int) string {
	header := styleSectionTitle.Render("Downloading")
	headerH := lipgloss.Height(header)

	items := m.downloader.Items()
	if len(items) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header,
			styleDim.Render("No active downloads. Press "+m.keys.Download.Help().Key+" on any video to start."))
	}

	titleW := m.width - colNum - 1 - colChannel - colDuration - 42 - 6
	if titleW < 20 {
		titleW = 20
	}
	colHeader := strings.Repeat(" ", colNum) + " " + "  " +
		styleColHeader.Width(titleW).Render("Title") + " " +
		styleColHeader.Width(colChannel).Render("Channel") + " " +
		styleColHeader.Width(colDuration).Render("Duration") + " " +
		styleColHeader.Render("Status")

	start, end := scrollWindowAt(m.dlVS, len(items), height-headerH-1)
	var rows []string
	rows = append(rows, colHeader)
	for i := start; i < end && i < len(items); i++ {
		rows = append(rows, m.renderDownloadRow(items[i], i == m.dlCursor, i+1))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(rows, "\n"))
}

func (m Model) renderDownloadRow(item downloader.Item, selected bool, num int) string {
	titleW := m.width - colNum - 1 - colChannel - colDuration - 42 - 6
	if titleW < 20 {
		titleW = 20
	}

	titleSuffix := ""
	if m.playAfterDownload[item.Video.ID] {
		titleSuffix = " ♪►"
	}
	title := truncate(item.Video.Title, titleW)
	channel := truncate(item.Video.Channel, colChannel-2)
	dur := item.Video.DurationStr()
	dlType := ""
	if item.Type == downloader.TypeAudio {
		dlType = styleChannel.Render("[audio]")
	}

	var statusPart string
	switch item.Status {
	case downloader.StatusPending:
		statusPart = stylePendingTag.Render("pending")
	case downloader.StatusActive:
		barW := 20
		bar := progressBar(item.Progress, barW)
		statusPart = fmt.Sprintf("%s %5.1f%%  %s  ETA %s",
			bar, item.Progress, styleActiveTag.Render(item.Speed), item.ETA)
	case downloader.StatusComplete:
		statusPart = styleCompleteTag.Render("done ✓")
	case downloader.StatusFailed:
		msg := "failed"
		if item.Err != nil {
			msg = "failed: " + truncate(item.Err.Error(), 30)
		}
		statusPart = styleFailedTag.Render(msg)
	}

	numStr := fmt.Sprintf("%*d", colNum, num)
	indicator := "  "
	if selected {
		indicator = styleSelected.Render("▶ ")
	}

	titleStyled := styleNormal.Width(titleW).Render(title + titleSuffix)
	if selected {
		titleStyled = styleSelected.Width(titleW).Render(title + titleSuffix)
	}

	return numStr + " " + indicator + titleStyled + " " +
		styleChannel.Width(colChannel).Render(channel) + " " +
		styleDuration.Width(colDuration).Render(dur) + " " +
		dlType + " " + statusPart
}

// ── Local ─────────────────────────────────────────────────────────────────────

func (m Model) renderLocal(height int) string {
	header := styleSectionTitle.Render("Local Library")
	headerH := lipgloss.Height(header)

	if len(m.localVideos) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header,
			styleDim.Render("No local videos. Download some with s."))
	}

	titleW := m.videoListTitleW()
	colHeader := strings.Repeat(" ", colNum) + " " + "  " +
		styleColHeader.Width(titleW).Render("Title") + " " +
		styleColHeader.Width(colChannel).Render("Channel") + " " +
		styleColHeader.Width(colDuration).Render("Duration") + " " +
		styleColHeader.Width(colViews).Render("Views") + " " +
		styleColHeader.Width(colDate).Render("Date")

	start, end := scrollWindowAt(m.localVS, len(m.localVideos), height-headerH-1)
	var rows []string
	rows = append(rows, colHeader)
	for i := start; i < end && i < len(m.localVideos); i++ {
		rows = append(rows, m.renderLocalRow(m.localVideos[i], i == m.localCursor, i+1))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(rows, "\n"))
}

func (m Model) renderLocalRow(lv db.LocalVideo, selected bool, num int) string {
	titleW := m.videoListTitleW()

	title := truncate(lv.Title, titleW)
	channel := truncate(lv.Channel, colChannel-2)
	dur := fmtDuration(lv.Duration)
	if lv.Status == db.StatusStarted && lv.LastPositionMs > 0 {
		dur = fmtDurWithPos(lv.LastPositionMs, lv.Duration)
	}
	views := fmtViews(lv.ViewCount)
	date := fmtDate(lv.UploadDate)
	dlType := ""
	if lv.DownloadType == "audio" {
		dlType = " ♪"
	}

	indicator := "  "
	sep := " "
	numStyle := styleRowNum
	chStyle := styleChannel.Width(colChannel)
	durStyle := styleDuration.Width(colDuration)
	viewsStyle := styleDuration.Width(colViews)
	dateStyle := styleChannel.Width(colDate)
	var ts lipgloss.Style

	switch {
	case selected:
		indicator = styleSelected.Render("▶ ")
		ts = styleSelected.Width(titleW)
		numStyle = numStyle.Background(colorBgSelect)
		sep = lipgloss.NewStyle().Background(colorBgSelect).Render(" ")
		chStyle = chStyle.Background(colorBgSelect)
		durStyle = durStyle.Background(colorBgSelect)
		viewsStyle = viewsStyle.Background(colorBgSelect)
		dateStyle = dateStyle.Background(colorBgSelect)
	case lv.Status == db.StatusNew:
		ts = styleBold.Width(titleW)
		indicator = styleSuccess.Render("● ")
	case lv.Status == db.StatusStarted:
		ts = styleNormal.Width(titleW)
		indicator = styleDim.Render("○ ")
	case lv.Status == db.StatusWatched:
		ts = styleDim.Width(titleW)
		indicator = styleDim.Render("  ")
	default:
		ts = styleNormal.Width(titleW)
	}

	numStr := numStyle.Render(fmt.Sprintf("%*d", colNum, num))
	return numStr + sep + indicator +
		ts.Render(title+dlType) + sep +
		chStyle.Render(channel) + sep +
		durStyle.Render(dur) + sep +
		viewsStyle.Render(views) + sep +
		dateStyle.Render(date)
}

// ── History ───────────────────────────────────────────────────────────────────

func (m Model) renderHistory(height int) string {
	if m.histDetailVideoID != "" {
		return m.renderHistoryDetail(height)
	}

	header := styleSectionTitle.Render("History")
	headerH := lipgloss.Height(header)

	if len(m.histEntries) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header,
			styleDim.Render("No history yet."))
	}

	// Summary: one row per video. Columns: num indicator status title channel dur views date
	colStatus := 14
	titleW := m.width - colNum - 1 - 2 - colStatus - 1 - colChannel - 1 - colDuration - 1 - colViews - 1 - colDate
	if titleW < 20 {
		titleW = 20
	}

	start, end := scrollWindowAt(m.histVS, len(m.histEntries), height-headerH)
	var rows []string
	for i := start; i < end && i < len(m.histEntries); i++ {
		e := m.histEntries[i]

		indicator := "  "
		sep := " "
		numStyle := styleRowNum
		statusStyle := styleWarning.Width(colStatus)
		if i == m.histCursor {
			indicator = styleSelected.Render("▶ ")
			numStyle = numStyle.Background(colorBgSelect)
			sep = lipgloss.NewStyle().Background(colorBgSelect).Render(" ")
			statusStyle = statusStyle.Background(colorBgSelect)
		}
		numStr := numStyle.Render(fmt.Sprintf("%*d", colNum, i+1))

		if e.EventType == "search" {
			queryW := m.width - colNum - 1 - 2 - colStatus - 1
			queryStyle := styleChannel.Width(queryW)
			if i == m.histCursor {
				queryStyle = styleSelected.Width(queryW)
			}
			rows = append(rows, numStr+sep+indicator+statusStyle.Render("search")+sep+
				queryStyle.Render(truncate(e.Details, queryW)))
			continue
		}

		title := truncate(e.Title, titleW)
		channel := truncate(e.Channel, colChannel-2)
		dur := fmtDuration(e.Duration)
		views := fmtViews(e.ViewCount)
		date := fmtDate(e.UploadDate)

		titleStyle := styleNormal.Width(titleW)
		chStyle := styleChannel.Width(colChannel)
		durStyle := styleDuration.Width(colDuration)
		viewsStyle := styleDuration.Width(colViews)
		dateStyle := styleChannel.Width(colDate)
		if i == m.histCursor {
			titleStyle = styleSelected.Width(titleW)
			chStyle = chStyle.Background(colorBgSelect)
			durStyle = durStyle.Background(colorBgSelect)
			viewsStyle = viewsStyle.Background(colorBgSelect)
			dateStyle = dateStyle.Background(colorBgSelect)
		}
		rows = append(rows, numStr+sep+indicator+statusStyle.Render(e.EventType)+sep+
			titleStyle.Render(title)+sep+
			chStyle.Render(channel)+sep+
			durStyle.Render(dur)+sep+
			viewsStyle.Render(views)+sep+
			dateStyle.Render(date))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(rows, "\n"))
}

func (m Model) renderHistoryDetail(height int) string {
	title := ""
	if len(m.histDetail) > 0 {
		title = m.histDetail[0].Title
	}
	header := styleSectionTitle.Render("← " + truncate(title, m.width-4))
	headerH := lipgloss.Height(header)

	colEvW := 14
	colTsW := 19
	var rows []string
	for i, e := range m.histDetail {
		if i >= height-headerH {
			break
		}
		evType := styleWarning.Width(colEvW).Render(e.EventType)
		ts := styleChannel.Width(colTsW).Render(e.Timestamp.Format("2006-01-02 15:04:05"))
		detail := styleDim.Render(e.Details)
		rows = append(rows, "  "+evType+" "+ts+" "+detail)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(rows, "\n"))
}

// ── Video detail overlay ──────────────────────────────────────────────────────

const vidDetailPanelW = 52

func (m Model) renderVideoDetailPanel(panelW, panelH, thumbH int) string {
	innerW := panelW - 2
	accent := lipgloss.NewStyle().Foreground(colorAccent)
	norm := func(s string) string { return styleNormal.Width(innerW).Render(s) }

	// inner rows (excluding borders); last 2 rows are always the footer.
	inner := panelH - 2
	const footerH = 2
	contentRows := inner - footerH

	var lines []string
	needsScroll := false

	if m.vidDetailLoading {
		lines = append(lines, norm(m.spinner.View()+" Loading…"))
	} else if m.vidDetailVideo != nil {
		v := m.vidDetailVideo
		thumbW := innerW

		// Thumbnail area. Kitty terminals get blank space here; the actual image
		// is placed by kittyImageOverlay() appended to the View() output.
		// Non-Kitty terminals get half-block rendering inline.
		if kittyCapable() {
			for i := 0; i < thumbH; i++ {
				lines = append(lines, norm(""))
			}
		} else {
			var thumbLines []string
			if rendered := renderThumbnail(m.vidDetailThumb, thumbW, thumbH); rendered != "" {
				thumbLines = strings.Split(rendered, "\n")
			}
			for len(thumbLines) < thumbH {
				thumbLines = append(thumbLines, strings.Repeat("░", thumbW))
			}
			lines = append(lines, thumbLines...)
		}

		// Title (word-wrapped, up to 3 lines).
		lines = append(lines, norm(""))
		for i, tl := range wordWrap(v.Title, innerW) {
			if i >= 3 {
				break
			}
			lines = append(lines, norm(tl))
		}

		// Metadata.
		lbl := styleDim
		meta := func(k, val string) string {
			return styleNormal.Width(innerW).Render(lbl.Render(k) + val)
		}
		lines = append(lines, norm(""))
		lines = append(lines, meta("Channel  ", truncate(v.Channel, innerW-9)))
		if v.Subscribers > 0 {
			lines = append(lines, meta("Subs     ", fmtViews(v.Subscribers)))
		}
		lines = append(lines, meta("Views    ", v.ViewsStr()))
		lines = append(lines, meta("Duration ", v.DurationStr()))
		lines = append(lines, meta("Date     ", v.DateStr()))
		lines = append(lines, styleHelp.Width(innerW).Render(""))
		lines = append(lines, styleHelp.Width(innerW).Render(truncate(v.URL, innerW)))

		// Description — fills remaining content rows.
		if v.Description != "" {
			lines = append(lines, styleColHeader.Width(innerW).Render(""))
			lines = append(lines, styleColHeader.Width(innerW).Render("Description"))
			available := contentRows - len(lines)
			if available > 0 {
				descLines := wordWrap(v.Description, innerW)
				needsScroll = len(descLines) > available
				maxVS := len(descLines) - 1
				if maxVS < 0 {
					maxVS = 0
				}
				vs := m.vidDetailDescVS
				if vs > maxVS {
					vs = maxVS
				}
				visible := descLines[vs:]
				if len(visible) > available {
					visible = visible[:available]
				}
				for _, dl := range visible {
					lines = append(lines, norm(dl))
				}
			}
		}
	}

	// Trim / pad content to exactly contentRows (footer is appended separately).
	for len(lines) < contentRows {
		lines = append(lines, norm(""))
	}
	lines = lines[:contentRows]

	// Footer — always pinned to the last two rows of the panel.
	footerText := "esc: close"
	if needsScroll {
		footerText = "j/k: scroll  esc: close"
	}
	lines = append(lines, styleHelp.Width(innerW).Render(""))
	lines = append(lines, styleHelp.Width(innerW).Render(footerText))

	// Assemble bordered box.
	top := accent.Render("╭─ Video Details " + strings.Repeat("─", innerW-16) + "╮")
	bot := accent.Render("╰" + strings.Repeat("─", innerW) + "╯")
	rows := make([]string, 0, panelH)
	rows = append(rows, top)
	for _, l := range lines {
		rows = append(rows, accent.Render("│")+l+accent.Render("│"))
	}
	rows = append(rows, bot)
	return strings.Join(rows, "\n")
}

// wordWrap splits text into lines of at most width visible characters,
// breaking at word boundaries (spaces). Existing newlines are preserved.
func wordWrap(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var result []string
	for _, para := range strings.Split(text, "\n") {
		if lipgloss.Width(para) <= width {
			result = append(result, para)
			continue
		}
		words := strings.Fields(para)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			candidate := line + " " + w
			if lipgloss.Width(candidate) <= width {
				line = candidate
			} else {
				result = append(result, line)
				line = w
			}
		}
		result = append(result, line)
	}
	return result
}

// ── Add-to-playlist overlay ───────────────────────────────────────────────────

func (m Model) renderAddOverlay(behind string) string {
	lines := []string{
		styleHeader.Render("Add to playlist"),
		"",
	}
	if m.ytPlLoaded && m.ytClient != nil {
		for i, pl := range m.ytPlaylists {
			label := "  " + pl.Title
			if m.addOverlaySel == i {
				label = styleSelected.Render("▶ " + pl.Title)
			}
			lines = append(lines, label)
		}
	} else {
		for i, pl := range m.playlists {
			label := "  " + pl.Name
			if m.addOverlaySel == i {
				label = styleSelected.Render("▶ " + pl.Name)
			}
			lines = append(lines, label)
		}
	}
	lines = append(lines, "", styleHelp.Render("j/k: move  enter: confirm  esc: cancel"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 2).
		Width(40).
		Render(strings.Join(lines, "\n"))

	// Center the overlay
	bw := lipgloss.Width(box)
	bh := lipgloss.Height(box)
	x := (m.width - bw) / 2
	if x < 0 {
		x = 0
	}

	behindLines := strings.Split(behind, "\n")
	totalLines := len(behindLines)
	y := (totalLines - bh) / 2
	if y < 0 {
		y = 0
	}

	overlayLines := strings.Split(box, "\n")
	for i, ol := range overlayLines {
		lineIdx := y + i
		if lineIdx >= len(behindLines) {
			behindLines = append(behindLines, "")
		}
		base := behindLines[lineIdx]
		baseRunes := []rune(base)
		// Pad base to width x
		for len(baseRunes) < x {
			baseRunes = append(baseRunes, ' ')
		}
		merged := string(baseRunes[:x]) + ol
		behindLines[lineIdx] = merged
	}
	return strings.Join(behindLines, "\n")
}

// ── Help overlay ──────────────────────────────────────────────────────────────

func (m Model) renderHelp(height int) string {
	lines := []string{
		styleHeader.Render("Keyboard Shortcuts"),
		"",
		styleHelp.Render("Navigation"),
		"  j / k / ↑ / ↓  " + "Move cursor",
		"  h / l          " + "Left / right pane",
		"  Ctrl+D / Ctrl+U" + "Page down / up",
		"  gg / G         " + "Go to top / bottom",
		"  {n}G           " + "Jump to row n",
		"  Tab / Shift+Tab" + "Next / prev tab",
		"  tr ts tp t/ td tl th" + "  Switch to tab by chord",
		"",
		styleHelp.Render("Video Actions"),
		"  d              " + "Download video",
		"  D              " + "Download audio",
		"  p              " + "Play local video / queue play after download",
		"  x              " + "Delete local video / playlist entry",
		"  c              " + "Copy video URL to clipboard",
		"  b              " + "Hide video from recommended",
		"  B              " + "Hide channel from recommended",
		"  r              " + "Refresh",
		"  w              " + "Add to Watch Later",
		"  a              " + "Add to playlist",
		"  S              " + "Subscribe to channel",
		"",
		styleHelp.Render("Sorting"),
		"  sd             " + "Sort by date",
		"  sv             " + "Sort by views",
		"  sn             " + "Sort by name",
		"  sc             " + "Sort by channel name",
		"  sD             " + "Sort by duration",
		"  ss             " + "Sort by subscribers",
		"",
		styleHelp.Render("General"),
		"  /              " + "Local search",
		"  m              " + "Toggle subscriptions mode",
		"  n              " + "New playlist",
		"  r              " + "Refresh",
		"  ?              " + "Toggle this help",
		"  q              " + "Quit",
		"",
		styleHelp.Render("Video states (in all lists)"),
		"  " + styleSuccess.Render("●") + " bold        " + "Downloaded, not watched",
		"  " + styleDim.Render("○") + " dim         " + "Started or watched",
	}

	content := strings.Join(lines, "\n")
	if height > 0 {
		// trim to available height
		split := strings.Split(content, "\n")
		if len(split) > height {
			split = split[:height]
		}
		content = strings.Join(split, "\n")
	}
	return content
}

// ── Utilities ─────────────────────────────────────────────────────────────────

func fmtDuration(secs int) string {
	if secs <= 0 {
		return ""
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func fmtViews(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	case n > 0:
		return fmt.Sprintf("%d", n)
	}
	return ""
}

func fmtDurWithPos(posMs int64, totalSecs int) string {
	return fmtDuration(int(posMs/1000)) + "/" + fmtDuration(totalSecs)
}

func fmtDate(yyyymmdd string) string {
	if len(yyyymmdd) != 8 {
		return yyyymmdd
	}
	return yyyymmdd[6:] + "/" + yyyymmdd[4:6] + "/" + yyyymmdd[:4]
}

// scrollWindowAt returns [start, end) anchored at viewStart (nvim-style).
func scrollWindowAt(vs, n, height int) (int, int) {
	if n == 0 || height <= 0 {
		return 0, 0
	}
	if height >= n {
		return 0, n
	}
	start := vs
	end := start + height
	if end > n {
		end = n
		start = end - height
		if start < 0 {
			start = 0
		}
	}
	return start, end
}
