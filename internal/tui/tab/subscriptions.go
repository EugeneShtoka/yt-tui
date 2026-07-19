package tab

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type subLoadedMsg struct {
	videos   []domain.Video
	channels []domain.Channel
}
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

	feed           feed.Feed
	channelAliases map[string]string
	spinner        spinner.Model
	table          table.Model
	numBuf         string

	sortMode        int
	sortChordActive bool
	gotoTopActive   bool

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

func (t Subscriptions) ID() tuipkg.TabID         { return tuipkg.TabSubscriptions }
func (t Subscriptions) Title() string            { return "Subscriptions" }
func (t Subscriptions) InterceptsInput() bool    { return false }
func (t Subscriptions) ShortHelp() []key.Binding {
	return []key.Binding{t.keys.Play, t.keys.Download, t.keys.Unsubscribe, t.keys.CopyURL, t.keys.VideoInfo, t.keys.SortChord}
}

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
		t.table.SetRows(toVideoRows(t.videosWithAliases(), t.positions, t.watched, t.localStatus, true, t.width))

	case spinner.TickMsg:
		if t.feed.Loading() || t.feed.Refreshing() {
			var cmd tea.Cmd
			t.spinner, cmd = t.spinner.Update(m)
			return t, cmd
		}

	case subLoadedMsg:
		t.feed = feed.New(m.videos)
		t.feed.Sort(t.sortMode)
		t.channelAliases = make(map[string]string, len(m.channels))
		for _, ch := range m.channels {
			if ch.Alias != "" {
				t.channelAliases[ch.ID] = ch.Alias
			}
		}
		t.table.SetRows(toVideoRows(t.videosWithAliases(), t.positions, t.watched, t.localStatus, true, t.width))
		t.table.GotoTop()

	case subAuxLoadedMsg:
		t.positions = m.positions
		t.watched = m.watched
		t.localStatus = m.localStatus
		t.table.SetRows(toVideoRows(t.videosWithAliases(), t.positions, t.watched, t.localStatus, true, t.width))

	case tuipkg.RefreshPositionsMsg:
		return t, t.subAuxCmd()

	case tea.KeyPressMsg:
		return t.subHandleKey(m)
	}
	return t, nil
}

func (t Subscriptions) View() tea.View {
	headerText := "Subscriptions"
	if t.feed.Refreshing() && t.spinner.View() != "" {
		headerText += "  " + styles.Dim.Render(t.spinner.View()+" refreshing…")
	}
	header := styles.SectionTitle.Render(headerText)
	if t.feed.Loading() && !t.feed.Refreshing() {
		return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, " "+t.spinner.View()+" Loading…"))
	}
	if t.feed.Len() == 0 {
		return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header,
			styles.Dim.PaddingLeft(1).Render("No videos. Press r to refresh.")))
	}
	parts := []string{header, t.table.View()}
	if t.numBuf != "" {
		parts = append(parts, gotoLineView(t.numBuf))
	}
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

func (t Subscriptions) subHandleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if t.sortChordActive {
		t.sortChordActive = false
		sk := t.keys.Sort
		switch {
		case key.Matches(msg, sk.Date):
			t.sortMode = feed.SortDate
		case key.Matches(msg, sk.Views):
			t.sortMode = feed.SortViews
		case key.Matches(msg, sk.Name):
			t.sortMode = feed.SortName
		case key.Matches(msg, sk.Channel):
			t.sortMode = feed.SortChannel
		case key.Matches(msg, sk.Duration):
			t.sortMode = feed.SortDuration
		}
		t.feed.Sort(t.sortMode)
		t.table.SetRows(toVideoRows(t.videosWithAliases(), t.positions, t.watched, t.localStatus, true, t.width))
		return t, nil
	}

	if consumed, doTop := handleGotoPrefix(&t.gotoTopActive, t.keys, msg); consumed {
		if doTop {
			t.numBuf = ""
			t.table.GotoTop()
		}
		return t, nil
	}

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
			t.table.SetRows(toVideoRows(t.videosWithAliases(), t.positions, t.watched, t.localStatus, true, t.width))
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
	case key.Matches(msg, keys.SortChord):
		t.sortChordActive = true
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
		return subLoadedMsg{videos: videos, channels: channels}
	}
}

func (t Subscriptions) videosWithAliases() []domain.Video {
	vids := t.feed.Videos()
	if len(t.channelAliases) == 0 {
		return vids
	}
	result := make([]domain.Video, len(vids))
	copy(result, vids)
	for i := range result {
		if a, ok := t.channelAliases[result[i].ChannelID]; ok {
			result[i].Channel = a
		}
	}
	return result
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
