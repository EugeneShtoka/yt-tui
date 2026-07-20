package videotable

import (
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"charm.land/lipgloss/v2"
	etable "github.com/evertras/bubble-table/table"
)

// VideoColumns extracts the evertras Column specs from a VideoColumnDef slice.
func VideoColumns(cols []VideoColumnDef) []etable.Column {
	return Columns(cols)
}

// BuildVideoRows converts a slice of domain.Video into evertras rows.
// Faded rows (watched/in-progress) receive a row-level styles.Dim style.
func BuildVideoRows(videos []domain.Video, cols []VideoColumnDef, ctx RenderContext) []etable.Row {
	cells := make([]VideoCell, len(videos))
	for i := range videos {
		cells[i] = VideoCell{Video: videos[i], Ctx: ctx}
	}
	return BuildRowsStyled(cells, cols, func(vc VideoCell) *lipgloss.Style {
		if isFaded(vc.Video, ctx) {
			return &styles.Dim
		}
		return nil
	})
}
