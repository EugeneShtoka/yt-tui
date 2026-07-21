package videotable

import (
	"fmt"

	"charm.land/lipgloss/v2"
	runewidth "github.com/mattn/go-runewidth"

	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	etable "github.com/evertras/bubble-table/table"
)

// ── Column key constants ────────────────────────────────────────────────────
// Shared keys (used across multiple tables)
const (
	KeyNum      = "num"
	KeyInd      = "ind"
	KeyTitle    = "title"
	KeyChannel  = "ch"
	KeyDuration = "dur"
	KeyCount    = "count"
	KeyDate     = "date"
	KeyLabel    = "label"
)

// Tab-specific keys
const (
	KeyHistType   = "histtype"
	KeyHistDetail = "histdetail"
	KeyHistTs     = "histts"

	KeyChName  = "chname"
	KeyChTags  = "chtags"
	KeyChSubs  = "chsubs"
	KeyChViews = "chviews"
	KeyChTitle = "chtitle"

	KeyDlStatus = "dlstatus"

	KeyActType   = "acttype"
	KeyActDetail = "actdetail"

	KeyTagLabel = "taglabel"
	KeyPlName   = "plname"

	KeySrchChName = "srchchname"
)

// ── Column width constants ──────────────────────────────────────────────────
const (
	ColIndicator  = 3
	ColChName     = 22
	ColChTags     = 14
	ColChSubs     = 12
	ColHistStatus = 14
	ColDlStatus   = 52
	ColActType    = 16
)

// ── Generic column factories ────────────────────────────────────────────────

func NumCol[T any]() ColumnDef[T] {
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyNum, fmt.Sprintf("%4s", "#"), render.ColNum),
		Cell: func(_ T, i int) any { return fmt.Sprintf("%4d", i+1) },
	}
}

func BlankIndicatorCol[T any]() ColumnDef[T] {
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyInd, " ", ColIndicator),
		Cell: func(_ T, _ int) any { return "   " },
	}
}

func IndicatorCol[T HasIndicator]() ColumnDef[T] {
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyInd, " ", ColIndicator),
		Cell: func(item T, _ int) any { return item.GetIndicator() },
	}
}

func TitleFlexCol[T HasTitle]() ColumnDef[T] {
	return ColumnDef[T]{
		Col:  etable.NewFlexColumn(KeyTitle, "Title", 1),
		Cell: func(item T, _ int) any { return item.GetTitle() },
	}
}

// AudioTitleFlexCol renders title + " ♪" for audio rows. The ♪ logic lives here,
// not in each type's GetBaseTitle.
func AudioTitleFlexCol[T HasAudioTitle]() ColumnDef[T] {
	return ColumnDef[T]{
		Col: etable.NewFlexColumn(KeyTitle, "Title", 1),
		Cell: func(item T, _ int) any {
			t := item.GetBaseTitle()
			if item.IsAudio() {
				t += " ♪"
			}
			return t
		},
	}
}

// ChannelCol renders the channel name. Alias resolution is handled at enrich time
// (VideoData.ChannelAlias, HistoryRow.displayChannel), so GetChannelName() already
// returns the display-ready value.
func ChannelCol[T HasChannelInfo]() ColumnDef[T] {
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyChannel, "Channel", render.ColChannel),
		Cell: func(item T, _ int) any { return item.GetChannelName() },
	}
}

// DurationCol renders "pos/total" when position > 0, otherwise "total".
// Both values use the active duration format. Column width is ColDurationPos.
func DurationCol[T HasDuration]() ColumnDef[T] {
	w := render.ColDurationPos
	return ColumnDef[T]{
		Col: etable.NewColumn(KeyDuration, calign("Duration", w), w),
		Cell: func(item T, _ int) any {
			total := render.Duration(item.GetDurationSecs())
			if pos := item.GetLastPositionSecs(); pos > 0 {
				return fmt.Sprintf("%*s", w, render.Duration(pos)+"/"+total)
			}
			return fmt.Sprintf("%*s", w, total)
		},
	}
}

func ViewsCol[T HasCount]() ColumnDef[T] {
	w := render.ColViews
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyCount, calign("Views", w+1), w+1),
		Cell: func(item T, _ int) any { return fmt.Sprintf("%*s ", w, render.Views(item.GetCount())) },
	}
}

func SubsCol[T HasCount]() ColumnDef[T] {
	w := render.ColViews
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyChSubs, calign("Subs", w+1), w+1),
		Cell: func(item T, _ int) any { return fmt.Sprintf("%*s ", w, render.Views(item.GetCount())) },
	}
}

func DateCol[T HasDate]() ColumnDef[T] {
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyDate, calign("Date", render.ColDate), render.ColDate),
		Cell: func(item T, _ int) any { return render.Date(item.GetRawDate()) },
	}
}

// StyledLabelCol renders a fixed-width label with the given lipgloss style.
// Used for event-type columns (history, activity) that always show a Warning-styled tag.
func StyledLabelCol[T HasLabel](header string, width int, style lipgloss.Style) ColumnDef[T] {
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyLabel, header, width),
		Cell: func(item T, _ int) any { return etable.NewStyledCell(item.GetLabel(), style) },
	}
}

// VideoTitleStyler returns a per-row Dim style for faded VideoData rows.
func VideoTitleStyler(vd VideoData) *lipgloss.Style {
	if isFadedVD(vd) {
		return &styles.Dim
	}
	return nil
}

// ralign right-aligns a string within width w.
func ralign(s string, w int) string {
	return fmt.Sprintf("%*s", w, s)
}

// calign center-aligns a string within width w (left-biased on odd remainder).
func calign(s string, w int) string {
	n := runewidth.StringWidth(s)
	if n >= w {
		return s
	}
	left := (w - n) / 2
	return fmt.Sprintf("%*s%-*s", left+n, s, w-left-n, "")
}
