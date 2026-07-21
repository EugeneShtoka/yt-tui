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

// DlDurationCol renders a pre-formatted duration string (not seconds).
// DownloadItem carries duration as a string from yt-dlp; no render.Duration call needed.
func DlDurationCol[T interface{ GetDurationStr() string }]() ColumnDef[T] {
	w := render.ColDurationPos
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyDuration, calign("Duration", w), w),
		Cell: func(item T, _ int) any { return fmt.Sprintf("%*s", w, item.GetDurationStr()) },
	}
}
