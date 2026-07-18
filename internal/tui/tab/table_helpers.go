package tab

import (
	"fmt"
	"strconv"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	runewidth "github.com/mattn/go-runewidth"
)

const colIndicator = 3

func tableStyles() table.Styles {
	s := table.DefaultStyles()
	s.Header = styles.ColHeader
	s.Cell = styles.Normal
	s.Selected = styles.Selected
	return s
}

func newTable() table.Model {
	return table.New(table.WithStyles(tableStyles()))
}

// ── column helpers ─────────────────────────────────────────────────────────────

func computeVideoColumns(width int, showChannel bool) []table.Column {
	titleW := width - render.ColNum - colIndicator - render.ColDuration - render.ColViews - render.ColDate
	if showChannel {
		titleW -= render.ColChannel
	}
	if titleW < 20 {
		titleW = 20
	}
	cols := []table.Column{
		{Title: "#", Width: render.ColNum},
		{Title: " ", Width: colIndicator},
		{Title: "Title", Width: titleW},
	}
	if showChannel {
		cols = append(cols, table.Column{Title: "Channel", Width: render.ColChannel})
	}
	return append(cols,
		table.Column{Title: "Duration", Width: render.ColDuration},
		table.Column{Title: "Views", Width: render.ColViews},
		table.Column{Title: "Date", Width: render.ColDate},
	)
}

func computeSearchResultColumns(width int) []table.Column {
	titleW := width - render.ColNum - colIndicator - render.ColChannel - render.ColDuration - render.ColViews - render.ColDate
	if titleW < 20 {
		titleW = 20
	}
	return []table.Column{
		{Title: "#", Width: render.ColNum},
		{Title: " ", Width: colIndicator},
		{Title: "Title / Name", Width: titleW},
		{Title: "Channel", Width: render.ColChannel},
		{Title: "Duration", Width: render.ColDuration},
		{Title: "Views", Width: render.ColViews},
		{Title: "Date", Width: render.ColDate},
	}
}

// ── row helpers ────────────────────────────────────────────────────────────────

func rowNum(i int) string {
	return fmt.Sprintf("%4d", i+1)
}

func ralign(s string, width int) string {
	return fmt.Sprintf("%*s", width, s)
}

func videoIndicator(v domain.Video, positions map[string]int64, watched map[string]bool, localStatus map[string]domain.VideoStatus) string {
	if _, hasPos := positions[v.ID]; hasPos {
		return " ○ "
	}
	if watched[v.ID] {
		return " ○ "
	}
	if st, ok := localStatus[v.ID]; ok {
		switch st {
		case domain.StatusNew:
			return " ● "
		case domain.StatusStarted, domain.StatusWatched:
			return " ○ "
		}
	}
	return "   "
}

func videoTitleStyle(v *domain.Video, positions map[string]int64, watched map[string]bool, localStatus map[string]domain.VideoStatus) lipgloss.Style {
	if st, ok := localStatus[v.ID]; ok {
		switch st {
		case domain.StatusNew:
			return styles.Bold
		case domain.StatusStarted, domain.StatusWatched:
			return styles.Dim
		}
	}
	if _, hasPos := positions[v.ID]; hasPos {
		return styles.Dim
	}
	if watched[v.ID] {
		return styles.Dim
	}
	return styles.Normal
}

func styledTitle(title string, style lipgloss.Style, safeWidth int) string {
	return style.Render(render.Truncate(title, safeWidth))
}

func titleSafeWidth(titleW int, style lipgloss.Style) int {
	overhead := runewidth.StringWidth(style.Render(""))
	w := titleW - overhead
	if w < 1 {
		w = 1
	}
	return w
}

func toVideoRows(videos []domain.Video, positions map[string]int64, watched map[string]bool, localStatus map[string]domain.VideoStatus, showChannel bool, width int) []table.Row {
	titleW := width - render.ColNum - colIndicator - render.ColDuration - render.ColViews - render.ColDate
	if showChannel {
		titleW -= render.ColChannel
	}
	if titleW < 20 {
		titleW = 20
	}
	rows := make([]table.Row, len(videos))
	for i := range videos {
		v := &videos[i]
		dur := render.Duration(v.Duration)
		if posMs := positions[v.ID]; posMs > 0 {
			dur = render.DurationWithPos(posMs, v.Duration)
		}
		style := videoTitleStyle(v, positions, watched, localStatus)
		title := styledTitle(v.Title, style, titleSafeWidth(titleW, style))
		row := table.Row{rowNum(i), videoIndicator(*v, positions, watched, localStatus), title}
		if showChannel {
			row = append(row, v.Channel)
		}
		rows[i] = append(row, ralign(dur, render.ColDuration-2), ralign(v.ViewsStr(), render.ColViews-2)+" ", v.DateStr())
	}
	return rows
}

// ── goto-line helpers ─────────────────────────────────────────────────────────

// checkGotoNum accumulates digit keypresses into buf. Returns true if consumed.
func checkGotoNum(buf *string, msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		if r := msg.Runes[0]; r >= '0' && r <= '9' {
			*buf += string(r)
			return true
		}
	}
	return false
}

// applyGoto sets the table cursor to the 1-based line number in numBuf.
func applyGoto(numBuf string, tbl *table.Model) {
	if n, err := strconv.Atoi(numBuf); err == nil && n > 0 {
		tbl.SetCursor(n - 1)
	}
}

// gotoLineView renders the accumulated number buffer as a goto hint.
func gotoLineView(numBuf string) string {
	if numBuf == "" {
		return ""
	}
	return styles.Bold.Render(":" + numBuf + "▌")
}
