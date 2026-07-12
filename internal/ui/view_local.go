package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// localView owns the Local tab's private cursor/scroll/sort state. The library
// collection itself (m.library, a library.Library) is written from several sites
// outside this tab, so it stays on the router; deletion is returned as an intent (see
// docs/TABVIEW_DESIGN.md, Finding 2).
type localView struct {
	cursor int
	vs     int // viewStart: first visible row
	sort   int // one of vidSort*; default vidSortNone
}

// localIntentKind describes an action the router must perform.
type localIntentKind uint8

const (
	localIntentNone localIntentKind = iota
	localIntentPlay
	localIntentDelete
	localIntentCopyURL
)

// localIntent is returned by update for the router to act on.
type localIntent struct {
	kind  localIntentKind
	video db.LocalVideo
}

// jumpTo implements goto-line navigation.
func (v *localView) jumpTo(idx, n, pageSize int) {
	v.cursor, v.vs = vsJump(idx, n, pageSize)
}

// jumpToLast implements goto-last navigation.
func (v *localView) jumpToLast(n, pageSize int) {
	v.cursor, v.vs = vsJump(n-1, n, pageSize)
}

// reclamp keeps cursor/scroll valid after the library length changes.
func (v *localView) reclamp(n, pageSize int) {
	v.cursor, v.vs = vsMove(clamp(v.cursor, n), v.vs, n, 0, pageSize, false)
}

// currentVideo returns the selected library entry as a youtube.Video.
func (v localView) currentVideo(videos []db.LocalVideo) (youtube.Video, bool) {
	if i := v.cursor; i >= 0 && i < len(videos) {
		lv := videos[i]
		return youtube.Video{
			ID:    lv.ID,
			Title: lv.Title,
			URL:   "https://www.youtube.com/watch?v=" + lv.ID,
		}, true
	}
	return youtube.Video{}, false
}

// context reports the Local tab's sort/chord context.
func (v localView) context(ctx viewCtx) ContextID { return CtxLocal }

// update handles navigation keys directly and returns an intent for the actions
// (play/delete/…) the router owns. The library is shared (ctx.library).
func (v *localView) update(msg tea.KeyMsg, ctx viewCtx) viewIntent {
	keys, videos := ctx.keys, ctx.library.Videos()
	n := len(videos)
	switch {
	case key.Matches(msg, keys.Up):
		v.cursor, v.vs = vsMove(v.cursor, v.vs, n, -1, ctx.pageSize, ctx.circular)
	case key.Matches(msg, keys.Down):
		v.cursor, v.vs = vsMove(v.cursor, v.vs, n, +1, ctx.pageSize, ctx.circular)
	case key.Matches(msg, keys.PageUp):
		v.cursor, v.vs = vsPage(v.cursor, v.vs, n, -1, ctx.pageSize, ctx.circular)
	case key.Matches(msg, keys.PageDown):
		v.cursor, v.vs = vsPage(v.cursor, v.vs, n, +1, ctx.pageSize, ctx.circular)
	case key.Matches(msg, keys.Play):
		if v.cursor < n {
			return localIntent{kind: localIntentPlay, video: videos[v.cursor]}
		}
	case key.Matches(msg, keys.Delete):
		if v.cursor < n {
			return localIntent{kind: localIntentDelete, video: videos[v.cursor]}
		}
	case key.Matches(msg, keys.CopyURL):
		return localIntent{kind: localIntentCopyURL}
	}
	return nil
}

// apply performs the router-side effects of a local-library action.
func (in localIntent) apply(m *Model) tea.Cmd {
	switch in.kind {
	case localIntentPlay:
		m.launchVideo(in.video)
	case localIntentDelete:
		lv := in.video
		_ = os.Remove(lv.FilePath)
		_ = m.db.DeleteLocalVideo(lv.ID)
		_ = m.db.AddHistory(lv.ID, "delete", "")
		if lv2, err := m.db.LocalVideos(); err == nil {
			m.library.Set(lv2)
		}
		m.local.reclamp(m.library.Len(), m.pageSize())
		m.setStatus("Deleted: "+truncate(lv.Title, 50), false)
	case localIntentCopyURL:
		m.copyCurrentURL()
	}
	return nil
}

// render draws the Local tab. The library is shared router state (ctx); titleW
// is computed by the router (it depends on the global column layout).
func (v localView) render(ctx viewCtx, height int) string {
	videos, titleW := ctx.library.Videos(), ctx.localTitleW
	header := styleSectionTitle.Render("Local Library")
	headerH := lipgloss.Height(header)

	if len(videos) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header,
			styleDim.Render("No local videos. Download some with s."))
	}

	colHeader := strings.Repeat(" ", colNum) + " " + "  " +
		styleColHeader.Width(titleW).Render("Title") + " " +
		styleColHeader.Width(colChannel).Render("Channel") + " " +
		styleColHeader.Width(colDuration).Render("Duration") + " " +
		styleColHeader.Width(colViews).Render("Views") + " " +
		styleColHeader.Width(colDate).Render("Date")

	start, end := scrollWindowAt(v.vs, len(videos), height-headerH-1)
	var rows []string
	rows = append(rows, colHeader)
	for i := start; i < end && i < len(videos); i++ {
		rows = append(rows, renderLocalRow(videos[i], i == v.cursor, i+1, titleW))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(rows, "\n"))
}

func renderLocalRow(lv db.LocalVideo, selected bool, num, titleW int) string {
	title := truncate(lv.Title, titleW)
	channel := truncate(lv.Channel, colChannel-2)
	dur := fmtDuration(lv.Duration)
	if lv.Status == db.StatusStarted && lv.LastPositionMs > 0 {
		dur = fmtDurWithPos(lv.LastPositionMs, lv.Duration)
	}
	views := fmtViews(lv.ViewCount)
	date := fmtDate(lv.UploadDate)
	dlType := ""
	if lv.DownloadType == "audio" {
		dlType = " ♪"
	}

	indicator := "  "
	sep := " "
	numStyle := styleRowNum
	chStyle := styleChannel.Width(colChannel)
	durStyle := styleDuration.Width(colDuration)
	viewsStyle := styleDuration.Width(colViews)
	dateStyle := styleChannel.Width(colDate)
	var ts lipgloss.Style

	switch {
	case selected:
		indicator = styleSelected.Render("▶ ")
		ts = styleSelected.Width(titleW)
		numStyle = numStyle.Background(colorBgSelect)
		sep = lipgloss.NewStyle().Background(colorBgSelect).Render(" ")
		chStyle = chStyle.Background(colorBgSelect)
		durStyle = durStyle.Background(colorBgSelect)
		viewsStyle = viewsStyle.Background(colorBgSelect)
		dateStyle = dateStyle.Background(colorBgSelect)
	case lv.Status == db.StatusNew:
		ts = styleBold.Width(titleW)
		indicator = styleSuccess.Render("● ")
	case lv.Status == db.StatusStarted:
		ts = styleNormal.Width(titleW)
		indicator = styleDim.Render("○ ")
	case lv.Status == db.StatusWatched:
		ts = styleDim.Width(titleW)
		indicator = styleDim.Render("  ")
	default:
		ts = styleNormal.Width(titleW)
	}

	numStr := numStyle.Render(fmt.Sprintf("%*d", colNum, num))
	return numStr + sep + indicator +
		ts.Render(title+dlType) + sep +
		chStyle.Render(channel) + sep +
		durStyle.Render(dur) + sep +
		viewsStyle.Render(views) + sep +
		dateStyle.Render(date)
}
