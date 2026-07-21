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

// ChLatestDurationCol renders the latest video's duration with position support.
func ChLatestDurationCol[T HasLatestVideo]() ColumnDef[T] {
	w := render.ColDurationPos
	return ColumnDef[T]{
		Col: etable.NewColumn(KeyDuration, calign("Duration", w), w),
		Cell: func(item T, _ int) any {
			vd := item.GetLatestVideo()
			total := render.Duration(vd.GetDurationSecs())
			if pos := vd.GetLastPositionSecs(); pos > 0 {
				return ralign(render.Duration(pos)+"/"+total, w)
			}
			return ralign(total, w)
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
