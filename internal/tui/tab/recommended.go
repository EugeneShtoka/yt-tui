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

type recCacheMsg struct{ videos []domain.Video }
type recFetchedMsg struct{ videos []domain.Video }
type recHiddenMsg struct{ videoID string }

type Recommended struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	width, height int

	feed    feed.Feed
	spinner spinner.Model
	nav     videotable.TableNav
	cols    []videotable.ColumnDef[videotable.VideoData]
	aux     videotable.AuxData

	sortMode        int
	sortChordActive bool
}

func NewRecommended(backend api.Backend, keys keymap.KeyMap, circular bool) Recommended {
	cols := []videotable.ColumnDef[videotable.VideoData]{
		videotable.NumCol[videotable.VideoData](), videotable.IndicatorCol[videotable.VideoData](), videotable.TitleFlexCol[videotable.VideoData](),
		videotable.ChannelCol[videotable.VideoData](), videotable.DurationCol[videotable.VideoData](), videotable.ViewsCol[videotable.VideoData](), videotable.DateCol[videotable.VideoData](),
	}
	return Recommended{
		backend:  backend,
		keys:     keys,
		circular: circular,
		spinner:  spinner.New(),
		nav:      videotable.NewTableNav(videotable.NewVideoTable(cols), circular, 2),
		cols:     cols,
	}
}

func (t Recommended) ID() tuipkg.TabID      { return tuipkg.TabRecommended }
func (t Recommended) Title() string         { return "Recommended" }
func (t Recommended) InterceptsInput() bool { return false }
func (t Recommended) SelectedVideo() (domain.Video, bool) {
	return t.feed.At(t.nav.Index())
}
func (t Recommended) ShortHelp() []key.Binding {
	return []key.Binding{t.keys.Play, t.keys.Download, t.keys.HideVideo, t.keys.CopyURL, t.keys.VideoInfo, t.keys.SortChord}
}

func (t Recommended) Init() tea.Cmd {
	t.feed.StartRefresh()
	return tea.Batch(t.recLoadCacheCmd(), videotable.LoadAuxDataCmd(t.backend), t.spinner.Tick)
}

func (t Recommended) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.nav.Resize(m.Width, m.Height)
		t.nav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.feed.Videos(), t.aux), t.cols))

	case spinner.TickMsg:
		if t.feed.Loading() || t.feed.Refreshing() {
			var cmd tea.Cmd
			t.spinner, cmd = t.spinner.Update(m)
			return t, cmd
		}

	case recCacheMsg:
		t.feed = feed.NewStarting(m.videos)
		t.feed.Sort(t.sortMode)
		t.nav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.feed.Videos(), t.aux), t.cols))
		t.nav.GotoRow(0)
		return t, t.recFetchCmd()

	case recFetchedMsg:
		cursor := t.feed.Merge(m.videos, t.nav.Index(), 0)
		t.feed.FinishFetch()
		t.feed.Sort(t.sortMode)
		t.nav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.feed.Videos(), t.aux), t.cols))
		t.nav.GotoRow(cursor)
		return t, t.recSaveCacheCmd()

	case videotable.AuxDataMsg:
		t.aux = m
		t.nav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.feed.Videos(), t.aux), t.cols))

	case tuipkg.RefreshPositionsMsg:
		return t, videotable.LoadAuxDataCmd(t.backend)

	case recHiddenMsg:
		t.feed.RemoveVideo(m.videoID)
		t.nav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.feed.Videos(), t.aux), t.cols))

	case tea.KeyPressMsg:
		return t.recHandleKey(m)
	}
	return t, nil
}

func (t Recommended) View() tea.View {
	headerText := "Recommended for you"
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

func (t Recommended) recHandleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		t.nav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.feed.Videos(), t.aux), t.cols))
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
		return t, tea.Batch(t.recFetchCmd(), t.spinner.Tick)
	case key.Matches(msg, keys.ForceRefresh):
		t.feed.Clear()
		t.feed.StartRefresh()
		return t, tea.Batch(t.recClearAndFetchCmd(), t.spinner.Tick)
	case key.Matches(msg, keys.HideVideo):
		if v, ok := t.feed.At(idx); ok {
			return t, t.recHideVideoCmd(v)
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

func (t Recommended) recLoadCacheCmd() tea.Cmd {
	return func() tea.Msg {
		videos, err := t.backend.GetFeedCache(context.Background(), "recommended")
		if err != nil || len(videos) == 0 {
			return t.recFetchCmd()()
		}
		return recCacheMsg{videos}
	}
}

func (t Recommended) recFetchCmd() tea.Cmd {
	return func() tea.Msg {
		videos, err := t.backend.Recommended(context.Background())
		if err != nil {
			return tuipkg.StatusMsg{Text: "recommended: " + err.Error(), IsErr: true}
		}
		return recFetchedMsg{videos}
	}
}

func (t Recommended) recClearAndFetchCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		_ = t.backend.ClearRecommended(ctx)
		videos, err := t.backend.Recommended(ctx)
		if err != nil {
			return tuipkg.StatusMsg{Text: "recommended: " + err.Error(), IsErr: true}
		}
		return recFetchedMsg{videos}
	}
}

func (t Recommended) recSaveCacheCmd() tea.Cmd {
	videos := t.feed.Videos()
	return func() tea.Msg {
		_ = t.backend.SaveFeedCache(context.Background(), "recommended", videos)
		return nil
	}
}

func (t Recommended) recHideVideoCmd(v domain.Video) tea.Cmd {
	return func() tea.Msg {
		_ = t.backend.HideRecVideo(context.Background(), v.ID)
		return recHiddenMsg{videoID: v.ID}
	}
}
