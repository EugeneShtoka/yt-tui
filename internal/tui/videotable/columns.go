package videotable

import (
	"fmt"

	"charm.land/lipgloss/v2"
	runewidth "github.com/mattn/go-runewidth"

	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
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
	KeyWatched  = "watched"
	KeyCount    = "count"
	KeyDate     = "date"
	KeyLabel    = "label"
	KeySize     = "size"
)

// Tab-specific keys
const (
	KeyHistType   = "histtype"
	KeyHistDetail = "histdetail"
	KeyHistTs     = "histts"

	KeyChName  = "chname"
	KeyChState = "chstate"
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
	ColChState    = 6
	ColChTags     = 14
	ColChSubs     = 12
	ColHistStatus = 14
	ColDlStatus   = 52
	ColActType    = 16
)

// ── Generic column factories ────────────────────────────────────────────────

// NumCol renders a 1-based row number, right-aligned to the "#" header width.
func NumCol[T any]() ColumnDef[T] {
	w := render.ColNum
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyNum, fmt.Sprintf("%*s", w, "#"), w),
		Cell: func(_ T, i int) any { return fmt.Sprintf("%*d", w, i+1) },
	}
}

// BlankIndicatorCol reserves the indicator column's width with no glyph, keeping
// column alignment consistent for tables that have no per-row indicator.
func BlankIndicatorCol[T any]() ColumnDef[T] {
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyInd, " ", ColIndicator),
		Cell: func(_ T, _ int) any { return "   " },
	}
}

// IndicatorCol renders each row's status glyph (e.g. downloaded/watched marker).
func IndicatorCol[T HasIndicator]() ColumnDef[T] {
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyInd, " ", ColIndicator),
		Cell: func(item T, _ int) any { return item.GetIndicator() },
	}
}

// TitleFlexCol renders the row title in a flex column that absorbs leftover width.
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

// DurationCol renders the video's total duration in the active format.
// The watched position lives in its own column (WatchedCol).
func DurationCol[T HasDuration]() ColumnDef[T] {
	w := render.ColDuration
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyDuration, calign("Duration", w), w),
		Cell: func(item T, _ int) any { return fmt.Sprintf("%*s", w, render.Duration(item.GetDurationSecs())) },
	}
}

// WatchedCol renders how far playback has reached (the resume position) in the
// active duration format, or blank when nothing has been watched. Paired with
// DurationCol, which now shows only the total.
func WatchedCol[T HasDuration]() ColumnDef[T] {
	w := render.ColDuration
	return ColumnDef[T]{
		Col: etable.NewColumn(KeyWatched, calign("Viewed", w), w),
		Cell: func(item T, _ int) any {
			if pos := item.GetLastPositionSecs(); pos > 0 {
				return fmt.Sprintf("%*s", w, render.Duration(pos))
			}
			return fmt.Sprintf("%*s", w, "")
		},
	}
}

// ViewsCol renders a compact view count (render.Views) under a "Views" header.
func ViewsCol[T HasCount]() ColumnDef[T] {
	w := render.ColViews
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyCount, calign("Views", w+1), w+1),
		Cell: func(item T, _ int) any { return fmt.Sprintf("%*s ", w, render.Views(item.GetCount())) },
	}
}

// SubsCol renders a compact subscriber count (render.Views) under a "Subs" header.
func SubsCol[T HasCount]() ColumnDef[T] {
	w := render.ColViews
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyChSubs, calign("Subs", w+1), w+1),
		Cell: func(item T, _ int) any { return fmt.Sprintf("%*s ", w, render.Views(item.GetCount())) },
	}
}

// SizeCol renders an on-disk file size, right-aligned like ViewsCol. Zero sizes
// render blank (render.Size), so pre-backfill rows show nothing rather than "0".
func SizeCol[T HasSize]() ColumnDef[T] {
	w := render.ColSize
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeySize, calign("Size", w+1), w+1),
		Cell: func(item T, _ int) any { return fmt.Sprintf("%*s ", w, render.Size(item.GetFileSize())) },
	}
}

// DateCol renders the row's upload date in the active format (render.Date).
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
