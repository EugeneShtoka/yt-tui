package videotable

import (
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	etable "github.com/evertras/bubble-table/table"
)

// VideoColumns extracts the evertras Column specs from a slice of VideoColumnDefs.
func VideoColumns(cols []VideoColumnDef) []etable.Column {
	out := make([]etable.Column, len(cols))
	
	for i := range cols {
		out[i] = cols[i].Col
	}

	return out
}

// BuildVideoRows converts a slice of domain.Video into evertras rows using the
// given column definitions and render context.
// Faded rows (watched or in-progress) receive a row-level styles.Dim style, which
// dims all cells uniformly — no per-cell ANSI overhead required.
func BuildVideoRows(videos []domain.Video, cols []VideoColumnDef, ctx RenderContext) []etable.Row {
	rows := make([]etable.Row, len(videos))
	for i := range videos {
		data := make(etable.RowData, len(cols))
		input := VideoCell{Video: videos[i], Index: i, Ctx: ctx}
		for j := range cols {
			data[cols[j].Col.Key()] = cols[j].Cell(input)
		}
		row := etable.NewRow(data)
		if isFaded(videos[i], ctx) {
			row = row.WithStyle(styles.Dim)
		}
		rows[i] = row
	}
	return rows
}
