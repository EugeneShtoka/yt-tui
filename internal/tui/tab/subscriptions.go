package tab

import (
	"context"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
)

type subLoadedMsg struct {
	videos   []domain.Video
	channels []domain.Channel
}

type Subscriptions struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	width, height int

	feed           feed.Feed
	channelAliases map[string]string
	spinner        spinner.Model
	nav            videotable.TableNav
	cols           []videotable.ColumnDef[videotable.VideoData]
	aux            videotable.AuxData

	sortMode        int
	sortChordActive bool
}

func NewSubscriptions(backend api.Backend, keys keymap.KeyMap, circular bool) Subscriptions {
	cols := []videotable.ColumnDef[videotable.VideoData]{
		videotable.VideoNumCol(), videotable.VideoIndicatorCol(), videotable.VideoTitleCol(),
		videotable.VideoChannelCol(), videotable.VideoDurationCol(), videotable.VideoCountCol(), videotable.VideoDateCol(),
	}
	return Subscriptions{
		backend:  backend,
		keys:     keys,
		circular: circular,
		spinner:  spinner.New(),
		nav:      videotable.NewTableNav(videotable.NewVideoTable(cols), circular, 2),
		cols:     cols,
	}
}

func (t Subscriptions) ID() tuipkg.TabID      { return tuipkg.TabSubscriptions }
func (t Subscriptions) Title() string         { return "Subscriptions" }
func (t Subscriptions) InterceptsInput() bool { return false }
func (t Subscriptions) ShortHelp() []key.Binding {
	return []key.Binding{t.keys.Play, t.keys.Download, t.keys.Unsubscribe, t.keys.CopyURL, t.keys.VideoInfo, t.keys.SortChord}
}

func (t Subscriptions) Init() tea.Cmd {
	t.feed.StartRefresh()
	return tea.Batch(t.subLoadCmd(), videotable.LoadAuxDataCmd(t.backend), t.spinner.Tick)
}

func (t Subscriptions) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.nav.Resize(m.Width, m.Height)
		t.nav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.feed.Videos(), t.aux, t.channelAliases), t.cols))

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
		t.nav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.feed.Videos(), t.aux, t.channelAliases), t.cols))
		t.nav.GotoRow(0)

	case videotable.AuxDataMsg:
		t.aux = m
		t.nav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.feed.Videos(), t.aux, t.channelAliases), t.cols))

	case tuipkg.RefreshPositionsMsg:
		return t, videotable.LoadAuxDataCmd(t.backend)

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
	parts := []string{header, t.nav.View()}
	if s := t.nav.NumBufView(); s != "" {
		parts = append(parts, s)
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
		t.nav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.feed.Videos(), t.aux, t.channelAliases), t.cols))
		return t, nil
	}

	if t.nav.HandleNav(msg, t.keys, t.feed.Len()) {
		return t, nil
	}

	keys := t.keys
	idx := t.nav.Index()

	switch {
	case key.Matches(msg, keys.Refresh):
		t.feed.StartRefresh()
		return t, tea.Batch(t.subLoadCmd(), t.spinner.Tick)
	case key.Matches(msg, keys.Unsubscribe):
		if v, ok := t.feed.At(idx); ok {
			ch := domain.Channel{ID: v.ChannelID, Name: v.Channel}
			t.feed.RemoveChannel(ch)
			t.nav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.feed.Videos(), t.aux, t.channelAliases), t.cols))
			return t, func() tea.Msg { return tuipkg.UnsubscribeMsg{Channel: ch} }
		}
	case key.Matches(msg, keys.SortChord):
		t.sortChordActive = true
	default:
		if v, ok := t.feed.At(idx); ok {
			if cmd, ok := HandleVideoAction(msg, v, keys); ok {
				return t, cmd
			}
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
		return subLoadedMsg{videos: videos, channels: channels}
	}
}
