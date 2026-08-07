package videotable

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	etable "github.com/evertras/bubble-table/table"
)

// TableNav manages a single evertras table with shared navigation state.
// Include it in tab structs to replace per-tab boilerplate for
// Up/Down/PageUp/PageDown/gg/G/goto-line.
type TableNav struct {
	tbl           etable.Model
	circular      bool
	overhead      int // UI rows above the table (header lines) — used for page size
	height        int // last total height from Resize
	width         int // last frame width from Resize/SetWidth — used to clamp overflow
	numCols       int // column count — used to compensate the border-width reservation
	numBuf        string
	gotoTopActive bool
}

// NewTableNav builds a standard table from cols and wraps it in a TableNav.
// overhead is the number of rendered lines above the table (e.g. 2 for one header).
// The column count is captured so Resize/SetWidth can compensate for evertras'
// per-column border reservation (see borderPad).
func NewTableNav[T any](cols []ColumnDef[T], circular bool, overhead int) TableNav {
	return TableNav{tbl: NewTable(cols), circular: circular, overhead: overhead, numCols: len(cols)}
}

// borderPad is the width evertras reserves for column borders (len(cols)+1).
// Our tables use a border whose divider/edge glyphs are empty strings, so that
// reserved width renders as nothing and would otherwise leave a blank gap on the
// right. Adding it back to the target width lets the flex column fill it. (#2)
func (n *TableNav) borderPad() int { return n.numCols + 1 }

// handleNavPrefix processes the gg chord and goto-line digit accumulation.
// It returns (consumed, handled): when handled is true the caller should return
// consumed immediately; otherwise the key falls through to movement handling.
func (n *TableNav) handleNavPrefix(msg tea.KeyPressMsg, keys keymap.KeyMap) (consumed, handled bool) {
	// gg chord
	if key.Matches(msg, keys.GotoPrefix) {
		if n.gotoTopActive {
			n.gotoTopActive = false
			n.numBuf = ""
			n.tbl = n.tbl.WithHighlightedRow(0)
		} else {
			n.gotoTopActive = true
		}
		return true, true
	}
	n.gotoTopActive = false

	// digit accumulation for goto-line
	if len(msg.Text) == 1 {
		if r := rune(msg.Text[0]); r >= '0' && r <= '9' {
			n.numBuf += string(r)
			return true, true
		}
	}
	return false, false
}

// HandleNav processes navigation key presses.
// rowCount is the current number of rows in the table.
// Returns true if the message was consumed (caller should return immediately).
func (n *TableNav) HandleNav(msg tea.KeyPressMsg, keys keymap.KeyMap, rowCount int) bool {
	if consumed, handled := n.handleNavPrefix(msg, keys); handled {
		return consumed
	}
	numBuf := n.numBuf
	n.numBuf = ""

	idx := n.tbl.GetHighlightedRowIndex()
	pageH := n.height - n.overhead
	if pageH < 1 {
		pageH = 1
	}

	switch {
	case key.Matches(msg, keys.GotoLine):
		if numBuf != "" {
			if lineNum, err := strconv.Atoi(numBuf); err == nil && lineNum > 0 {
				n.tbl = n.tbl.WithHighlightedRow(lineNum - 1)
			}
		} else if rowCount > 0 {
			n.tbl = n.tbl.WithHighlightedRow(rowCount - 1)
		}
	case key.Matches(msg, keys.GotoBottom):
		if rowCount > 0 {
			n.tbl = n.tbl.WithHighlightedRow(rowCount - 1)
		}
	case key.Matches(msg, keys.Up):
		if idx > 0 {
			n.tbl = n.tbl.WithHighlightedRow(idx - 1)
		} else if n.circular && rowCount > 0 {
			n.tbl = n.tbl.WithHighlightedRow(rowCount - 1)
		}
	case key.Matches(msg, keys.Down):
		if idx < rowCount-1 {
			n.tbl = n.tbl.WithHighlightedRow(idx + 1)
		} else if n.circular && rowCount > 0 {
			n.tbl = n.tbl.WithHighlightedRow(0)
		}
	case key.Matches(msg, keys.PageUp):
		if newIdx := idx - pageH; newIdx > 0 {
			n.tbl = n.tbl.WithHighlightedRow(newIdx)
		} else {
			n.tbl = n.tbl.WithHighlightedRow(0)
		}
	case key.Matches(msg, keys.PageDown):
		if newIdx := idx + pageH; newIdx < rowCount {
			n.tbl = n.tbl.WithHighlightedRow(newIdx)
		} else if rowCount > 0 {
			n.tbl = n.tbl.WithHighlightedRow(rowCount - 1)
		}
	default:
		if numBuf != "" {
			n.numBuf = numBuf // restore — digit wasn't followed by a goto command
		}
		return false
	}
	return true
}

// SetRows updates the table rows.
func (n *TableNav) SetRows(rows []etable.Row) {
	n.tbl = n.tbl.WithRows(rows)
}

// GotoRow sets the highlighted row to idx (0-based).
func (n *TableNav) GotoRow(idx int) {
	n.tbl = n.tbl.WithHighlightedRow(idx)
}

// Resize updates table dimensions. overhead rows are reserved for headers above.
// +1 compensates for evertras reserving a bottom-border line that WithOuterBorder(false) never renders.
func (n *TableNav) Resize(w, h int) {
	n.height = h
	n.width = w
	n.tbl = n.tbl.WithTargetWidth(w + n.borderPad()).WithTargetHeight(h - n.overhead + 1)
}

// Index returns the 0-based highlighted row index.
func (n *TableNav) Index() int {
	return n.tbl.GetHighlightedRowIndex()
}

// View returns the rendered table string. Each line is clamped to the frame
// width so a table whose fixed columns can't fit the given width (e.g. the
// Local tab's 8 columns on an 80-col terminal) truncates its rightmost column
// rather than emitting an over-wide line that lipgloss would wrap and corrupt
// (the ClampLine invariant). It is a no-op for tables that already fit.
func (n *TableNav) View() string {
	tbl := n.tbl
	if styles.ListBorderDimmed {
		// The list is behind a focused info panel: fade its frame so the active
		// panel (normal border) reads as focused. Applied to a local copy so the
		// table's own normal-border color stays intact for the next frame.
		tbl = tbl.WithBorderForeground(styles.ColorBorderDim)
	}
	v := tbl.View()
	if n.width <= 0 {
		return v
	}
	lines := strings.Split(v, "\n")
	for i, l := range lines {
		lines[i] = render.ClampLine(l, n.width)
	}
	return strings.Join(lines, "\n")
}

// NumBufView returns the goto-line prefix hint or "" if no digits buffered.
// It reads as a chord-in-flight indicator (bold digits + what's expected next),
// deliberately *not* a leading ":" that would mimic the command palette.
func (n *TableNav) NumBufView() string {
	if n.numBuf == "" {
		return ""
	}
	return styles.Bold.Render(n.numBuf) + styles.Help.Render(" — digit or G: go to line")
}

// Model returns the underlying etable.Model for cases that need direct access.
func (n *TableNav) Model() etable.Model {
	return n.tbl
}

// SetModel replaces the underlying etable.Model.
func (n *TableNav) SetModel(tbl etable.Model) {
	n.tbl = tbl
}

// SetWidth sets only the table width, leaving height unchanged.
func (n *TableNav) SetWidth(w int) {
	n.width = w
	n.tbl = n.tbl.WithTargetWidth(w + n.borderPad())
}

// SetTargetHeight sets the rendered table height and adjusts the internal
// total height so page-step equals h (for split-pane use where height is
// not simply totalHeight-overhead).
// +1 compensates for evertras reserving a bottom-border line that WithOuterBorder(false) never renders.
func (n *TableNav) SetTargetHeight(h int) {
	n.tbl = n.tbl.WithTargetHeight(h + 1)
	n.height = h + n.overhead
}

// ClearNumBuf discards any partially typed goto-line digits.
func (n *TableNav) ClearNumBuf() {
	n.numBuf = ""
}
