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

type recCacheMsg struct{ videos []domain.Video }
type recFetchedMsg struct{ videos []domain.Video }
type recAuxLoadedMsg struct {
	positions   map[string]int64
	watched     map[string]bool
	localStatus map[string]domain.VideoStatus
}
type recHiddenMsg struct{ videoID string }

// ── Recommended ───────────────────────────────────────────────────────────────

// Recommended is the Recommended tab: a YouTube-sourced video feed.
type Recommended struct {
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

func NewRecommended(backend api.Backend, keys keymap.KeyMap, circular bool) Recommended {
	sp := spinner.New()
	return Recommended{
		backend:  backend,
		keys:     keys,
		circular: circular,
		spinner:  sp,
	}
}

// ── tui.Tab interface ─────────────────────────────────────────────────────────

func (t Recommended) ID() tuipkg.TabID          { return tuipkg.TabRecommended }
func (t Recommended) Title() string             { return "Recommended" }
func (t Recommended) ShortHelp() []key.Binding { return nil }
func (t Recommended) InterceptsInput() bool { return false }

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (t Recommended) Init() tea.Cmd {
	t.feed.StartRefresh()
	return tea.Batch(
		t.recLoadCacheCmd(),
		t.recLoadAuxCmd(),
		t.spinner.Tick,
	)
}

func (t Recommended) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height

	case spinner.TickMsg:
		if t.feed.Loading() || t.feed.Refreshing() {
			var cmd tea.Cmd
			t.spinner, cmd = t.spinner.Update(m)
			return t, cmd
		}

	case recCacheMsg:
		t.feed = feed.NewStarting(m.videos)
		return t, t.recFetchCmd()

	case recFetchedMsg:
		t.cursor = t.feed.Merge(m.videos, t.cursor, 0)
		t.feed.FinishFetch()

	case recAuxLoadedMsg:
		t.positions = m.positions
		t.watched = m.watched
		t.localStatus = m.localStatus

	case recHiddenMsg:
		t.feed.RemoveVideo(m.videoID)
		if t.cursor >= t.feed.Len() && t.cursor > 0 {
			t.cursor--
		}

	case tea.KeyMsg:
		return t.recHandleKey(m)
	}
	return t, nil
}

func (t Recommended) View() string {
	ctx := VideoListCtx{
		Width:       t.width,
		ShowChannel: true,
		Positions:   t.positions,
		Watched:     t.watched,
		LocalStatus: t.localStatus,
	}
	return renderVideoList(
		ctx,
		"Recommended for you",
		t.feed.Videos(),
		t.cursor, t.vs, t.height,
		t.feed.Loading(), t.feed.Refreshing(),
		t.spinner.View(),
	)
}

// ── key handling ──────────────────────────────────────────────────────────────

func (t Recommended) recHandleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := t.feed.Len()
	pageH := t.recPageHeight()

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
		return t, tea.Batch(t.recFetchCmd(), t.spinner.Tick)

	case key.Matches(msg, keys.ForceRefresh):
		t.feed.Clear()
		t.feed.StartRefresh()
		return t, tea.Batch(t.recClearAndFetchCmd(), t.spinner.Tick)

	case key.Matches(msg, keys.HideVideo):
		if v, ok := t.feed.At(t.cursor); ok {
			return t, t.recHideVideoCmd(v)
		}

	case key.Matches(msg, keys.HideChannel):
		if v, ok := t.feed.At(t.cursor); ok {
			ch := domain.Channel{ID: v.ChannelID, Name: v.Channel}
			return t, func() tea.Msg { return tuipkg.HideChannelMsg{Channel: ch} }
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
	}
	return t, nil
}

// ── background commands ───────────────────────────────────────────────────────

func (t Recommended) recLoadCacheCmd() tea.Cmd {
	return func() tea.Msg {
		videos, err := t.backend.GetFeedCache(context.Background(), "recommended")
		if err != nil || len(videos) == 0 {
			return t.recFetchCmd()() // no cache — go straight to fetch
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

func (t Recommended) recHideVideoCmd(v domain.Video) tea.Cmd {
	return func() tea.Msg {
		_ = t.backend.HideRecVideo(context.Background(), v.ID)
		return recHiddenMsg{videoID: v.ID}
	}
}

func (t Recommended) recPageHeight() int {
	h := t.height - 2 // section header + col header
	if h < 1 {
		h = 1
	}
	return h
}
