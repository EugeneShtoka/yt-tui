package tab

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type subLoadedMsg struct{ videos []domain.Video }
type subAuxLoadedMsg struct {
	positions   map[string]int64
	watched     map[string]bool
	localStatus map[string]domain.VideoStatus
}

type Subscriptions struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	width, height int

	feed    feed.Feed
	spinner spinner.Model
	table   table.Model
	numBuf  string

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
		table:    newTable(),
	}
}

func (t Subscriptions) ID() tuipkg.TabID          { return tuipkg.TabSubscriptions }
func (t Subscriptions) Title() string             { return "Subscriptions" }
func (t Subscriptions) ShortHelp() []key.Binding { return nil }
func (t Subscriptions) InterceptsInput() bool     { return false }

func (t Subscriptions) Init() tea.Cmd {
	t.feed.StartRefresh()
	return tea.Batch(t.subLoadCmd(), t.subAuxCmd(), t.spinner.Tick)
}

func (t Subscriptions) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.table.SetColumns(computeVideoColumns(t.width, true))
		t.table.SetHeight(t.height - 2)
		t.table.SetRows(toVideoRows(t.feed.Videos(), t.positions, t.watched, t.localStatus, true))

	case spinner.TickMsg:
		if t.feed.Loading() || t.feed.Refreshing() {
			var cmd tea.Cmd
			t.spinner, cmd = t.spinner.Update(m)
			return t, cmd
		}

	case subLoadedMsg:
		t.feed = feed.New(m.videos)
		t.table.SetRows(toVideoRows(t.feed.Videos(), t.positions, t.watched, t.localStatus, true))
		t.table.GotoTop()

	case subAuxLoadedMsg:
		t.positions = m.positions
		t.watched = m.watched
		t.localStatus = m.localStatus
		t.table.SetRows(toVideoRows(t.feed.Videos(), t.positions, t.watched, t.localStatus, true))

	case tuipkg.RefreshPositionsMsg:
		return t, t.subAuxCmd()

	case tea.KeyMsg:
		return t.subHandleKey(m)
	}
	return t, nil
}

func (t Subscriptions) View() string {
	headerText := "Subscriptions"
	if t.feed.Refreshing() && t.spinner.View() != "" {
		headerText += "  " + styles.Dim.Render(t.spinner.View()+" refreshing…")
	}
	header := styles.SectionTitle.Render(headerText)
	if t.feed.Loading() && !t.feed.Refreshing() {
		return lipgloss.JoinVertical(lipgloss.Left, header, " "+t.spinner.View()+" Loading…")
	}
	if t.feed.Len() == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header,
			styles.Dim.PaddingLeft(1).Render("No videos. Press r to refresh."))
	}
	parts := []string{header, t.table.View()}
	if t.numBuf != "" {
		parts = append(parts, gotoLineView(t.numBuf))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (t Subscriptions) subHandleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if checkGotoNum(&t.numBuf, msg) {
		return t, nil
	}
	numBuf := t.numBuf
	t.numBuf = ""

	keys := t.keys
	n := t.feed.Len()

	switch {
	case key.Matches(msg, keys.GotoLine):
		if numBuf != "" {
			applyGoto(numBuf, &t.table)
		} else {
			t.table.GotoBottom()
		}
	case key.Matches(msg, keys.GotoBottom):
		t.table.GotoBottom()
	case key.Matches(msg, keys.Up):
		if t.circular && n > 0 && t.table.Cursor() == 0 {
			t.table.GotoBottom()
		} else {
			t.table.MoveUp(1)
		}
	case key.Matches(msg, keys.Down):
		if t.circular && n > 0 && t.table.Cursor() == n-1 {
			t.table.GotoTop()
		} else {
			t.table.MoveDown(1)
		}
	case key.Matches(msg, keys.PageUp):
		t.table.MoveUp(t.table.Height())
	case key.Matches(msg, keys.PageDown):
		t.table.MoveDown(t.table.Height())

	case key.Matches(msg, keys.Refresh):
		t.feed.StartRefresh()
		return t, tea.Batch(t.subLoadCmd(), t.spinner.Tick)

	case key.Matches(msg, keys.Unsubscribe):
		if v, ok := t.feed.At(t.table.Cursor()); ok {
			ch := domain.Channel{ID: v.ChannelID, Name: v.Channel}
			t.feed.RemoveChannel(ch)
			t.table.SetRows(toVideoRows(t.feed.Videos(), t.positions, t.watched, t.localStatus, true))
			return t, func() tea.Msg { return tuipkg.UnsubscribeMsg{Channel: ch} }
		}
	case key.Matches(msg, keys.Play):
		if v, ok := t.feed.At(t.table.Cursor()); ok {
			return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v} }
		}
	case key.Matches(msg, keys.PlayAudio):
		if v, ok := t.feed.At(t.table.Cursor()); ok {
			return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v, AudioOnly: true} }
		}
	case key.Matches(msg, keys.Download):
		if v, ok := t.feed.At(t.table.Cursor()); ok {
			return t, func() tea.Msg { return tuipkg.EnqueueMsg{Video: v} }
		}
	case key.Matches(msg, keys.DownloadAudio):
		if v, ok := t.feed.At(t.table.Cursor()); ok {
			return t, func() tea.Msg { return tuipkg.EnqueueMsg{Video: v, AudioOnly: true} }
		}
	case key.Matches(msg, keys.CopyURL):
		if v, ok := t.feed.At(t.table.Cursor()); ok {
			return t, func() tea.Msg { return tuipkg.CopyURLMsg{URL: v.URL} }
		}
	case key.Matches(msg, keys.VideoInfo):
		if v, ok := t.feed.At(t.table.Cursor()); ok {
			return t, func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: "video_detail", Video: v} }
		}
	case key.Matches(msg, keys.AddList):
		if v, ok := t.feed.At(t.table.Cursor()); ok {
			return t, func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: "add_to_playlist", Video: v} }
		}
	case key.Matches(msg, keys.HideChannel):
		if v, ok := t.feed.At(t.table.Cursor()); ok {
			ch := domain.Channel{ID: v.ChannelID, Name: v.Channel}
			return t, func() tea.Msg { return tuipkg.HideChannelMsg{Channel: ch} }
		}
	}
	return t, nil
}

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
