package videotable

import (
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	etable "github.com/evertras/bubble-table/table"
)

// VideoColumns extracts the evertras Column specs from a slice of VideoColumnDefs.
func VideoColumns(cols []VideoColumnDef) []etable.Column {
	out := make([]etable.Column, len(cols))
	for i, c := range cols {
		out[i] = c.Col
	}
	return out
}

// BuildVideoRows converts a slice of domain.Video into evertras rows using the
// given column definitions and render context.
//
// Faded rows (watched or in-progress) receive a row-level styles.Dim style, which
// dims all cells uniformly — no per-cell ANSI overhead required.
func BuildVideoRows(videos []domain.Video, cols []VideoColumnDef, ctx RenderContext) []etable.Row {
	rows := make([]etable.Row, len(videos))
	for i, v := range videos {
		data := make(etable.RowData, len(cols))
		input := VideoCell{Video: v, Index: i, Ctx: ctx}
		for _, col := range cols {
			data[col.Col.Key()] = col.Cell(input)
		}
		row := etable.NewRow(data)
		if isFaded(v, ctx) {
			row = row.WithStyle(styles.Dim)
		}
		rows[i] = row
	}
	return rows
}
