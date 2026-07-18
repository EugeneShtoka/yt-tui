package tab

import (
	"fmt"
	"strconv"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

const colIndicator = 2

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
	return styles.RowNum.Render(fmt.Sprintf("%4d", i+1))
}

func videoIndicator(v domain.Video, positions map[string]int64, watched map[string]bool, localStatus map[string]domain.VideoStatus) string {
	if _, hasPos := positions[v.ID]; hasPos {
		return styles.Dim.Render("○ ")
	}
	if watched[v.ID] {
		return styles.Dim.Render("○ ")
	}
	if st, ok := localStatus[v.ID]; ok {
		switch st {
		case domain.StatusNew:
			return styles.Success.Render("● ")
		case domain.StatusStarted, domain.StatusWatched:
			return styles.Dim.Render("○ ")
		}
	}
	return "  "
}

func toVideoRows(videos []domain.Video, positions map[string]int64, watched map[string]bool, localStatus map[string]domain.VideoStatus, showChannel bool) []table.Row {
	rows := make([]table.Row, len(videos))
	for i := range videos {
		v := &videos[i]
		dur := v.DurationStr()
		if posMs := positions[v.ID]; posMs > 0 {
			dur = render.DurationWithPos(posMs, v.Duration)
		}
		row := table.Row{rowNum(i), videoIndicator(*v, positions, watched, localStatus), v.Title}
		if showChannel {
			row = append(row, v.Channel)
		}
		rows[i] = append(row, dur, v.ViewsStr(), v.DateStr())
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
