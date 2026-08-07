package overlay

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

// ── rendering ─────────────────────────────────────────────────────────────────

const (
	// titleMaxLines caps how many wrapped title lines the metadata block shows.
	titleMaxLines = 3
	// modalChromeRows is the rows a modal spends on chrome (top/bottom borders,
	// padding, title, footer) — subtracted from height to get the content area.
	modalChromeRows = 8
)

func (vd VideoDetail) renderPanel(panelW, panelH, thumbH int) string {
	innerW := panelW - 2
	// Focused panel keeps the normal border (same as any frame at rest); losing
	// focus fades it to the dim border. The out-of-focus side always dims — never
	// a loud accent — so switching focus reads as a quiet handoff between panels.
	borderColor := styles.ColorBorder
	if !vd.focused {
		borderColor = styles.ColorBorderDim
	}
	accent := lipgloss.NewStyle().Foreground(borderColor)
	norm := func(s string) string { return styles.Normal.Width(innerW).Render(s) }

	innerH := panelH - 2
	const footerH = 2
	contentRows := innerH - footerH

	var lines []string
	needsScroll := false

	if vd.loading {
		lines = append(lines, norm(vd.spinnerFrame+" Loading…"))
	} else if vd.video != nil {
		v := vd.video
		lines = append(lines, vd.renderThumbnailLines(innerW, thumbH, norm)...)
		lines = append(lines, vd.renderMetadataLines(innerW, norm)...)
		if v.Description != "" {
			var descLines []string
			descLines, needsScroll = vd.renderDescriptionLines(innerW, contentRows, len(lines), norm)
			lines = append(lines, descLines...)
		}
	}

	for len(lines) < contentRows {
		lines = append(lines, norm(""))
	}
	lines = lines[:contentRows]

	lines = append(lines,
		styles.Help.Width(innerW).Render(""),
		styles.Help.Width(innerW).Render(vd.renderFooterText(innerW, needsScroll)),
	)

	return vd.framePanel(lines, accent, innerW, panelH)
}

// renderThumbnailLines renders the thumbnail region: blank rows on Kitty (the
// image is placed separately), the half-block render when present, or a hatched
// placeholder otherwise.
func (vd VideoDetail) renderThumbnailLines(innerW, thumbH int, norm func(string) string) []string {
	var lines []string
	switch {
	case kittyCapable():
		for i := 0; i < thumbH; i++ {
			lines = append(lines, norm(""))
		}
	case vd.thumbRendered != "":
		lines = append(lines, strings.Split(vd.thumbRendered, "\n")...)
	default:
		placeholder := strings.Repeat("░", innerW)
		for i := 0; i < thumbH; i++ {
			lines = append(lines, norm(placeholder))
		}
	}
	return lines
}

// renderMetadataLines renders the title and metadata block (channel, subs,
// views, duration, date, URL).
func (vd VideoDetail) renderMetadataLines(innerW int, norm func(string) string) []string {
	v := vd.video
	var lines []string
	lines = append(lines, norm(""))
	for i, tl := range render.WordWrap(v.Title, innerW) {
		if i >= titleMaxLines {
			break
		}
		lines = append(lines, norm(tl))
	}

	lbl := styles.Dim
	meta := func(k, val string) string {
		return styles.Normal.Width(innerW).Render(lbl.Render(k) + val)
	}
	lines = append(lines, norm(""), meta("Channel  ", render.Truncate(v.Channel, innerW-9)))
	if v.Subscribers > 0 {
		lines = append(lines, meta("Subs     ", render.Views(v.Subscribers)))
	}
	lines = append(lines,
		meta("Views    ", render.Views(v.ViewCount)),
		meta("Duration ", render.Duration(v.Duration)),
		meta("Date     ", render.Date(v.UploadDate)),
		styles.Help.Width(innerW).Render(""),
		styles.Help.Width(innerW).Render(render.Truncate(v.URL, innerW)),
	)
	return lines
}

// renderDescriptionLines renders the "Description" header and the scrolled
// slice of wrapped description lines that fits in the remaining content rows.
// usedRows is the number of content rows already consumed by the caller. It
// reports needsScroll when the description overflows the available space.
func (vd VideoDetail) renderDescriptionLines(innerW, contentRows, usedRows int, norm func(string) string) (lines []string, needsScroll bool) {
	lines = append(lines, styles.ColHeader.Width(innerW).Render(""), styles.ColHeader.Width(innerW).Render("Description"))
	available := contentRows - usedRows - len(lines)
	if available <= 0 {
		return lines, false
	}
	descLines := vd.descLines
	needsScroll = len(descLines) > available
	maxVS := len(descLines) - 1
	if maxVS < 0 {
		maxVS = 0
	}
	vs := vd.descVS
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
	return lines, needsScroll
}

// renderFooterText builds the footer hint line (scroll + close, or just close).
func (vd VideoDetail) renderFooterText(innerW int, needsScroll bool) string {
	closeHint := vd.keys.Escape.Help().Key + ": close"
	if needsScroll {
		scrollHint := vd.keys.Down.Help().Key + "/" + vd.keys.Up.Help().Key + ": scroll"
		return render.JustifyEnds(scrollHint, closeHint, innerW)
	}
	return render.JustifyEnds("", closeHint, innerW)
}

// framePanel wraps the content lines in the rounded border with the title bar.
func (vd VideoDetail) framePanel(lines []string, accent lipgloss.Style, innerW, panelH int) string {
	border := lipgloss.RoundedBorder()
	title := " Video Details "
	top := accent.Render(border.TopLeft + border.Top + title + strings.Repeat(border.Top, innerW-len(title)-1) + border.TopRight)
	bot := accent.Render(border.BottomLeft + strings.Repeat(border.Bottom, innerW) + border.BottomRight)
	rows := make([]string, 0, panelH)
	rows = append(rows, top)
	for _, l := range lines {
		rows = append(rows, accent.Render(border.Left)+render.ClampLine(l, innerW)+accent.Render(border.Right))
	}
	rows = append(rows, bot)
	return strings.Join(rows, "\n")
}

func (vd VideoDetail) renderLinksModal(behind string, width int) string {
	if !vd.linksLoaded {
		return behind
	}
	links := vd.links
	const innerW = 56
	lines := []string{styles.Bold.Render("Links in description"), ""}
	for i, lnk := range links {
		num := fmt.Sprintf("%2d. ", i+1)
		text := lnk.Label
		if text == "" {
			text = lnk.URL
		}
		text = render.Truncate(render.Sanitize(text), innerW-len(num)-2)
		row := num + text
		if i == vd.linkSel {
			lines = append(lines, styles.Selected.Render("▶ "+row))
		} else {
			lines = append(lines, "  "+row)
		}
	}
	if len(links) > 0 {
		lines = append(lines, "", styles.Help.Render(render.Truncate(render.Sanitize(links[vd.linkSel].URL), innerW)))
	}
	actionHint := vd.keys.DrillDown.Help().Key + ": open  " + vd.keys.CopyURL.Help().Key + ": copy"
	closeHint := vd.keys.Escape.Help().Key + ": close"
	lines = append(lines, "", styles.Help.Render(render.JustifyEnds(actionHint, closeHint, innerW)))
	return placeOverlayBox(behind, strings.Join(lines, "\n"), width, innerW+render.BorderPad)
}

func (vd VideoDetail) renderChaptersModal(behind string, width int) string {
	if !vd.chaptersLoaded {
		return behind
	}
	chapters := vd.chapters
	const innerW = 58
	lines := []string{styles.Bold.Render("Chapters"), ""}
	for i, ch := range chapters {
		ts := fmtChapterTime(ch.OriginalStart)
		label := fmt.Sprintf("%-7s  %s", ts, render.Truncate(render.Sanitize(ch.Title), innerW-11))
		if i == vd.chapterSel {
			lines = append(lines, styles.Selected.Render("▶ "+label))
		} else {
			lines = append(lines, "  "+label)
		}
	}
	playKey := vd.keys.Play.Help().Key
	audioKey := vd.keys.PlayAudio.Help().Key
	copyKey := vd.keys.CopyURL.Help().Key
	closeKey := vd.keys.Escape.Help().Key
	actionHint := fmt.Sprintf("%s: stream  %s: audio  %s: copy url", playKey, audioKey, copyKey)
	closeHint := closeKey + ": close"
	lines = append(lines, "", styles.Help.Render(render.JustifyEnds(actionHint, closeHint, innerW)))
	return placeOverlayBox(behind, strings.Join(lines, "\n"), width, innerW+render.BorderPad)
}

// chapterHeaderRE matches a chapter header line in the display transcript,
// "## <timestamp> <title>" (timestamp H:MM:SS or M:SS). Group 1 is the title
// without the marker or timestamp — what the reader sees in the overlay.
var chapterHeaderRE = regexp.MustCompile(`^## \d+:\d\d(?::\d\d)?\s+(.*)$`)

// transcriptWrapped word-wraps the loaded transcript to the current popup width.
// Shared by the renderer and the chapter-navigation key handler so both agree on
// line indices.
func (vd VideoDetail) transcriptWrapped() []string {
	return render.WordWrap(vd.transcriptText, transcriptTextWidth(vd.transcriptWidth, vd.transcriptTermWidth()))
}

// transcriptHeaderRows returns the indices of wrapped lines that begin a timed
// chapter section ("## <timestamp> <title>"). Used by [/] navigation and to
// decide whether to show the chapter hint — the flat-fallback "## Transcript"
// header (no timestamp) is deliberately excluded.
func transcriptHeaderRows(lines []string) []int {
	var rows []int
	for i, l := range lines {
		if chapterHeaderRE.MatchString(l) {
			rows = append(rows, i)
		}
	}
	return rows
}

// renderTranscriptLine renders one wrapped transcript line: a "## ..." header
// shows as a bold title (marker and, for chapters, the timestamp stripped);
// everything else is clamped plain text.
func renderTranscriptLine(l string, innerW int) string {
	if strings.HasPrefix(l, "## ") {
		title := strings.TrimSpace(strings.TrimPrefix(l, "## "))
		if m := chapterHeaderRE.FindStringSubmatch(l); m != nil {
			title = m[1]
		}
		return styles.Bold.Render(render.ClampLine(title, innerW))
	}
	return render.ClampLine(l, innerW)
}

// renderTranscriptModal renders the scrollable transcript popup with a copy-all
// hint. height bounds how many wrapped lines are shown at once.
func (vd VideoDetail) renderTranscriptModal(behind string, width, height int) string {
	boxW := transcriptBoxWidth(vd.transcriptWidth, width)
	innerW := transcriptTextWidth(vd.transcriptWidth, width)
	lines := render.WordWrap(vd.transcriptText, innerW)

	maxRows := height - modalChromeRows
	if maxRows < 3 {
		maxRows = 3
	}
	needsScroll := len(lines) > maxRows
	maxVS := len(lines) - maxRows
	if maxVS < 0 {
		maxVS = 0
	}
	vs := vd.transcriptVS
	if vs > maxVS {
		vs = maxVS
	}

	out := []string{styles.Bold.Render("Transcript"), ""}
	if len(lines) == 0 {
		out = append(out, styles.Dim.Render("(no transcript text)"))
	} else {
		visible := lines[vs:]
		if len(visible) > maxRows {
			visible = visible[:maxRows]
		}
		for _, l := range visible {
			out = append(out, renderTranscriptLine(l, innerW))
		}
	}

	copyHint := vd.keys.CopyTranscript.Help().Key + ": copy all"
	closeHint := vd.keys.Escape.Help().Key + ": close"
	var left string
	if needsScroll {
		left = vd.keys.Down.Help().Key + "/" + vd.keys.Up.Help().Key + ": scroll   " + copyHint
	} else {
		left = copyHint
	}
	if len(transcriptHeaderRows(lines)) > 0 {
		left += "   " + vd.keys.PrevChapter.Help().Key + "/" + vd.keys.NextChapter.Help().Key + ": chapters"
	}
	out = append(out, "", styles.Help.Render(render.JustifyEnds(left, closeHint, innerW)))
	return placeOverlayBox(behind, strings.Join(out, "\n"), width, boxW)
}

// renderTranscriptLoading centers a small spinner popup while a transcript fetch
// is in flight. Shared by the standalone transcript overlay and the in-panel
// fetch (which stays on vdPanel until the fetch resolves into the modal).
func (vd VideoDetail) renderTranscriptLoading(behind string, width int) string {
	return placeOverlayBox(behind, styles.Normal.Render(vd.spinnerFrame+" Loading transcript…"), width, 32)
}

// Render composes the side panel to the right of behind.
// Returns (composedView, kittySeq); kittySeq is non-empty only on Kitty terminals.
func (vd VideoDetail) Render(behind string, width, height int) string {
	// Standalone links/chapters modal (opened directly from the table): no side
	// panel — just center the modal over the full-width tab content, fully
	// independent of the info panel.
	if vd.initialView != InitialViewPanel {
		switch vd.subState {
		case vdLinks:
			return vd.renderLinksModal(behind, width)
		case vdChapters:
			return vd.renderChaptersModal(behind, width)
		case vdTranscript:
			return vd.renderTranscriptModal(behind, width, height)
		}
		if vd.initialView == InitialViewTranscript {
			// transcript fetch still in flight — show a small loading box
			return vd.renderTranscriptLoading(behind, width)
		}
		return behind // still loading — leave the tab content untouched
	}

	_, thumbH := vd.thumbDimensions()
	panel := vd.renderPanel(panelW, height, thumbH)

	// Clamp each line of 'behind' to fill the width left of the panel. The tab
	// content is rendered panelGap columns narrower (via WidthReduction), so the
	// padding here leaves a blank gap between the content and the panel.
	leftW := width - panelW
	if leftW < 0 {
		leftW = 0
	}

	behindLines := strings.Split(behind, "\n")
	for i, line := range behindLines {
		behindLines[i] = render.ClampLine(line, leftW)
	}
	croppedBehind := strings.Join(behindLines, "\n")

	composed := lipgloss.JoinHorizontal(lipgloss.Top, croppedBehind, panel)
	// Modals stack on top of the composed view.
	switch vd.subState {
	case vdLinks:
		composed = vd.renderLinksModal(composed, width)
	case vdChapters:
		composed = vd.renderChaptersModal(composed, width)
	case vdTranscript:
		composed = vd.renderTranscriptModal(composed, width, height)
	}
	// A first-open transcript fetch is still in flight (subState is still vdPanel
	// until it resolves) — show the loading popup over the panel.
	if vd.transcriptLoading {
		composed = vd.renderTranscriptLoading(composed, width)
	}

	// Kitty image placement is handled via tea.Raw commands in Update/kittyCmd,
	// not via embedded escape sequences in the rendered string (bubbletea v2's
	// cell renderer drops APC sequences from Content).
	return composed
}
