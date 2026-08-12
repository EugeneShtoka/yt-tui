package tab

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

func (t Downloading) fetchItemsCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := t.backend.DownloadItems(t.ctx)
		if err != nil {
			return tuipkg.StatusMsg{Text: "download queue: " + err.Error(), IsErr: true}
		}
		return dlItemsMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabDownloading}, items: items}
	}
}

func (t Downloading) subscribeEventsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(t.ctx)
		ch, err := t.backend.Events(ctx)
		if err != nil {
			cancel()
			return tuipkg.StatusMsg{Text: "events: " + err.Error(), IsErr: true}
		}
		return dlEventsReadyMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabDownloading}, ch: ch, cancel: cancel}
	}
}

func (t Downloading) waitEventCmd() tea.Cmd {
	if t.events == nil {
		return nil
	}
	ch := t.events
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return dlEventsClosedMsg{retryDelay: time.Second}
		}
		return ev
	}
}

// resubscribeCmd attempts to re-open the event stream. The backoff wait is done
// by the caller (via tea.Tick) so no command holds a worker in a sleep; on
// failure it hands back the doubled delay for the next round.
func (t Downloading) resubscribeCmd(delay time.Duration) tea.Cmd {
	b := t.backend
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(t.ctx)
		ch, err := b.Events(ctx)
		if err != nil {
			cancel()
			next := delay * 2
			if next > 30*time.Second {
				next = 30 * time.Second
			}
			return dlEventsClosedMsg{retryDelay: next}
		}
		return dlEventsReadyMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabDownloading}, ch: ch, cancel: cancel}
	}
}
