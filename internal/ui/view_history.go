package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// historyView owns the History tab's private state.
type historyView struct {
	entries       []domain.HistoryEntry
	cursor        int
	vs            int // viewStart: first visible row
	loaded        bool
	detailVideoID string
	detail        []domain.HistoryEntry
}

// histIntentKind describes what cross-tab or cross-layer action the router must handle.
type histIntentKind uint8

const (
	histIntentNone        histIntentKind = iota
	histIntentDrillSearch                // navigate to search tab; entry.Details is the query
	histIntentPlay                       // play video built from entry
	histIntentDelete                     // delete entry; router handles shared-state cleanup
	histIntentHide                       // hide channel; router calls hideChannel
)

// historyIntent is the value returned by update for the router to act on.
type historyIntent struct {
	kind  histIntentKind
	entry domain.HistoryEntry
}

// load fetches the history log. Cursor resets to 0 on each load (matches original behaviour).
func (v *historyView) load(store Store, setErr func(string)) {
	entries, err := store.HistoryVideos(200)
	if err != nil {
		setErr("history: " + err.Error())
		return
	}
	v.entries = entries
	v.cursor = 0
	v.detailVideoID = ""
	v.loaded = true
}

// clear resets all history state (called by "clear history" command).
func (v *historyView) clear() {
	*v = historyView{}
}

// context returns the context ID for the current cursor position.
func (v *historyView) context(ctx viewCtx) ContextID {
	if v.detailVideoID != "" {
		return CtxHistoryVideo
	}
	if v.cursor < len(v.entries) && v.entries[v.cursor].EventType == "search" {
		return CtxHistorySearch
	}
	return CtxHistoryVideo
}

func (v historyView) currentVideo(_ viewCtx) (domain.Video, bool) { return domain.Video{}, false }

func (v *historyView) jumpTo(idx int, ctx viewCtx) {
	v.cursor, v.vs = vsJump(idx, len(v.entries), ctx.pageSize)
}

func (v *historyView) jumpToLast(ctx viewCtx) {
	v.jumpTo(len(v.entries)-1, ctx)
}

// update handles key input for the History tab. It returns an intent for any
// action that requires the router (cross-tab navigation, play, delete, hide).
// The view mutates its own cursor/scroll/detail state directly.
func (v *historyView) update(msg tea.KeyMsg, ctx viewCtx) viewIntent {
	keys := ctx.keys
	if v.detailVideoID != "" {
		switch {
		case key.Matches(msg, keys.Escape), key.Matches(msg, keys.Left):
			v.detailVideoID = ""
			v.detail = nil
		}
		return nil
	}

	n := len(v.entries)
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
			e := v.entries[v.cursor]
			if e.EventType != "search" {
				return historyIntent{kind: histIntentPlay, entry: e}
			}
		}
	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
		if v.cursor < n {
			e := v.entries[v.cursor]
			if e.EventType == "search" {
				return historyIntent{kind: histIntentDrillSearch, entry: e}
			}
			if detail, err := ctx.db.VideoHistory(e.VideoID); err == nil {
				v.detailVideoID = e.VideoID
				v.detail = detail
			}
		}
	case key.Matches(msg, keys.Delete):
		if v.cursor < n {
			e := v.entries[v.cursor]
			v.entries = append(v.entries[:v.cursor], v.entries[v.cursor+1:]...)
			if v.cursor >= len(v.entries) && v.cursor > 0 {
				v.cursor--
			}
			return historyIntent{kind: histIntentDelete, entry: e}
		}
	case key.Matches(msg, keys.HideChannel):
		if v.cursor < n {
			e := v.entries[v.cursor]
			if e.EventType != "search" {
				return historyIntent{kind: histIntentHide, entry: e}
			}
		}
	}
	return nil
}

// apply performs the router-side effects of a history action.
func (in historyIntent) apply(m *Model) tea.Cmd {
	switch in.kind {
	case histIntentDrillSearch:
		// Navigate to search tab with query pre-filled; user presses Enter again to search.
		m.activeTab = tabSearch
		cmd := m.onTabActivated()
		m.searchInput.SetValue(in.entry.Details)
		m.searchInput.CursorEnd()
		return cmd
	case histIntentPlay:
		e := in.entry
		v := domain.Video{ID: e.VideoID, Title: e.Title, URL: "https://www.youtube.com/watch?v=" + e.VideoID}
		m.playVideo(v)
	case histIntentDelete:
		e := in.entry
		if e.EventType == "search" {
			_ = m.db.DeleteSearchHistory(e.Details)
			m.setStatus("Removed search: "+truncate(e.Details, 50), false)
		} else {
			if lv, ok := m.library.ByID(e.VideoID); ok {
				_ = os.Remove(lv.FilePath)
				_ = m.db.DeleteLocalVideo(lv.ID)
				if lv2, err := m.db.LocalVideos(); err == nil {
					m.library.Set(lv2)
				}
			}
			_ = m.db.DeleteVideoHistory(e.VideoID)
			_ = m.db.DeleteVideoPosition(e.VideoID)
			delete(m.streamedVideoIDs, e.VideoID)
			delete(m.videoPositions, e.VideoID)
			m.setStatus("Deleted: "+truncate(e.Title, 50), false)
		}
	case histIntentHide:
		m.hideChannel(in.entry.ChannelID, in.entry.Channel)
	}
	return nil
}

// render draws the history tab, dispatching to renderDetail when a video is open.
func (v historyView) render(ctx viewCtx, height int) string {
	width := ctx.width
	if v.detailVideoID != "" {
		return v.renderDetail(width, height)
	}

	header := styleSectionTitle.Render("History")
	headerH := lipgloss.Height(header)

	if len(v.entries) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header,
			styleDim.Render("No history yet."))
	}

	colStatus := 14
	titleW := width - colNum - 1 - 2 - colStatus - 1 - colChannel - 1 - colDuration - 1 - colViews - 1 - colDate
	if titleW < 20 {
		titleW = 20
	}

	colHeader := strings.Repeat(" ", colNum) + " " + "  " +
		styleColHeader.Width(colStatus).Render("Type") + " " +
		styleColHeader.Width(titleW).Render("Title") + " " +
		styleColHeader.Width(colChannel).Render("Channel") + " " +
		styleColHeader.Width(colDuration).Render("Duration") + " " +
		styleColHeader.Width(colViews).Render("Views") + " " +
		styleColHeader.Width(colDate).Render("Date")

	start, end := scrollWindowAt(v.vs, len(v.entries), height-headerH-1)
	var rows []string
	rows = append(rows, colHeader)
	for i := start; i < end && i < len(v.entries); i++ {
		e := v.entries[i]

		indicator := "  "
		sep := " "
		numStyle := styleRowNum
		statusStyle := styleWarning.Width(colStatus)
		if i == v.cursor {
			indicator = styleSelected.Render("▶ ")
			numStyle = numStyle.Background(colorBgSelect)
			sep = lipgloss.NewStyle().Background(colorBgSelect).Render(" ")
			statusStyle = statusStyle.Background(colorBgSelect)
		}
		numStr := numStyle.Render(fmt.Sprintf("%*d", colNum, i+1))

		if e.EventType == "search" {
			queryW := width - colNum - 1 - 2 - colStatus - 1
			queryStyle := styleChannel.Width(queryW)
			if i == v.cursor {
				queryStyle = styleSelected.Width(queryW)
			}
			rows = append(rows, numStr+sep+indicator+statusStyle.Render("search")+sep+
				queryStyle.Render(truncate(e.Details, queryW)))
			continue
		}

		title := truncate(e.Title, titleW)
		channel := truncate(e.Channel, colChannel-2)
		dur := fmtDuration(e.Duration)
		views := fmtViews(e.ViewCount)
		date := fmtDate(e.UploadDate)

		titleStyle := styleNormal.Width(titleW)
		chStyle := styleChannel.Width(colChannel)
		durStyle := styleDuration.Width(colDuration)
		viewsStyle := styleDuration.Width(colViews)
		dateStyle := styleChannel.Width(colDate)
		if i == v.cursor {
			titleStyle = styleSelected.Width(titleW)
			chStyle = chStyle.Background(colorBgSelect)
			durStyle = durStyle.Background(colorBgSelect)
			viewsStyle = viewsStyle.Background(colorBgSelect)
			dateStyle = dateStyle.Background(colorBgSelect)
		}
		rows = append(rows, numStr+sep+indicator+statusStyle.Render(e.EventType)+sep+
			titleStyle.Render(title)+sep+
			chStyle.Render(channel)+sep+
			durStyle.Render(dur)+sep+
			viewsStyle.Render(views)+sep+
			dateStyle.Render(date))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(rows, "\n"))
}

func (v historyView) renderDetail(width, height int) string {
	title := ""
	if len(v.detail) > 0 {
		title = v.detail[0].Title
	}
	header := styleSectionTitle.Render("← " + truncate(title, width-4))
	headerH := lipgloss.Height(header)

	colEvW := 14
	colTsW := 19
	var rows []string
	for i, e := range v.detail {
		if i >= height-headerH {
			break
		}
		evType := styleWarning.Width(colEvW).Render(e.EventType)
		ts := styleChannel.Width(colTsW).Render(e.Timestamp.Format("2006-01-02 15:04:05"))
		detail := styleDim.Render(e.Details)
		rows = append(rows, "  "+evType+" "+ts+" "+detail)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(rows, "\n"))
}
