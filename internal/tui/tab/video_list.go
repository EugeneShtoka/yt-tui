package tab

import (
	"fmt"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/tui/nav"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/charmbracelet/lipgloss"
)

// VideoListCtx carries the display context shared across all video-list views.
type VideoListCtx struct {
	Width       int
	ShowChannel bool
	Positions   map[string]int64       // video resume positions (ms)
	Watched     map[string]bool        // streamed-without-downloading indicator
	LocalStatus map[string]domain.VideoStatus // downloaded video status
}

// videoTitleWidth computes the title column width from the remaining space.
func (c VideoListCtx) videoTitleWidth() int {
	w := c.Width - render.ColNum - 1 - 2 - render.ColDuration - 1 - render.ColViews - 1 - render.ColDate
	if c.ShowChannel {
		w -= render.ColChannel + 1
	}
	if w < 20 {
		w = 20
	}
	return w
}

// renderVideoList renders a full video-list tab body.
// spinnerView is the current spinner frame; pass "" when not loading.
func renderVideoList(
	ctx VideoListCtx,
	title string,
	videos []domain.Video,
	cursor, vs, height int,
	loading, refreshing bool,
	spinnerView string,
) string {
	headerText := title
	if refreshing && spinnerView != "" {
		headerText += "  " + styles.Dim.Render(spinnerView+" refreshing…")
	}
	header := styles.SectionTitle.Render(headerText)
	headerH := lipgloss.Height(header)
	listH := height - headerH

	if loading && !refreshing {
		body := spinnerView + " Loading…"
		return lipgloss.JoinVertical(lipgloss.Left, header, body)
	}
	if len(videos) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header,
			styles.Dim.Render("No videos. Press r to refresh."))
	}

	body := renderVideoRows(ctx, videos, cursor, vs, listH)
	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}

func renderVideoRows(ctx VideoListCtx, videos []domain.Video, cursor, vs, height int) string {
	if height <= 0 {
		height = 10
	}
	titleW := ctx.videoTitleWidth()
	colHdr := renderVideoColHeader(ctx, titleW)
	start, end := nav.Window(vs, len(videos), height-1)

	rows := make([]string, 0, end-start+1)
	rows = append(rows, colHdr)
	for i := start; i < end; i++ {
		rows = append(rows, renderVideoRow(ctx, videos[i], i == cursor, titleW, i+1))
	}
	return strings.Join(rows, "\n")
}

func renderVideoColHeader(ctx VideoListCtx, titleW int) string {
	h := strings.Repeat(" ", render.ColNum) + " " + "  " +
		styles.ColHeader.Width(titleW).Render("Title") + " "
	if ctx.ShowChannel {
		h += styles.ColHeader.Width(render.ColChannel).Render("Channel") + " "
	}
	return h +
		styles.ColHeader.Width(render.ColDuration).Render("Duration") + " " +
		styles.ColHeader.Width(render.ColViews).Render("Views") + " " +
		styles.ColHeader.Width(render.ColDate).Render("Date")
}

func renderVideoRow(ctx VideoListCtx, v domain.Video, selected bool, titleW, num int) string {
	dur := v.DurationStr()
	if posMs := ctx.Positions[v.ID]; posMs > 0 {
		dur = render.DurationWithPos(posMs, v.Duration)
	}

	localSt, hasLocal := ctx.LocalStatus[v.ID]
	indicator := "  "
	sep := " "
	numStyle := styles.RowNum
	chStyle := styles.Channel.Width(render.ColChannel)
	durStyle := styles.Duration.Width(render.ColDuration)
	viewsStyle := styles.Duration.Width(render.ColViews)
	dateStyle := styles.Channel.Width(render.ColDate)
	var titleStyle lipgloss.Style

	switch {
	case selected:
		titleStyle = styles.Selected.Width(titleW)
		indicator = styles.Selected.Render("▶ ")
		numStyle = numStyle.Background(styles.ColorBgSelect)
		sep = lipgloss.NewStyle().Background(styles.ColorBgSelect).Render(" ")
		chStyle = chStyle.Background(styles.ColorBgSelect)
		durStyle = durStyle.Background(styles.ColorBgSelect)
		viewsStyle = viewsStyle.Background(styles.ColorBgSelect)
		dateStyle = dateStyle.Background(styles.ColorBgSelect)
	case hasLocal && localSt == domain.StatusNew:
		titleStyle = styles.Bold.Width(titleW)
		indicator = styles.Success.Render("● ")
	case hasLocal && (localSt == domain.StatusStarted || localSt == domain.StatusWatched):
		titleStyle = styles.Dim.Width(titleW)
		indicator = styles.Dim.Render("○ ")
	case !hasLocal && ctx.Watched[v.ID]:
		titleStyle = styles.Dim.Width(titleW)
		indicator = styles.Dim.Render("○ ")
	default:
		titleStyle = styles.Normal.Width(titleW)
	}

	numStr := numStyle.Render(fmt.Sprintf("%*d", render.ColNum, num))
	row := numStr + sep + indicator + titleStyle.Render(render.Truncate(v.Title, titleW)) + sep
	if ctx.ShowChannel {
		row += chStyle.Render(render.Truncate(v.Channel, render.ColChannel-2)) + sep
	}
	return row +
		durStyle.Render(dur) + sep +
		viewsStyle.Render(v.ViewsStr()) + sep +
		dateStyle.Render(v.DateStr())
}
