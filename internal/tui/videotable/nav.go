package videotable

import (
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	etable "github.com/evertras/bubble-table/table"
)

// TableNav manages a single evertras table with shared navigation state.
// Include it in tab structs to replace per-tab boilerplate for
// Up/Down/PageUp/PageDown/gg/G/goto-line.
type TableNav struct {
	tbl             etable.Model
	circular        bool
	overhead        int // UI rows above the table (header lines) — used for page size
	height          int // last total height from Resize
	numBuf          string
	gotoTopActive   bool
	SortChordActive bool // exported: tab sets on SortChord key, checks before HandleNav
}

// NewTableNav wraps tbl in a TableNav.
// overhead is the number of rendered lines above the table (e.g. 2 for one header).
func NewTableNav(tbl etable.Model, circular bool, overhead int) TableNav {
	return TableNav{tbl: tbl, circular: circular, overhead: overhead}
}

// HandleNav processes navigation key presses.
// rowCount is the current number of rows in the table.
// Returns true if the message was consumed (caller should return immediately).
func (n *TableNav) HandleNav(msg tea.KeyPressMsg, keys keymap.KeyMap, rowCount int) bool {
	// gg chord
	if key.Matches(msg, keys.GotoPrefix) {
		if n.gotoTopActive {
			n.gotoTopActive = false
			n.numBuf = ""
			n.tbl = n.tbl.WithHighlightedRow(0)
		} else {
			n.gotoTopActive = true
		}
		return true
	}
	n.gotoTopActive = false

	// digit accumulation for goto-line
	if len(msg.Text) == 1 {
		if r := rune(msg.Text[0]); r >= '0' && r <= '9' {
			n.numBuf += string(r)
			return true
		}
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
func (n *TableNav) Resize(w, h int) {
	n.height = h
	n.tbl = n.tbl.WithTargetWidth(w).WithTargetHeight(h - n.overhead)
}

// Index returns the 0-based highlighted row index.
func (n *TableNav) Index() int {
	return n.tbl.GetHighlightedRowIndex()
}

// View returns the rendered table string.
func (n *TableNav) View() string {
	return n.tbl.View()
}

// NumBufView returns the goto-line overlay (":42▌") or "" if no digits buffered.
func (n *TableNav) NumBufView() string {
	if n.numBuf == "" {
		return ""
	}
	return styles.Bold.Render(":" + n.numBuf + "▌")
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
	n.tbl = n.tbl.WithTargetWidth(w)
}

// SetTargetHeight sets the rendered table height and adjusts the internal
// total height so page-step equals h (for split-pane use where height is
// not simply totalHeight-overhead).
func (n *TableNav) SetTargetHeight(h int) {
	n.tbl = n.tbl.WithTargetHeight(h)
	n.height = h + n.overhead
}

// ClearNumBuf discards any partially typed goto-line digits.
func (n *TableNav) ClearNumBuf() {
	n.numBuf = ""
}
