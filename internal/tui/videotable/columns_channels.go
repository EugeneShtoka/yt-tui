package videotable

import (
	"fmt"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	etable "github.com/evertras/bubble-table/table"
)

// HasChannelTags is implemented by channel-list row types that carry a tag slice.
type HasChannelTags interface {
	GetTags() []string
}

// HasChannelState is implemented by channel-list row types that expose a
// subscription-state label (already resolved to display text, e.g. "YT" /
// "Local" / "—"), keeping the domain enum out of the videotable layer.
type HasChannelState interface {
	GetStateLabel() string
}

// HasBlocked is implemented by channel-list row types that report whether the
// channel is blocked, so the indicator column can render a marker.
type HasBlocked interface {
	IsBlocked() bool
}

// HasLatestVideo is implemented by channel-list row types that embed latest-video data.
// The returned VideoData provides duration, views, and date for the "latest video" columns.
type HasLatestVideo interface {
	GetLatestVideo() VideoData
}

// ChNameCol renders the channel display name at the narrower channel-list width.
func ChNameCol[T HasChannelInfo]() ColumnDef[T] {
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyChName, "Channel", ColChName),
		Cell: func(item T, _ int) any { return item.GetChannelName() },
	}
}

// ChTagsCol renders comma-joined channel tags.
func ChTagsCol[T HasChannelTags]() ColumnDef[T] {
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyChTags, "Tags", ColChTags),
		Cell: func(item T, _ int) any { return strings.Join(item.GetTags(), ", ") },
	}
}

// ChStateCol renders the channel's subscription-state label (YT / Local / —).
func ChStateCol[T HasChannelState]() ColumnDef[T] {
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyChState, "State", ColChState),
		Cell: func(item T, _ int) any { return item.GetStateLabel() },
	}
}

// ChBlockedIndicatorCol renders a block marker for blocked channels and a blank
// cell otherwise, occupying the same fixed slot as the generic indicator column.
func ChBlockedIndicatorCol[T HasBlocked]() ColumnDef[T] {
	return ColumnDef[T]{
		Col: etable.NewColumn(KeyInd, " ", ColIndicator),
		Cell: func(item T, _ int) any {
			if item.IsBlocked() {
				return " ⛔"
			}
			return "   "
		},
	}
}

// ChLatestDurationCol renders the latest video's total duration.
func ChLatestDurationCol[T HasLatestVideo]() ColumnDef[T] {
	w := render.ColDuration
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyDuration, calign("Duration", w), w),
		Cell: func(item T, _ int) any { return ralign(render.Duration(item.GetLatestVideo().GetDurationSecs()), w) },
	}
}

// ChLatestWatchedCol renders how far the latest video has been watched, or blank
// when unwatched. Paired with ChLatestDurationCol (total only).
func ChLatestWatchedCol[T HasLatestVideo]() ColumnDef[T] {
	w := render.ColDuration
	return ColumnDef[T]{
		Col: etable.NewColumn(KeyWatched, calign("Viewed", w), w),
		Cell: func(item T, _ int) any {
			if pos := item.GetLatestVideo().GetLastPositionSecs(); pos > 0 {
				return ralign(render.Duration(pos), w)
			}
			return ralign("", w)
		},
	}
}

// ChLatestViewsCol renders the latest video's view count.
func ChLatestViewsCol[T HasLatestVideo]() ColumnDef[T] {
	w := render.ColViews
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyChViews, calign("Views", w+1), w+1),
		Cell: func(item T, _ int) any { return fmt.Sprintf("%*s ", w, render.Views(item.GetLatestVideo().GetCount())) },
	}
}

// ChLatestDateCol renders the latest video's upload date.
func ChLatestDateCol[T HasLatestVideo]() ColumnDef[T] {
	return ColumnDef[T]{
		Col:  etable.NewColumn(KeyDate, "Date", render.ColDate),
		Cell: func(item T, _ int) any { return render.Date(item.GetLatestVideo().GetRawDate()) },
	}
}
