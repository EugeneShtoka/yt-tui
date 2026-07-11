package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	runewidth "github.com/mattn/go-runewidth"
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
		_, thumbH := m.thumbDimensions()

		m.width -= vidDetailPanelW
		left := m.renderContent(contentH)
		if m.addOverlay {
			left = m.renderAddOverlay(left)
		}
		m.width += vidDetailPanelW
		panel := m.renderVideoDetailPanel(vidDetailPanelW, contentH, thumbH)
		content = lipgloss.JoinHorizontal(lipgloss.Top, left, panel)
		if m.vidDetailKittyOverlay != "" {
			kittyOverlay = m.vidDetailKittyOverlay
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

	if m.linkOverlay {
		content = m.renderLinkOverlay(content)
	}
	if m.chapterOverlay {
		content = m.renderChapterOverlay(content)
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

	kb := m.cfg.Keybindings
	right := styleHelp.Render(kh + ": help  " + kq + ": quit")

	// Non-context-help states (chords, status messages) always render single-row.
	var fixed string
	switch {
	case m.cmdMode:
		cmdView := ":" + m.cmdInput.View()
		space := m.width - 1 - lipgloss.Width(cmdView) - lipgloss.Width(right)
		if space < 1 {
			space = 1
		}
		return cmdView + strings.Repeat(" ", space) + right
	case m.pendingChord != "":
		fixed = styleWarning.Render(m.chordHint())
	case m.gPending && !m.vidDetailOverlay && !m.linkOverlay && !m.chapterOverlay && !m.addOverlay:
		fixed = styleWarning.Render(kb.GotoPrefix + " → " + kb.GotoPrefix + ": top")
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
		space := m.width - lipgloss.Width(fixed) - lipgloss.Width(right)
		if space < 1 {
			space = 1
		}
		return fixed + strings.Repeat(" ", space) + right
	}

	// Try full context hints; fall back to minimal if they don't fit on one line.
	helpRaw := m.contextHelpRaw()
	if lipgloss.Width(styleHelp.Render(helpRaw))+1+lipgloss.Width(right) > m.width {
		helpRaw = m.minimalHintRaw()
	}
	left := styleHelp.Render(helpRaw)
	space := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if space < 1 {
		space = 1
	}
	return left + strings.Repeat(" ", space) + right
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
	return fmt.Sprintf("j/k: move  %s: tab  %s: stream", kb.TabChord, play)
}

// hintEntry is a single "key: label" pair for the status bar hint.
type hintEntry struct{ key, label string }

// hintFn resolves hint entries for the current model state.
type hintFn func(m Model) []hintEntry

func hintFixed(k, label string) hintFn {
	return func(Model) []hintEntry { return []hintEntry{{k, label}} }
}

func hintK(fn func(km keyMap) key.Binding, label string) hintFn {
	return func(m Model) []hintEntry { return []hintEntry{{fn(m.keys).Help().Key, label}} }
}

// hintRegistry defines every possible hint action exactly once.
var hintRegistry = map[string]hintFn{
	"move": func(m Model) []hintEntry {
		return []hintEntry{{m.keys.Down.Help().Key + "/" + m.keys.Up.Help().Key, "move"}}
	},
	"back": func(m Model) []hintEntry {
		return []hintEntry{{m.keys.Left.Help().Key, "back"}}
	},
	"chords": func(m Model) []hintEntry {
		ctx := m.currentContext()
		var out []hintEntry
		for _, chord := range m.chordDefs() {
			if len(validActions(chord.actions, ctx)) > 0 {
				out = append(out, hintEntry{chord.trigger, chord.name})
			}
		}
		return out
	},
	"play":           hintK(func(km keyMap) key.Binding { return km.Play }, "stream video"),
	"play_audio":     hintK(func(km keyMap) key.Binding { return km.PlayAudio }, "stream audio"),
	"download":       hintK(func(km keyMap) key.Binding { return km.Download }, "download video"),
	"download_audio": hintK(func(km keyMap) key.Binding { return km.DownloadAudio }, "download audio"),
	"copy_url":       hintK(func(km keyMap) key.Binding { return km.CopyURL }, "copy url"),
	"info":           hintK(func(km keyMap) key.Binding { return km.VideoInfo }, "info"),
	"hide_video":     hintK(func(km keyMap) key.Binding { return km.HideVideo }, "hide video"),
	"hide_channel":   hintK(func(km keyMap) key.Binding { return km.HideChannel }, "block channel"),
	"add_playlist":   hintK(func(km keyMap) key.Binding { return km.AddList }, "add to playlist"),
	"new_playlist":   hintK(func(km keyMap) key.Binding { return km.NewList }, "new playlist"),
	"delete":         hintK(func(km keyMap) key.Binding { return km.Delete }, "delete"),
	"refresh":        hintK(func(km keyMap) key.Binding { return km.Refresh }, "refresh"),
	"open":           hintK(func(km keyMap) key.Binding { return km.DrillDown }, "open"),
	"open_channel":   hintK(func(km keyMap) key.Binding { return km.DrillDown }, "open channel"),
	"open_tag":       hintK(func(km keyMap) key.Binding { return km.DrillDown }, "open tag"),
	"search_again":   hintK(func(km keyMap) key.Binding { return km.DrillDown }, "search"),
	"details":        hintK(func(km keyMap) key.Binding { return km.DrillDown }, "details"),
	"filter":         func(m Model) []hintEntry { return []hintEntry{{m.cfg.Keybindings.Filter, "filter"}} },
	"unsubscribe":    hintK(func(km keyMap) key.Binding { return km.Unsubscribe }, "unsubscribe"),
	"rename":         hintK(func(km keyMap) key.Binding { return km.RenameChannel }, "rename"),
	"edit_tags":      hintK(func(km keyMap) key.Binding { return km.TagChannel }, "tags"),
	"open_links":     hintK(func(km keyMap) key.Binding { return km.OpenLinks }, "links"),
	"open_chapters":  hintK(func(km keyMap) key.Binding { return km.OpenChapters }, "chapters"),
	"toggle_mode": func(m Model) []hintEntry {
		label := "tag view"
		if m.subChTagsMode {
			label = "flat view"
		}
		return []hintEntry{{m.keys.ToggleMode.Help().Key, label}}
	},
}

// tabHintIDs returns the ordered action IDs to display for the current tab and UI state.
func (m Model) tabHintIDs() []string {
	videoBase := []string{"move", "chords", "play", "play_audio", "download", "download_audio", "copy_url", "info", "open_links", "open_chapters"}
	switch m.activeTab {
	case tabRecommended:
		return append(videoBase, "hide_video", "hide_channel", "add_playlist")
	case tabSubscriptions:
		return append(videoBase, "unsubscribe")
	case tabChannels:
		if m.subChTagsMode {
			if m.subChPane == 0 {
				return []string{"move", "open_tag", "toggle_mode"}
			}
			return append(videoBase, "back", "toggle_mode")
		}
		if m.subChPane == 0 {
			return []string{"move", "open", "chords", "rename", "edit_tags", "unsubscribe", "toggle_mode"}
		}
		return []string{"move", "play", "play_audio", "info", "open_links", "open_chapters", "download", "download_audio", "copy_url", "back"}
	case tabPlaylists:
		if m.playlistPane == 1 {
			return []string{"move", "info", "open", "new_playlist", "delete"}
		}
		return []string{"move", "open", "new_playlist", "delete"}
	case tabSearch:
		if m.searchChSel != nil {
			return []string{"move", "play", "play_audio", "info", "open_links", "open_chapters", "download", "download_audio", "copy_url", "filter", "back"}
		}
		return []string{"move", "chords", "play", "play_audio", "open_channel", "info", "download", "download_audio", "copy_url", "open_links"}
	case tabDownloading:
		return []string{"move", "play", "play_audio", "info", "hide_channel", "delete"}
	case tabLocal:
		return []string{"move", "chords", "play", "delete"}
	case tabHistory:
		if m.history.detailVideoID != "" {
			return []string{"back"}
		}
		if m.history.cursor < len(m.history.entries) && m.history.entries[m.history.cursor].EventType == "search" {
			return []string{"move", "search_again", "delete", "refresh"}
		}
		return []string{"move", "chords", "play", "details", "hide_channel", "delete", "refresh"}
	case tabActivity:
		return []string{"move", "open", "refresh"}
	}
	return []string{"move"}
}

func (m Model) fullHintRaw() string {
	var parts []string
	for _, id := range m.tabHintIDs() {
		fn, ok := hintRegistry[id]
		if !ok {
			continue
		}
		for _, e := range fn(m) {
			parts = append(parts, e.key+": "+e.label)
		}
	}
	return strings.Join(parts, "  ")
}

// ── Content router ────────────────────────────────────────────────────────────

func (m Model) contentVideos(raw []youtube.Video, cursor int) ([]youtube.Video, int) {
	if m.localFilter != "" {
		return filterText(raw, m.localFilter), m.localFilterCursor
	}
	return raw, cursor
}

func (m Model) renderContent(height int) string {
	if v := m.activeView(); v != nil {
		return v.render(m.viewCtx(), height)
	}
	switch m.activeTab {
	case tabChannels:
		return m.renderSubChannels(height)
	case tabPlaylists:
		return m.renderPlaylists(height)
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
	if posMs := m.videoPositions[v.ID]; posMs > 0 {
		dur = fmtDurWithPos(posMs, v.Duration)
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
	case !hasLocal && m.streamedVideoIDs[v.ID]:
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

func (m Model) renderSubChannels(height int) string {
	headerText := "Channels"
	if m.subChLoading {
		headerText += "  " + styleDim.Render(m.spinner.View()+" loading…")
	}
	if m.subChTagsMode {
		headerText += "  " + styleDim.Render("[tags]")
	}
	header := styleSectionTitle.Render(headerText)
	headerH := lipgloss.Height(header)

	if m.subChTagsMode {
		return m.renderSubChannelsTags(header, headerH, height)
	}

	// ── Flat mode ─────────────────────────────────────────────────────────────
	if m.subChPane == 0 {
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
		m.renderSubChannelsVideoPane(header, headerH, height, m.sortedChannels(), m.subChCursor)...)
}

func (m Model) renderSubChannelsTags(header string, headerH, height int) string {
	switch m.subChPane {
	case 0: // tag list
		var body string
		if m.subChLoading && len(m.subChannels) == 0 {
			body = m.spinner.View() + " Loading channels…"
		} else {
			body = m.renderTagList(height - headerH)
		}
		return lipgloss.JoinVertical(lipgloss.Left, header, body)

	case 1: // video list for selected tag
		tagHeader := styleSectionTitle.Render("← " + tagDisplayName(m.subChTagSel))
		tagH := lipgloss.Height(tagHeader)
		filterLine := m.filterBar()
		filterH := 0
		if filterLine != "" {
			filterH = 1
		}
		vids, cur := m.contentVideos(m.tagVideos(), m.subChCursor)
		body := m.renderVideoRows(vids, cur, m.subChVS, height-headerH-tagH-filterH)
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
		vids, cur := m.contentVideos(m.subChVideos, m.subChVidCursor)
		body = m.renderVideoRows(vids, cur, m.subChVidVS, height-headerH-subH-filterH)
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
	start, end := scrollWindowAt(m.subChTagVS, len(items), windowH)
	rows := []string{colHeader}

	for i := start; i < end && i < len(items); i++ {
		tag := items[i]
		count := len(m.channelsInTag(tag))
		label := fmt.Sprintf("%s (%d)", tagDisplayName(tag), count)
		selected := i == m.subChTagCursor

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
	start, end := scrollWindowAt(m.subChVS, len(channels), windowH)
	rows := []string{colHeader}

	for i := start; i < end && i < len(channels); i++ {
		ch := channels[i]
		latest := m.subChLatest[ch.ID]
		selected := i == m.subChCursor

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

// ── Playlists ─────────────────────────────────────────────────────────────────

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

// renderSearch draws the Search tab. The cursor/scroll values are owned by the
// searchView and passed in (see view_search.go); everything else it reads is
// router-owned state (input, spinner, result slices).
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
			if m.vidDetailThumbRendered != "" {
				thumbLines = strings.Split(m.vidDetailThumbRendered, "\n")
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
				descLines := m.vidDetailDescLines
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
	var footerLine func(string) string
	kb := m.cfg.Keybindings
	closeKey := m.keys.Escape.Help().Key
	closeHint := closeKey + ": close"
	if m.gPending && !m.linkOverlay {
		footerLine = func(_ string) string {
			return styleWarning.Width(innerW).Render(kb.GotoPrefix + "→" + kb.GotoPrefix + ": top")
		}
		lines = append(lines, footerLine(""))
		lines = append(lines, footerLine(""))
	} else {
		var footerText string
		if needsScroll {
			scrollHint := m.keys.Down.Help().Key + "/" + m.keys.Up.Help().Key + ": scroll"
			space := innerW - lipgloss.Width(scrollHint) - lipgloss.Width(closeHint)
			if space < 1 {
				space = 1
			}
			footerText = scrollHint + strings.Repeat(" ", space) + closeHint
		} else {
			space := innerW - lipgloss.Width(closeHint)
			if space < 1 {
				space = 1
			}
			footerText = strings.Repeat(" ", space) + closeHint
		}
		lines = append(lines, styleHelp.Width(innerW).Render(""))
		lines = append(lines, styleHelp.Width(innerW).Render(footerText))
	}

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
// breaking at word boundaries (spaces). Long tokens (e.g. URLs) are
// hard-broken at the width boundary. Existing newlines are preserved.
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
		cur := ""
		for _, w := range words {
			if lipgloss.Width(w) > width {
				// flush current accumulator, then hard-break the long token
				if cur != "" {
					result = append(result, cur)
					cur = ""
				}
				runes := []rune(w)
				for len(runes) > 0 {
					taken := 0
					col := 0
					for taken < len(runes) {
						cw := runewidth.RuneWidth(runes[taken])
						if col+cw > width {
							break
						}
						col += cw
						taken++
					}
					if taken == 0 {
						taken = 1
					}
					result = append(result, string(runes[:taken]))
					runes = runes[taken:]
				}
				continue
			}
			var candidate string
			if cur == "" {
				candidate = w
			} else {
				candidate = cur + " " + w
			}
			if lipgloss.Width(candidate) <= width {
				cur = candidate
			} else {
				result = append(result, cur)
				cur = w
			}
		}
		if cur != "" {
			result = append(result, cur)
		}
	}
	return result
}

// ── Add-to-playlist overlay ───────────────────────────────────────────────────

func (m Model) renderAddOverlay(behind string) string {
	if m.addOverlayCreateMode {
		label := "New local list:"
		if m.addOverlayCreateYT {
			label = "New remote playlist:"
		}
		lines := []string{
			styleHeader.Render("Create playlist"),
			"",
			styleInputPrompt.Render(label),
			m.addOverlayInput.View(),
			"",
			styleHelp.Render("enter: confirm  esc: back"),
		}
		return m.placeOverlayBox(behind, strings.Join(lines, "\n"), 40)
	}

	lines := []string{
		styleHeader.Render("Add to playlist"),
		"",
	}
	base := m.overlayCreateBase()
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
	localLabel := "  Create local list"
	if m.addOverlaySel == base {
		localLabel = styleSelected.Render("▶ Create local list")
	}
	lines = append(lines, localLabel)
	if m.ytClient != nil {
		remoteLabel := "  Create remote playlist"
		if m.addOverlaySel == base+1 {
			remoteLabel = styleSelected.Render("▶ Create remote playlist")
		}
		lines = append(lines, remoteLabel)
	}
	kb := m.cfg.Keybindings
	const addW = 36
	if m.gPending {
		lines = append(lines, "", styleWarning.Render(kb.GotoPrefix+"→"+kb.GotoPrefix+": top"))
	} else {
		actionHint := "j/k: move  enter: confirm"
		cancelHint := m.keys.Escape.Help().Key + ": cancel"
		space := addW - lipgloss.Width(actionHint) - lipgloss.Width(cancelHint)
		if space < 1 {
			space = 1
		}
		lines = append(lines, "", styleHelp.Render(actionHint+strings.Repeat(" ", space)+cancelHint))
	}

	return m.placeOverlayBox(behind, strings.Join(lines, "\n"), 40)
}

func (m Model) renderLinkOverlay(behind string) string {
	links := m.linkOverlayURLs
	lines := []string{
		styleHeader.Render("Links in description"),
		"",
	}
	innerW := 56
	for i, lnk := range links {
		num := fmt.Sprintf("%2d. ", i+1)
		text := lnk.Label
		if text == "" {
			text = lnk.URL
		}
		text = truncate(text, innerW-len(num)-2)
		row := num + text
		if i == m.linkOverlaySel {
			lines = append(lines, styleSelected.Render("▶ "+row))
		} else {
			lines = append(lines, "  "+row)
		}
	}
	if len(links) > 0 {
		lines = append(lines, "", styleHelp.Render(truncate(links[m.linkOverlaySel].URL, innerW)))
	}
	kb := m.cfg.Keybindings
	if m.gPending {
		lines = append(lines, "", styleWarning.Render(kb.GotoPrefix+"→"+kb.GotoPrefix+": top"))
	} else {
		actionHint := "enter: open  y: copy"
		closeHint := m.keys.Escape.Help().Key + ": close"
		space := innerW - lipgloss.Width(actionHint) - lipgloss.Width(closeHint)
		if space < 1 {
			space = 1
		}
		lines = append(lines, "", styleHelp.Render(actionHint+strings.Repeat(" ", space)+closeHint))
	}
	return m.placeOverlayBox(behind, strings.Join(lines, "\n"), innerW+6)
}

func fmtChapterTime(secs float64) string {
	s := int(secs)
	h, min, sec := s/3600, (s%3600)/60, s%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, min, sec)
	}
	return fmt.Sprintf("%d:%02d", min, sec)
}

func (m Model) renderChapterOverlay(behind string) string {
	chapters := m.chapterOverlayItems
	const innerW = 58
	lines := []string{
		styleHeader.Render("Chapters"),
		"",
	}
	for i, ch := range chapters {
		ts := fmtChapterTime(ch.OriginalStart)
		label := fmt.Sprintf("%-7s  %s", ts, truncate(ch.Title, innerW-11))
		if i == m.chapterOverlaySel {
			lines = append(lines, styleSelected.Render("▶ "+label))
		} else {
			lines = append(lines, "  "+label)
		}
	}
	kb := m.cfg.Keybindings
	if m.gPending {
		lines = append(lines, "", styleWarning.Render(kb.GotoPrefix+"→"+kb.GotoPrefix+": top"))
	} else {
		playKey := m.keys.Play.Help().Key
		audioKey := m.keys.PlayAudio.Help().Key
		copyKey := m.keys.CopyURL.Help().Key
		closeKey := m.keys.Escape.Help().Key
		actionHint := fmt.Sprintf("%s: stream  %s: audio  %s: copy url", playKey, audioKey, copyKey)
		closeHint := closeKey + ": close"
		space := innerW - lipgloss.Width(actionHint) - lipgloss.Width(closeHint)
		if space < 1 {
			space = 1
		}
		lines = append(lines, "", styleHelp.Render(actionHint+strings.Repeat(" ", space)+closeHint))
	}
	return m.placeOverlayBox(behind, strings.Join(lines, "\n"), innerW+6)
}

// placeOverlayBox renders content inside a rounded box and overlays it centered on behind.
func (m Model) placeOverlayBox(behind, content string, width int) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 2).
		Width(width).
		Render(content)

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
		row := behindLines[lineIdx]
		// Pad row so there's content up to x+bw before slicing.
		if visW := lipgloss.Width(row); visW < x {
			row += strings.Repeat(" ", x-visW)
		}
		left := ansi.Truncate(row, x, "")
		right := ansi.TruncateLeft(row, x+bw, "")
		behindLines[lineIdx] = left + ol + right
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
