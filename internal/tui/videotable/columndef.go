package videotable

import (
	"charm.land/lipgloss/v2"
	etable "github.com/evertras/bubble-table/table"
)

// ColumnDef[T] pairs an evertras column spec with a typed cell renderer.
// T is the row-input type; index is the 0-based row position in the table.
// Cell returns a plain string or etable.StyledCell.
type ColumnDef[T any] struct {
	Col  etable.Column
	Cell func(item T, index int) any
}

// Columns extracts the etable.Column specs from a ColumnDef slice.
func Columns[T any](cols []ColumnDef[T]) []etable.Column {
	out := make([]etable.Column, len(cols))
	for i := range cols {
		out[i] = cols[i].Col
	}
	return out
}

// NewTable constructs a standard evertras table from typed column defs.
// g and G are unbound — tabs manage gg/G navigation via TableNav.
func NewTable[T any](cols []ColumnDef[T]) etable.Model {
	return newEtable(Columns(cols))
}

// BuildRows converts items to evertras rows using typed column definitions.
func BuildRows[T any](items []T, cols []ColumnDef[T]) []etable.Row {
	return buildRowsImpl(items, cols, nil)
}

// BuildRowsStyled is BuildRows with optional per-row lipgloss styling.
// styler returns nil to skip styling for a given item.
func BuildRowsStyled[T any](items []T, cols []ColumnDef[T], styler func(T) *lipgloss.Style) []etable.Row {
	return buildRowsImpl(items, cols, styler)
}

func buildRowsImpl[T any](items []T, cols []ColumnDef[T], styler func(T) *lipgloss.Style) []etable.Row {
	rows := make([]etable.Row, len(items))
	for i := range items {
		data := make(etable.RowData, len(cols))
		for j := range cols {
			data[cols[j].Col.Key()] = cols[j].Cell(items[i], i)
		}
		row := etable.NewRow(data)
		if styler != nil {
			if st := styler(items[i]); st != nil {
				row = row.WithStyle(*st)
			}
		}
		rows[i] = row
	}
	return rows
}
