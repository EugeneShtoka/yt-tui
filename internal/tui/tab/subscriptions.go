package tab

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/nav"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// ── tab-private messages ──────────────────────────────────────────────────────

type subLoadedMsg struct{ videos []domain.Video }
type subAuxLoadedMsg struct {
	positions   map[string]int64
	watched     map[string]bool
	localStatus map[string]domain.VideoStatus
}

// ── Subscriptions ─────────────────────────────────────────────────────────────

// Subscriptions is the Subscriptions tab: all videos from subscribed channels.
type Subscriptions struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	width, height int

	feed    feed.Feed
	cursor  int
	vs      int
	spinner spinner.Model

	positions   map[string]int64
	watched     map[string]bool
	localStatus map[string]domain.VideoStatus
}

func NewSubscriptions(backend api.Backend, keys keymap.KeyMap, circular bool) Subscriptions {
	return Subscriptions{
		backend:  backend,
		keys:     keys,
		circular: circular,
		spinner:  spinner.New(),
	}
}

// ── tui.Tab interface ─────────────────────────────────────────────────────────

func (t Subscriptions) ID() tuipkg.TabID           { return tuipkg.TabSubscriptions }
func (t Subscriptions) Title() string              { return "Subscriptions" }
func (t Subscriptions) ShortHelp() []key.Binding  { return nil }
func (t Subscriptions) InterceptsInput() bool { return false }

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (t Subscriptions) Init() tea.Cmd {
	t.feed.StartRefresh()
	return tea.Batch(t.subLoadCmd(), t.subAuxCmd(), t.spinner.Tick)
}

func (t Subscriptions) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height

	case spinner.TickMsg:
		if t.feed.Loading() || t.feed.Refreshing() {
			var cmd tea.Cmd
			t.spinner, cmd = t.spinner.Update(m)
			return t, cmd
		}

	case subLoadedMsg:
		t.feed = feed.New(m.videos)

	case subAuxLoadedMsg:
		t.positions = m.positions
		t.watched = m.watched
		t.localStatus = m.localStatus

	case tea.KeyMsg:
		return t.subHandleKey(m)
	}
	return t, nil
}

func (t Subscriptions) View() string {
	ctx := VideoListCtx{
		Width:       t.width,
		ShowChannel: true,
		Positions:   t.positions,
		Watched:     t.watched,
		LocalStatus: t.localStatus,
	}
	return renderVideoList(
		ctx,
		"Subscriptions",
		t.feed.Videos(),
		t.cursor, t.vs, t.height,
		t.feed.Loading(), t.feed.Refreshing(),
		t.spinner.View(),
	)
}

// ── key handling ──────────────────────────────────────────────────────────────

func (t Subscriptions) subHandleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := t.feed.Len()
	pageH := t.subPageHeight()

	switch {
	case key.Matches(msg, keys.Up):
		t.cursor, t.vs = nav.Move(t.cursor, t.vs, n, -1, pageH, t.circular)
	case key.Matches(msg, keys.Down):
		t.cursor, t.vs = nav.Move(t.cursor, t.vs, n, +1, pageH, t.circular)
	case key.Matches(msg, keys.PageUp):
		t.cursor, t.vs = nav.Page(t.cursor, t.vs, n, -1, pageH, t.circular)
	case key.Matches(msg, keys.PageDown):
		t.cursor, t.vs = nav.Page(t.cursor, t.vs, n, +1, pageH, t.circular)
	case key.Matches(msg, keys.GotoBottom):
		t.cursor, t.vs = nav.Jump(n-1, n, pageH)

	case key.Matches(msg, keys.Refresh):
		t.feed.StartRefresh()
		return t, tea.Batch(t.subLoadCmd(), t.spinner.Tick)

	case key.Matches(msg, keys.Unsubscribe):
		if v, ok := t.feed.At(t.cursor); ok {
			ch := domain.Channel{ID: v.ChannelID, Name: v.Channel}
			t.feed.RemoveChannel(ch)
			if newN := t.feed.Len(); t.cursor >= newN {
				if newN > 0 {
					t.cursor = newN - 1
				} else {
					t.cursor, t.vs = 0, 0
				}
			}
			return t, func() tea.Msg { return tuipkg.UnsubscribeMsg{Channel: ch} }
		}

	case key.Matches(msg, keys.Play):
		if v, ok := t.feed.At(t.cursor); ok {
			return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v} }
		}
	case key.Matches(msg, keys.PlayAudio):
		if v, ok := t.feed.At(t.cursor); ok {
			return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v, AudioOnly: true} }
		}
	case key.Matches(msg, keys.Download):
		if v, ok := t.feed.At(t.cursor); ok {
			return t, func() tea.Msg { return tuipkg.EnqueueMsg{Video: v} }
		}
	case key.Matches(msg, keys.DownloadAudio):
		if v, ok := t.feed.At(t.cursor); ok {
			return t, func() tea.Msg { return tuipkg.EnqueueMsg{Video: v, AudioOnly: true} }
		}
	case key.Matches(msg, keys.CopyURL):
		if v, ok := t.feed.At(t.cursor); ok {
			return t, func() tea.Msg { return tuipkg.CopyURLMsg{URL: v.URL} }
		}
	case key.Matches(msg, keys.VideoInfo):
		if v, ok := t.feed.At(t.cursor); ok {
			return t, func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: "video_detail", Video: v} }
		}
	case key.Matches(msg, keys.AddList):
		if v, ok := t.feed.At(t.cursor); ok {
			return t, func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: "add_to_playlist", Video: v} }
		}
	case key.Matches(msg, keys.HideChannel):
		if v, ok := t.feed.At(t.cursor); ok {
			ch := domain.Channel{ID: v.ChannelID, Name: v.Channel}
			return t, func() tea.Msg { return tuipkg.HideChannelMsg{Channel: ch} }
		}
	}
	return t, nil
}

// ── background commands ───────────────────────────────────────────────────────

func (t Subscriptions) subLoadCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		channels, err := t.backend.GetSubscribedChannels(ctx)
		if err != nil {
			return tuipkg.StatusMsg{Text: "subscriptions: " + err.Error(), IsErr: true}
		}
		ids := make([]string, len(channels))
		for i, ch := range channels {
			ids[i] = ch.ID
		}
		videos, err := t.backend.GetAllChannelVideos(ctx, ids)
		if err != nil {
			return tuipkg.StatusMsg{Text: "subscriptions: " + err.Error(), IsErr: true}
		}
		feed.SortVideos(videos, feed.SortDate)
		return subLoadedMsg{videos}
	}
}

func (t Subscriptions) subAuxCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		positions, _ := t.backend.AllVideoPositions(ctx)
		watched, _ := t.backend.WatchedVideoIDs(ctx)
		localVids, _ := t.backend.LocalVideos(ctx)
		localStatus := make(map[string]domain.VideoStatus, len(localVids))
		for i := range localVids {
			localStatus[localVids[i].ID] = localVids[i].Status
		}
		return subAuxLoadedMsg{positions: positions, watched: watched, localStatus: localStatus}
	}
}

func (t Subscriptions) subPageHeight() int {
	h := t.height - 3 // section title (2 lines incl. MarginBottom) + col header
	if h < 1 {
		h = 1
	}
	return h
}
