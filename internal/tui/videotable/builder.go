package videotable

import (
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"charm.land/lipgloss/v2"
	etable "github.com/evertras/bubble-table/table"
)

// BuildVideoRows converts a pre-enriched []VideoData into evertras rows.
// Faded rows (watched/in-progress) receive a row-level Dim style.
func BuildVideoRows(vds []VideoData, cols []ColumnDef[VideoData]) []etable.Row {
	return BuildRowsStyled(vds, cols, func(vd VideoData) *lipgloss.Style {
		if isFadedVD(vd) {
			return &styles.Dim
		}
		return nil
	})
}
