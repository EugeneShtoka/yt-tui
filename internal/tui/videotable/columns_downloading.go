package videotable

import (
	"fmt"

	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	etable "github.com/evertras/bubble-table/table"
)

// DlStatusCol renders the download progress/status cell.
// renderer is provided by the tab since status styling uses tab-local style vars.
func DlStatusCol[T any](renderer func(T) any) ColumnDef[T] {
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyDlStatus, "Status", ColDlStatus),
		Cell: func(item T, _ int) any { return renderer(item) },
	}
}

// DlDurationCol renders the queue entry's duration, formatting seconds via
// render.Duration so it honors the configured DurationFormat like every other
// duration column in the UI.
func DlDurationCol[T interface{ GetDurationSecs() int }]() ColumnDef[T] {
	w := render.ColDuration
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyDuration, calign("Duration", w), w),
		Cell: func(item T, _ int) any { return fmt.Sprintf("%*s", w, render.Duration(item.GetDurationSecs())) },
	}
}
