package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// downloadingView owns the Downloading tab's private cursor/scroll state.
// The download queue itself lives in the shared downloader (router-owned) and
// deletion mutates the shared local library, so actions are returned as intents
// for the router to perform (see docs/TABVIEW_DESIGN.md, Finding 2).
type downloadingView struct {
	cursor int
	vs     int // viewStart: first visible row
}

// dlIntentKind describes an action the router must perform.
type dlIntentKind uint8

const (
	dlIntentNone dlIntentKind = iota
	dlIntentPlay
	dlIntentPlayAudio
	dlIntentHide
	dlIntentDelete
	dlIntentCopyURL
)

// downloadingIntent is returned by update for the router to act on.
type downloadingIntent struct {
	kind dlIntentKind
	item downloader.Item
}

// jumpTo implements goto-line navigation.
func (v *downloadingView) jumpTo(idx, n, pageSize int) {
	v.cursor, v.vs = vsJump(idx, n, pageSize)
}

// jumpToLast implements goto-last navigation.
func (v *downloadingView) jumpToLast(n, pageSize int) {
	v.cursor, v.vs = vsJump(n-1, n, pageSize)
}

// reclamp keeps cursor/scroll valid after the queue length changes.
func (v *downloadingView) reclamp(n, pageSize int) {
	v.cursor, v.vs = vsMove(clamp(v.cursor, n), v.vs, n, 0, pageSize, false)
}

// currentItem returns the selected queue item, if any.
func (v downloadingView) currentItem(items []downloader.Item) (downloader.Item, bool) {
	if i := v.cursor; i >= 0 && i < len(items) {
		return items[i], true
	}
	return downloader.Item{}, false
}

// context reports the Downloading tab's sort/chord context.
func (v downloadingView) context() ContextID { return CtxDownloading }

// update handles navigation keys directly and returns an intent for the actions
// (play/delete/…) the router owns. The download queue is shared (ctx.dlItems).
func (v *downloadingView) update(msg tea.KeyMsg, ctx viewCtx) viewIntent {
	keys, items := ctx.keys, ctx.dlItems
	n := len(items)
	switch {
	case key.Matches(msg, keys.Up):
		v.cursor, v.vs = vsMove(v.cursor, v.vs, n, -1, ctx.pageSize, ctx.circular)
	case key.Matches(msg, keys.Down):
		v.cursor, v.vs = vsMove(v.cursor, v.vs, n, +1, ctx.pageSize, ctx.circular)
	case key.Matches(msg, keys.Play):
		if v.cursor < n {
			return downloadingIntent{kind: dlIntentPlay, item: items[v.cursor]}
		}
	case key.Matches(msg, keys.PlayAudio):
		if v.cursor < n {
			return downloadingIntent{kind: dlIntentPlayAudio, item: items[v.cursor]}
		}
	case key.Matches(msg, keys.HideChannel):
		if v.cursor < n {
			return downloadingIntent{kind: dlIntentHide, item: items[v.cursor]}
		}
	case key.Matches(msg, keys.Delete):
		if v.cursor < n {
			return downloadingIntent{kind: dlIntentDelete, item: items[v.cursor]}
		}
	case key.Matches(msg, keys.CopyURL):
		return downloadingIntent{kind: dlIntentCopyURL}
	}
	return nil
}

// apply performs the router-side effects of a downloading action.
func (in downloadingIntent) apply(m *Model) tea.Cmd {
	switch in.kind {
	case dlIntentPlay:
		item := in.item
		if item.Status == downloader.StatusComplete {
			if lv, ok := m.localVideoIDs[item.Video.ID]; ok {
				m.launchVideo(lv)
			}
		} else {
			m.playVideo(item.Video)
		}
	case dlIntentPlayAudio:
		m.playAudio(in.item.Video)
	case dlIntentHide:
		v := in.item.Video
		m.hideChannel(v.ChannelID, v.Channel)
	case dlIntentDelete:
		item := in.item
		m.downloader.Remove(item.Video.ID)
		// Also remove any downloaded or partial file and DB record.
		if item.FilePath != "" {
			_ = os.Remove(item.FilePath)
		}
		_ = m.db.DeleteLocalVideo(item.Video.ID)
		_ = m.db.AddHistory(item.Video.ID, "delete", "")
		if lv, err := m.db.LocalVideos(); err == nil {
			m.localVideos = lv
		}
		m.downloading.reclamp(len(m.downloader.Items()), m.pageSize())
		m.setStatus("Deleted: "+truncate(item.Video.Title, 50), false)
	case dlIntentCopyURL:
		m.copyCurrentURL()
	}
	return nil
}

// render draws the Downloading tab. The queue and play-after set are shared
// router state, passed via ctx.
func (v downloadingView) render(ctx viewCtx, height int) string {
	items, playAfter, width := ctx.dlItems, ctx.playAfter, ctx.width
	header := styleSectionTitle.Render("Downloading")
	headerH := lipgloss.Height(header)

	if len(items) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header, "",
			styleDim.PaddingLeft(1).Render("No active downloads. Press "+ctx.keys.Download.Help().Key+" on any video to start."))
	}

	titleW := width - colNum - 1 - colChannel - colDuration - 42 - 6
	if titleW < 20 {
		titleW = 20
	}
	colHeader := strings.Repeat(" ", colNum) + " " + "  " +
		styleColHeader.Width(titleW).Render("Title") + " " +
		styleColHeader.Width(colChannel).Render("Channel") + " " +
		styleColHeader.Width(colDuration).Render("Duration") + " " +
		styleColHeader.Render("Status")

	start, end := scrollWindowAt(v.vs, len(items), height-headerH-1)
	var rows []string
	rows = append(rows, colHeader)
	for i := start; i < end && i < len(items); i++ {
		rows = append(rows, renderDownloadRow(items[i], i == v.cursor, i+1, width, playAfter))
	}
	parts := []string{header, "", strings.Join(rows, "\n")}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func renderDownloadRow(item downloader.Item, selected bool, num, width int, playAfter map[string]bool) string {
	titleW := width - colNum - 1 - colChannel - colDuration - 42 - 6
	if titleW < 20 {
		titleW = 20
	}

	titleSuffix := ""
	if playAfter[item.Video.ID] {
		titleSuffix = " ♪►"
	}
	title := truncate(item.Video.Title, titleW)
	channel := truncate(item.Video.Channel, colChannel-2)
	dur := item.Video.DurationStr()
	dlType := ""
	if item.Type == downloader.TypeAudio {
		dlType = styleChannel.Render("[audio]")
	}

	var statusPart string
	switch item.Status {
	case downloader.StatusPending:
		statusPart = stylePendingTag.Render("pending")
	case downloader.StatusActive:
		barW := 20
		bar := progressBar(item.Progress, barW)
		statusPart = fmt.Sprintf("%s %5.1f%%  %s  ETA %s",
			bar, item.Progress, styleActiveTag.Render(item.Speed), item.ETA)
	case downloader.StatusComplete:
		statusPart = styleCompleteTag.Render("done ✓")
	case downloader.StatusFailed:
		msg := "failed"
		if item.Err != nil {
			msg = "failed: " + truncate(item.Err.Error(), 30)
		}
		statusPart = styleFailedTag.Render(msg)
	}

	numStr := fmt.Sprintf("%*d", colNum, num)
	indicator := "  "
	if selected {
		indicator = styleSelected.Render("▶ ")
	}

	titleStyled := styleNormal.Width(titleW).Render(title + titleSuffix)
	if selected {
		titleStyled = styleSelected.Width(titleW).Render(title + titleSuffix)
	}

	return numStr + " " + indicator + titleStyled + " " +
		styleChannel.Width(colChannel).Render(channel) + " " +
		styleDuration.Width(colDuration).Render(dur) + " " +
		dlType + " " + statusPart
}
