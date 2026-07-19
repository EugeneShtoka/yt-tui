package tab

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
		{Title: ralign("#", render.ColNum), Width: render.ColNum},
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
	return swapReset(style.Render(render.Truncate(title, safeWidth)))
}

// swapReset replaces lipgloss's trailing full SGR reset with a partial reset
// that clears bold/faint and foreground but preserves background. This prevents
// the selected-row highlight from being cleared mid-row by inline cell styles.
func swapReset(s string) string {
	const partial = "\033[22;39m"
	if strings.HasSuffix(s, "\033[0m") {
		return s[:len(s)-4] + partial
	}
	if strings.HasSuffix(s, "\033[m") {
		return s[:len(s)-3] + partial
	}
	return s
}

// dimSwapReset is like swapReset but uses \033[22m (resets bold/faint only,
// leaves foreground and background intact). Sufficient for Dim-only cells since
// Dim does not set a foreground color. Saves 3 runewidth chars vs swapReset,
// which is essential for narrow columns (views, date, duration).
func dimSwapReset(s string) string {
	const partial = "\033[22m"
	if strings.HasSuffix(s, "\033[0m") {
		return s[:len(s)-4] + partial
	}
	if strings.HasSuffix(s, "\033[m") {
		return s[:len(s)-3] + partial
	}
	return s
}

func titleSafeWidth(titleW int, style lipgloss.Style) int {
	// bubbles/table calls runewidth.Truncate on each cell, which counts ANSI bytes
	// as visible chars. Measure the overhead as runewidth − ansi.StringWidth so
	// that pre-truncating content to (titleW − overhead) keeps the final cell at
	// exactly titleW in runewidth without triggering bubbles/table's truncator.
	styled := swapReset(style.Render("X"))
	overhead := runewidth.StringWidth(styled) - ansi.StringWidth(styled)
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

	// ColChannel is not inflated like ColViews/ColDate/ColDuration, so we still
	// need to pre-truncate the channel name to leave room for the ANSI overhead.
	chSafeWidth := render.ColChannel - render.DimCellOverhead
	if chSafeWidth < 1 {
		chSafeWidth = 1
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

		// Mirror videoTitleStyle priority: localStatus wins over positions/watched.
		faded := false
		if st, ok := localStatus[v.ID]; ok {
			faded = st == domain.StatusStarted || st == domain.StatusWatched
		} else {
			_, hasPos := positions[v.ID]
			faded = hasPos || watched[v.ID]
		}

		// ColDuration/ColViews/ColDate are inflated by DimCellOverhead so that
		// dim-styled cells (overhead + content) fit exactly within the column.
		// All rows right-align into (col - DimCellOverhead); the table's lipgloss
		// Width style pads non-dim rows with the remaining trailing spaces.
		durCell := ralign(dur, render.ColDuration-render.DimCellOverhead)
		viewsCell := ralign(v.ViewsStr(), render.ColViews-render.DimCellOverhead-1) + " "
		dateCell := v.DateStr()

		row := table.Row{rowNum(i), videoIndicator(*v, positions, watched, localStatus), title}
		if showChannel {
			ch := v.Channel
			if faded {
				ch = dimSwapReset(styles.Dim.Render(render.Truncate(v.Channel, chSafeWidth)))
			}
			row = append(row, ch)
		}
		if faded {
			durCell = dimSwapReset(styles.Dim.Render(durCell))
			viewsCell = dimSwapReset(styles.Dim.Render(viewsCell))
			dateCell = dimSwapReset(styles.Dim.Render(dateCell))
		}
		rows[i] = append(row, durCell, viewsCell, dateCell)
	}
	return rows
}

// ── goto-top chord helpers ────────────────────────────────────────────────────

// handleGotoPrefix manages the 'gg' chord: first press arms the flag; second
// press triggers GotoTop and returns true. Any other key clears the flag.
// Returns (consumed, doGotoTop).
func handleGotoPrefix(active *bool, keys keymap.KeyMap, msg tea.KeyPressMsg) (consumed, doGotoTop bool) {
	if key.Matches(msg, keys.GotoPrefix) {
		if *active {
			*active = false
			return true, true
		}
		*active = true
		return true, false
	}
	*active = false
	return false, false
}

// ── goto-line helpers ─────────────────────────────────────────────────────────

// checkGotoNum accumulates digit keypresses into buf. Returns true if consumed.
func checkGotoNum(buf *string, msg tea.KeyPressMsg) bool {
	if len(msg.Text) == 1 {
		if r := rune(msg.Text[0]); r >= '0' && r <= '9' {
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
