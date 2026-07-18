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

// ── tab-private messages ──────────────────────────────────────────────────────

type recCacheMsg struct{ videos []domain.Video }
type recFetchedMsg struct{ videos []domain.Video }
type recAuxLoadedMsg struct {
	positions   map[string]int64
	watched     map[string]bool
	localStatus map[string]domain.VideoStatus
}
type recHiddenMsg struct{ videoID string }

// ── Recommended ───────────────────────────────────────────────────────────────

type Recommended struct {
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

func NewRecommended(backend api.Backend, keys keymap.KeyMap, circular bool) Recommended {
	return Recommended{
		backend:  backend,
		keys:     keys,
		circular: circular,
		spinner:  spinner.New(),
		table:    newTable(),
	}
}

func (t Recommended) ID() tuipkg.TabID          { return tuipkg.TabRecommended }
func (t Recommended) Title() string             { return "Recommended" }
func (t Recommended) ShortHelp() []key.Binding { return nil }
func (t Recommended) InterceptsInput() bool     { return false }

func (t Recommended) Init() tea.Cmd {
	t.feed.StartRefresh()
	return tea.Batch(t.recLoadCacheCmd(), t.recLoadAuxCmd(), t.spinner.Tick)
}

func (t Recommended) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	case recCacheMsg:
		t.feed = feed.NewStarting(m.videos)
		t.table.SetRows(toVideoRows(t.feed.Videos(), t.positions, t.watched, t.localStatus, true))
		t.table.GotoTop()
		return t, t.recFetchCmd()

	case recFetchedMsg:
		cursor := t.feed.Merge(m.videos, t.table.Cursor(), 0)
		t.feed.FinishFetch()
		t.table.SetRows(toVideoRows(t.feed.Videos(), t.positions, t.watched, t.localStatus, true))
		t.table.SetCursor(cursor)
		return t, t.recSaveCacheCmd()

	case tuipkg.RefreshPositionsMsg:
		return t, t.recLoadAuxCmd()

	case recAuxLoadedMsg:
		t.positions = m.positions
		t.watched = m.watched
		t.localStatus = m.localStatus
		t.table.SetRows(toVideoRows(t.feed.Videos(), t.positions, t.watched, t.localStatus, true))

	case recHiddenMsg:
		t.feed.RemoveVideo(m.videoID)
		t.table.SetRows(toVideoRows(t.feed.Videos(), t.positions, t.watched, t.localStatus, true))

	case tea.KeyMsg:
		return t.recHandleKey(m)
	}
	return t, nil
}

func (t Recommended) View() string {
	headerText := "Recommended for you"
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

func (t Recommended) recHandleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		return t, tea.Batch(t.recFetchCmd(), t.spinner.Tick)

	case key.Matches(msg, keys.ForceRefresh):
		t.feed.Clear()
		t.feed.StartRefresh()
		return t, tea.Batch(t.recClearAndFetchCmd(), t.spinner.Tick)

	case key.Matches(msg, keys.HideVideo):
		if v, ok := t.feed.At(t.table.Cursor()); ok {
			return t, t.recHideVideoCmd(v)
		}
	case key.Matches(msg, keys.HideChannel):
		if v, ok := t.feed.At(t.table.Cursor()); ok {
			ch := domain.Channel{ID: v.ChannelID, Name: v.Channel}
			return t, func() tea.Msg { return tuipkg.HideChannelMsg{Channel: ch} }
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

func (t Recommended) recLoadAuxCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		positions, _ := t.backend.AllVideoPositions(ctx)
		watched, _ := t.backend.WatchedVideoIDs(ctx)
		localVids, _ := t.backend.LocalVideos(ctx)
		localStatus := make(map[string]domain.VideoStatus, len(localVids))
		for i := range localVids {
			localStatus[localVids[i].ID] = localVids[i].Status
		}
		return recAuxLoadedMsg{positions: positions, watched: watched, localStatus: localStatus}
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
