package ui

import (
	"fmt"
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

// update handles navigation keys directly and returns an intent for the actions
// (play/delete/…) the router owns. items is the shared download queue.
func (v *downloadingView) update(msg tea.KeyMsg, keys keyMap, items []downloader.Item, pageSize int, circular bool) downloadingIntent {
	n := len(items)
	switch {
	case key.Matches(msg, keys.Up):
		v.cursor, v.vs = vsMove(v.cursor, v.vs, n, -1, pageSize, circular)
	case key.Matches(msg, keys.Down):
		v.cursor, v.vs = vsMove(v.cursor, v.vs, n, +1, pageSize, circular)
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
	return downloadingIntent{}
}

// render draws the Downloading tab. items and playAfter are shared router state.
func (v downloadingView) render(items []downloader.Item, playAfter map[string]bool, width int, downloadKey string, height int) string {
	header := styleSectionTitle.Render("Downloading")
	headerH := lipgloss.Height(header)

	if len(items) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header, "",
			styleDim.PaddingLeft(1).Render("No active downloads. Press "+downloadKey+" on any video to start."))
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
