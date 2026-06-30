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

	// Available height for content
	reservedLines := lipgloss.Height(tabBar) + lipgloss.Height(status)
	if m.showHelp {
		return lipgloss.JoinVertical(lipgloss.Left,
			tabBar,
			m.renderHelp(m.height-reservedLines),
			status,
		)
	}

	content := m.renderContent(m.height - reservedLines)

	if m.addOverlay {
		content = m.renderAddOverlay(content)
	}

	return lipgloss.JoinVertical(lipgloss.Left, tabBar, content, status)
}

// ── Tab bar ───────────────────────────────────────────────────────────────────

var tabKeys = [7]string{"F2", "F3", "F4", "F5", "F6", "F7", "F8"}

func (m Model) renderTabBar() string {
	var tabs []string
	for i, id := range m.tabs {
		fkey := ""
		if i < len(tabKeys) {
			fkey = tabKeys[i]
		}
		label := fmt.Sprintf("[%s] %s", fkey, tabNames[id])
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
	var left string
	if m.status != "" && time.Since(m.statusAt) < 5*time.Second {
		if m.statusErr {
			left = styleError.Render("✗ " + m.status)
		} else {
			left = styleSuccess.Render("✓ " + m.status)
		}
	} else {
		left = m.contextHelp()
	}
	right := styleHelp.Render("? help  q quit")
	space := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if space < 1 {
		space = 1
	}
	return left + strings.Repeat(" ", space) + right
}

func (m Model) contextHelp() string {
	switch m.activeTab {
	case tabRecommended, tabSearch:
		return styleHelp.Render("j/k: move  s: dl video  S: dl audio  w: watch later  a: playlist")
	case tabSubscriptions:
		mode := "all videos"
		if m.subMode == subModeChannels {
			mode = "channels"
		}
		return styleHelp.Render(fmt.Sprintf("j/k: move  t: toggle mode (%s)  s: dl video  S: dl audio", mode))
	case tabPlaylists:
		return styleHelp.Render("j/k: move  enter: open  n: new  d: delete")
	case tabDownloading:
		return styleHelp.Render("j/k: move")
	case tabLocal:
		return styleHelp.Render("j/k: move  p: play  d: delete")
	case tabHistory:
		return styleHelp.Render("j/k: move  r: refresh")
	}
	return ""
}

// ── Content router ────────────────────────────────────────────────────────────

func (m Model) renderContent(height int) string {
	switch m.activeTab {
	case tabRecommended:
		return m.renderVideoList(m.recVideos, m.recCursor, m.recLoading, m.recRefreshing, height, "Recommended for you")
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
	colChannel  = 22
	colDuration = 8
	colViews    = 8
	colDate     = 11
)

func (m Model) renderVideoList(
	videos []youtube.Video,
	cursor int,
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
	listH := height - headerH

	if loading && !refreshing {
		body := m.spinner.View() + " Loading…"
		return lipgloss.JoinVertical(lipgloss.Left, header, body)
	}
	if len(videos) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header, styleDim.Render("No videos. Press r to refresh."))
	}

	body := m.renderVideoRows(videos, cursor, listH)
	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}

func (m Model) renderVideoRows(videos []youtube.Video, cursor, height int) string {
	if height <= 0 {
		height = 10
	}

	titleW := m.width - colChannel - colDuration - colViews - colDate - 4
	if titleW < 20 {
		titleW = 20
	}

	colHeader := m.renderVideoColHeader(titleW)
	start, end := scrollWindow(cursor, len(videos), height-1)

	var rows []string
	rows = append(rows, colHeader)
	for i := start; i < end && i < len(videos); i++ {
		v := videos[i]
		rows = append(rows, m.renderVideoRow(v, i == cursor, titleW))
	}
	return strings.Join(rows, "\n")
}

func (m Model) renderVideoColHeader(titleW int) string {
	return "  " +
		styleColHeader.Width(titleW).Render("Title") + " " +
		styleColHeader.Width(colChannel).Render("Channel") + " " +
		styleColHeader.Width(colDuration).Render("Dur") + " " +
		styleColHeader.Width(colViews).Render("Views") + " " +
		styleColHeader.Width(colDate).Render("Date")
}

func (m Model) renderVideoRow(v youtube.Video, selected bool, titleW int) string {
	// Status indicator
	indicator := "  "
	lv, hasLocal := m.db.HasLocalVideo(v.ID)

	title := truncate(v.Title, titleW)
	channel := truncate(v.Channel, colChannel-2)
	dur := v.DurationStr()
	views := v.ViewsStr()
	date := v.DateStr()

	var titleStyle lipgloss.Style
	switch {
	case selected:
		titleStyle = styleSelected.Width(titleW)
		indicator = styleSelected.Render("▶ ")
	case hasLocal && lv.Status == db.StatusNew:
		titleStyle = styleBold.Width(titleW)
		indicator = styleSuccess.Render("● ")
	case hasLocal && (lv.Status == db.StatusStarted || lv.Status == db.StatusWatched):
		titleStyle = styleDim.Width(titleW)
		indicator = styleDim.Render("○ ")
	default:
		titleStyle = styleNormal.Width(titleW)
	}

	return indicator +
		titleStyle.Render(title) + " " +
		styleChannel.Width(colChannel).Render(channel) + " " +
		styleDuration.Width(colDuration).Render(dur) + " " +
		styleDuration.Width(colViews).Render(views) + " " +
		styleChannel.Width(colDate).Render(date)
}

// ── Subscriptions ─────────────────────────────────────────────────────────────

func (m Model) renderSubscriptions(height int) string {
	modeLabel := "All Videos"
	if m.subMode == subModeChannels {
		modeLabel = "Channels"
	}
	header := styleSectionTitle.Render("Subscriptions · " + modeLabel + "  [t: toggle]")
	headerH := lipgloss.Height(header)

	if m.subMode == subModeAll {
		subTitle := "Subscriptions · All Videos  [t: toggle]"
		return m.renderVideoList(m.subVideos, m.subCursor, m.subLoading, m.subRefreshing, height, subTitle)
	}

	// Channel mode
	if m.subChPane == 0 {
		// Channel list
		body := ""
		if m.subChLoading {
			body = m.spinner.View() + " Loading channels…"
		} else if len(m.subChannels) == 0 {
			body = styleDim.Render("No channels found.")
		} else {
			body = m.renderChannelList(height - headerH)
		}
		return lipgloss.JoinVertical(lipgloss.Left, header, body)
	}

	// Channel videos
	chName := ""
	if m.subChCursor < len(m.subChannels) {
		chName = m.subChannels[m.subChCursor].Name
	}
	subHeader := styleSectionTitle.Render("← " + chName + "  [h/esc: back]")
	subH := lipgloss.Height(subHeader)

	body := ""
	if m.subChVidLoading {
		body = m.spinner.View() + " Loading…"
	} else {
		body = m.renderVideoRows(m.subChVideos, m.subChVidCursor, height-headerH-subH)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, subHeader, body)
}

func (m Model) renderChannelList(height int) string {
	channels := m.subChannels
	if len(channels) == 0 {
		return ""
	}
	start, end := scrollWindow(m.subChCursor, len(channels), height)
	var rows []string
	for i := start; i < end && i < len(channels); i++ {
		ch := channels[i]
		name := truncate(ch.Name, m.width-4)
		if i == m.subChCursor {
			rows = append(rows, styleSelected.Render("▶ "+name))
		} else {
			rows = append(rows, "  "+name)
		}
	}
	return strings.Join(rows, "\n")
}

// ── Playlists ─────────────────────────────────────────────────────────────────

func (m Model) renderPlaylists(height int) string {
	header := styleSectionTitle.Render("Playlists  [n: new  d: delete  enter: open]")
	headerH := lipgloss.Height(header)

	if m.createMode {
		prompt := styleInputPrompt.Render("New playlist name: ") + m.createInput.View()
		body := ""
		if len(m.playlists) > 0 {
			body = m.renderPlaylistRows(height-headerH-2) + "\n\n"
		}
		return lipgloss.JoinVertical(lipgloss.Left, header, body+prompt)
	}

	if m.playlistPane == 1 && m.playlistCursor < len(m.playlists) {
		pl := m.playlists[m.playlistCursor]
		subHeader := styleSectionTitle.Render("← " + pl.Name + "  [h/esc: back  d: remove]")
		subH := lipgloss.Height(subHeader)

		vids := m.playlistVidCache[pl.ID]
		body := ""
		if len(vids) == 0 {
			body = styleDim.Render("Empty playlist. Add videos with 'a' from other tabs.")
		} else {
			body = m.renderVideoRows(vids, m.playlistVidCursor, height-headerH-subH)
		}
		return lipgloss.JoinVertical(lipgloss.Left, header, subHeader, body)
	}

	if len(m.playlists) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header,
			styleDim.Render("No playlists yet. Press n to create one."))
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, m.renderPlaylistRows(height-headerH))
}

func (m Model) renderPlaylistRows(height int) string {
	playlists := m.playlists
	start, end := scrollWindow(m.playlistCursor, len(playlists), height)
	var rows []string
	for i := start; i < end && i < len(playlists); i++ {
		pl := playlists[i]
		label := truncate(pl.Name, m.width-4)
		if i == m.playlistCursor {
			rows = append(rows, styleSelected.Render("▶ "+label))
		} else {
			rows = append(rows, "  "+label)
		}
	}
	return strings.Join(rows, "\n")
}

// ── Search ────────────────────────────────────────────────────────────────────

func (m Model) renderSearch(height int) string {
	prompt := styleInputPrompt.Render("Search: ") + m.searchInput.View()
	promptH := 1

	body := ""
	if m.searchLoading {
		body = m.spinner.View() + " Searching…"
	} else if len(m.searchVideos) == 0 && m.lastQuery != "" {
		body = styleDim.Render("No results for: " + m.lastQuery)
	} else if len(m.searchVideos) == 0 {
		body = styleDim.Render("Press / or F5 to search YouTube")
	} else {
		header := styleSectionTitle.Render("Results for: " + m.lastQuery)
		headerH := lipgloss.Height(header)
		rows := m.renderVideoRows(m.searchVideos, m.searchCursor, height-promptH-1-headerH)
		body = lipgloss.JoinVertical(lipgloss.Left, header, rows)
	}

	return lipgloss.JoinVertical(lipgloss.Left, prompt, body)
}

// ── Downloading ───────────────────────────────────────────────────────────────

func (m Model) renderDownloading(height int) string {
	header := styleSectionTitle.Render("Downloading")
	headerH := lipgloss.Height(header)

	items := m.downloader.Items()
	if len(items) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header,
			styleDim.Render("No active downloads. Press s on any video to start."))
	}

	titleW := m.width - colChannel - colDuration - 42 - 4
	if titleW < 20 {
		titleW = 20
	}
	colHeader := "  " +
		styleColHeader.Width(titleW).Render("Title") + " " +
		styleColHeader.Width(colChannel).Render("Channel") + " " +
		styleColHeader.Width(colDuration).Render("Dur") + " " +
		styleColHeader.Render("Status")

	start, end := scrollWindow(m.dlCursor, len(items), height-headerH-1)
	var rows []string
	rows = append(rows, colHeader)
	for i := start; i < end && i < len(items); i++ {
		item := items[i]
		rows = append(rows, m.renderDownloadRow(item, i == m.dlCursor))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(rows, "\n"))
}

func (m Model) renderDownloadRow(item downloader.Item, selected bool) string {
	titleW := m.width - colChannel - colDuration - 42 - 4
	if titleW < 20 {
		titleW = 20
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

	indicator := "  "
	if selected {
		indicator = styleSelected.Render("▶ ")
	}

	titleStyled := styleNormal.Width(titleW).Render(title)
	if selected {
		titleStyled = styleSelected.Width(titleW).Render(title)
	}

	return indicator + titleStyled + " " +
		styleChannel.Width(colChannel).Render(channel) + " " +
		styleDuration.Width(colDuration).Render(dur) + " " +
		dlType + " " + statusPart
}

// ── Local ─────────────────────────────────────────────────────────────────────

func (m Model) renderLocal(height int) string {
	header := styleSectionTitle.Render("Local Library  [p: play  d: delete]")
	headerH := lipgloss.Height(header)

	if len(m.localVideos) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header,
			styleDim.Render("No local videos. Download some with s."))
	}

	titleW := m.width - colChannel - colDuration - colViews - colDate - 4
	if titleW < 20 {
		titleW = 20
	}
	colHeader := "  " +
		styleColHeader.Width(titleW).Render("Title") + " " +
		styleColHeader.Width(colChannel).Render("Channel") + " " +
		styleColHeader.Width(colDuration).Render("Dur") + " " +
		styleColHeader.Width(colViews).Render("Views") + " " +
		styleColHeader.Width(colDate).Render("Date")

	start, end := scrollWindow(m.localCursor, len(m.localVideos), height-headerH-1)
	var rows []string
	rows = append(rows, colHeader)
	for i := start; i < end && i < len(m.localVideos); i++ {
		lv := m.localVideos[i]
		rows = append(rows, m.renderLocalRow(lv, i == m.localCursor))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(rows, "\n"))
}

func (m Model) renderLocalRow(lv db.LocalVideo, selected bool) string {
	titleW := m.width - colChannel - colDuration - colViews - colDate - 4
	if titleW < 20 {
		titleW = 20
	}

	title := truncate(lv.Title, titleW)
	channel := truncate(lv.Channel, colChannel-2)
	dur := fmtDuration(lv.Duration)
	views := fmtViews(lv.ViewCount)
	date := fmtDate(lv.UploadDate)
	dlType := ""
	if lv.DownloadType == "audio" {
		dlType = " ♪"
	}

	indicator := "  "
	var ts lipgloss.Style
	switch {
	case selected:
		indicator = styleSelected.Render("▶ ")
		ts = styleSelected.Width(titleW)
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

	return indicator +
		ts.Render(title+dlType) + " " +
		styleChannel.Width(colChannel).Render(channel) + " " +
		styleDuration.Width(colDuration).Render(dur) + " " +
		styleDuration.Width(colViews).Render(views) + " " +
		styleChannel.Width(colDate).Render(date)
}

// ── History ───────────────────────────────────────────────────────────────────

func (m Model) renderHistory(height int) string {
	header := styleSectionTitle.Render("History  [r: refresh]")
	headerH := lipgloss.Height(header)

	if len(m.histEntries) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header,
			styleDim.Render("No history yet."))
	}

	start, end := scrollWindow(m.histCursor, len(m.histEntries), height-headerH)
	titleW := m.width - 19 - 8 - colChannel - colDuration - colViews - colDate - 8
	if titleW < 20 {
		titleW = 20
	}

	var rows []string
	for i := start; i < end && i < len(m.histEntries); i++ {
		e := m.histEntries[i]
		ts := styleChannel.Width(19).Render(e.Timestamp.Format("2006-01-02 15:04:05"))
		evType := styleWarning.Width(8).Render(e.EventType)
		title := truncate(e.Title, titleW)
		channel := truncate(e.Channel, colChannel-2)
		dur := fmtDuration(e.Duration)
		views := fmtViews(e.ViewCount)
		date := fmtDate(e.UploadDate)

		indicator := "  "
		style := styleNormal.Width(titleW)
		if i == m.histCursor {
			indicator = styleSelected.Render("▶ ")
			style = styleSelected.Width(titleW)
		}
		rows = append(rows, indicator+ts+" "+evType+" "+style.Render(title)+" "+
			styleChannel.Width(colChannel).Render(channel)+" "+
			styleDuration.Width(colDuration).Render(dur)+" "+
			styleDuration.Width(colViews).Render(views)+" "+
			styleChannel.Width(colDate).Render(date))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(rows, "\n"))
}

// ── Add-to-playlist overlay ───────────────────────────────────────────────────

func (m Model) renderAddOverlay(behind string) string {
	lines := []string{
		styleHeader.Render("Add to playlist"),
		"",
	}
	// Option 0: Watch Later
	wlLabel := "  Watch Later"
	if m.addOverlaySel == 0 {
		wlLabel = styleSelected.Render("▶ Watch Later")
	}
	lines = append(lines, wlLabel)
	for i, pl := range m.playlists {
		label := "  " + pl.Name
		if m.addOverlaySel == i+1 {
			label = styleSelected.Render("▶ " + pl.Name)
		}
		lines = append(lines, label)
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
		"  Tab / Shift+Tab" + "Next / prev tab",
		"  F2–F8          " + "Direct tab access",
		"",
		styleHelp.Render("Video Actions"),
		"  s              " + "Download video (with SponsorBlock)",
		"  S              " + "Download audio only",
		"  p              " + "Play local video",
		"  d              " + "Delete local video",
		"  w              " + "Add to Watch Later",
		"  a              " + "Add to playlist",
		"",
		styleHelp.Render("General"),
		"  /              " + "Search YouTube",
		"  t              " + "Toggle subscriptions mode",
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

func fmtDate(yyyymmdd string) string {
	if len(yyyymmdd) != 8 {
		return yyyymmdd
	}
	return yyyymmdd[6:] + "/" + yyyymmdd[4:6] + "/" + yyyymmdd[:4]
}

// scrollWindow returns [start, end) for a cursor within a list of n items, fitting in height rows.
func scrollWindow(cursor, n, height int) (int, int) {
	if n == 0 || height <= 0 {
		return 0, 0
	}
	if height >= n {
		return 0, n
	}
	// Keep cursor in middle third where possible
	half := height / 2
	start := cursor - half
	if start < 0 {
		start = 0
	}
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
