package videotable

import (
	"fmt"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	etable "github.com/evertras/bubble-table/table"
	"charm.land/lipgloss/v2"
)

// Column key constants — used as RowData map keys.
const (
	KeyNum       = "num"
	KeyIndicator = "ind"
	KeyTitle     = "title"
	KeyChannel   = "ch"
	KeyDuration  = "dur"
	KeyViews     = "views"
	KeyDate      = "date"
)

// VideoCell is the input passed to every cell renderer.
type VideoCell struct {
	Video domain.Video
	Index int // 0-based row index
	Ctx   RenderContext
}

// VideoColumnDef pairs an evertras column spec with a cell renderer.
type VideoColumnDef struct {
	Col  etable.Column
	Cell func(VideoCell) any // returns string or etable.StyledCell
}

// Pre-defined column definitions. Tabs compose a slice of these and pass it to
// NewVideoTable / BuildVideoRows.
//
// Column widths are the clean content widths (no DimCellOverhead inflation).
// evertras uses ansi.StringWidth for truncation, so ANSI styling is invisible
// to width math and no overhead adjustments are needed.
var (
	Num = VideoColumnDef{
		Col:  etable.NewColumn(KeyNum, fmt.Sprintf("%4s", "#"), 4),
		Cell: func(vc VideoCell) any { return fmt.Sprintf("%4d", vc.Index+1) },
	}

	Indicator = VideoColumnDef{
		Col:  etable.NewColumn(KeyIndicator, " ", 3),
		Cell: func(vc VideoCell) any { return indicatorStr(vc.Video, vc.Ctx) },
	}

	// Title is a flex column — grows to fill the remaining width set by WithTargetWidth.
	Title = VideoColumnDef{
		Col: etable.NewFlexColumn(KeyTitle, "Title", 1),
		Cell: func(vc VideoCell) any {
			st := titleStyle(vc.Video, vc.Ctx)
			return etable.NewStyledCell(vc.Video.Title, st)
		},
	}

	Channel = VideoColumnDef{
		Col: etable.NewColumn(KeyChannel, "Channel", 30),
		Cell: func(vc VideoCell) any {
			ch := vc.Video.Channel
			if vc.Ctx.Aliases != nil {
				if a, ok := vc.Ctx.Aliases[vc.Video.ChannelID]; ok && a != "" {
					ch = a
				}
			}
			return ch
		},
	}

	Views = VideoColumnDef{
		Col:  etable.NewColumn(KeyViews, "Views", 8),
		Cell: func(vc VideoCell) any { return fmt.Sprintf("%8s", vc.Video.ViewsStr()) },
	}

	Date = VideoColumnDef{
		Col:  etable.NewColumn(KeyDate, "Date", 11),
		Cell: func(vc VideoCell) any { return vc.Video.DateStr() },
	}
)

// DurationCol returns a VideoColumnDef whose width is computed from the active
// duration format. Call it from a tab constructor (after render.SetDurFmt) rather
// than at package init to capture the correct column width.
func DurationCol() VideoColumnDef {
	w := render.ColDuration - render.DimCellOverhead
	return VideoColumnDef{
		Col: etable.NewColumn(KeyDuration, "Duration", w),
		Cell: func(vc VideoCell) any {
			dur := render.Duration(vc.Video.Duration)
			if posMs := vc.Ctx.Positions[vc.Video.ID]; posMs > 0 {
				dur = render.DurationWithPos(posMs, vc.Video.Duration)
			}
			return fmt.Sprintf("%*s", w, dur)
		},
	}
}

// isFaded returns true when a video should be rendered with the Dim style.
// localStatus takes priority; positions and watched are fallbacks.
func isFaded(v domain.Video, ctx RenderContext) bool {
	if st, ok := ctx.LocalStatus[v.ID]; ok {
		return st == domain.StatusStarted || st == domain.StatusWatched
	}
	_, hasPos := ctx.Positions[v.ID]
	return hasPos || ctx.Watched[v.ID]
}

// titleStyle returns the lipgloss style for a video's title cell.
func titleStyle(v domain.Video, ctx RenderContext) lipgloss.Style {
	if st, ok := ctx.LocalStatus[v.ID]; ok {
		switch st {
		case domain.StatusNew:
			return styles.Bold
		case domain.StatusStarted, domain.StatusWatched:
			return styles.Dim
		}
	}
	if _, hasPos := ctx.Positions[v.ID]; hasPos {
		return styles.Dim
	}
	if ctx.Watched[v.ID] {
		return styles.Dim
	}
	return styles.Normal
}

// indicatorStr returns the 3-char indicator symbol for a video row.
func indicatorStr(v domain.Video, ctx RenderContext) string {
	if _, hasPos := ctx.Positions[v.ID]; hasPos {
		return " ○ "
	}
	if ctx.Watched[v.ID] {
		return " ○ "
	}
	if st, ok := ctx.LocalStatus[v.ID]; ok {
		switch st {
		case domain.StatusNew:
			return " ● "
		case domain.StatusStarted, domain.StatusWatched:
			return " ○ "
		}
	}
	return "   "
}
