package tab

import (
	"context"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
)

type localLoadedMsg struct {
	tuipkg.TabTarget
	videos []domain.LocalVideo
	status string
}

// LocalVideoRow wraps domain.LocalVideo to add the presentation-layer indicator.
type LocalVideoRow struct {
	domain.LocalVideo
}

func (r LocalVideoRow) GetIndicator() string {
	switch r.Status {
	case domain.StatusNew:
		return " ● "
	case domain.StatusStarted, domain.StatusWatched:
		return " ○ "
	}
	return "   "
}

func toLocalVideoRows(videos []domain.LocalVideo) []LocalVideoRow {
	rows := make([]LocalVideoRow, len(videos))
	for i := range videos {
		rows[i] = LocalVideoRow{videos[i]}
	}
	return rows
}

// localBackend is the narrow slice of the backend the Local library tab needs:
// downloaded-file bookkeeping plus history logging. Declared consumer-side (ISP).
type localBackend interface {
	api.LibraryBackend
	api.HistoryBackend
}

type Local struct {
	ctx      context.Context
	backend  localBackend
	keys     keymap.KeyMap
	circular bool

	width, height int

	videos []domain.LocalVideo
	loaded bool
	nav    videotable.TableNav
	cols   []videotable.ColumnDef[LocalVideoRow]

	sort sortState
}

func localStyler(lv LocalVideoRow) *lipgloss.Style {
	if lv.Status == domain.StatusStarted || lv.Status == domain.StatusWatched {
		return &styles.Dim
	}
	return nil
}

// localColumns is the full, natural-order column set for the Local library
// list. Extracted so the per-panel column selector (config) and the column-key
// catalog (tab.PanelColumnKeys) share one source of truth.
func localColumns() []videotable.ColumnDef[LocalVideoRow] {
	return []videotable.ColumnDef[LocalVideoRow]{
		videotable.NumCol[LocalVideoRow](),
		videotable.IndicatorCol[LocalVideoRow](),
		videotable.AudioTitleFlexCol[LocalVideoRow](),
		videotable.ChannelCol[LocalVideoRow](),
		videotable.WatchedCol[LocalVideoRow](), videotable.DurationCol[LocalVideoRow](),
		videotable.ViewsCol[LocalVideoRow](),
		videotable.SizeCol[LocalVideoRow](),
		videotable.DateCol[LocalVideoRow](),
	}
}

func NewLocal(ctx context.Context, backend localBackend, keys keymap.KeyMap, circular bool, defaultSort string, wantCols ...string) Local {
	cols := videotable.SelectColumns(localColumns(), wantCols)
	return Local{
		ctx:      ctx,
		backend:  backend,
		keys:     keys,
		circular: circular,
		nav:      videotable.NewTableNav(cols, circular, 2),
		cols:     cols,
		sort:     newSortState(sortModeOr(defaultSort, feed.SortViews), videotable.ColumnKeys(cols)),
	}
}

func (t Local) ID() tuipkg.TabID      { return tuipkg.TabLocal }
func (t Local) Title() string         { return "Local" }
func (t Local) InterceptsInput() bool { return false }
func (t Local) Loading() bool         { return false }
func (t Local) SelectedVideo() (domain.Video, bool) {
	idx := t.nav.Index()
	if idx < 0 || idx >= len(t.videos) {
		return domain.Video{}, false
	}
	lv := t.videos[idx]
	return domain.Video{
		ID:      lv.ID,
		Title:   lv.Title,
		Channel: lv.Channel,
		URL:     "https://www.youtube.com/watch?v=" + lv.ID,
	}, true
}
func (t Local) ShortHelp() []key.Binding {
	return []key.Binding{t.keys.Play, t.keys.Download, t.keys.Delete, t.keys.CopyURL, t.keys.VideoInfo, t.keys.SortChord}
}

func (t Local) Init() tea.Cmd { return t.localLoadCmd("") }

func (t Local) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.nav.Resize(m.Width, m.Height)
		t.nav.SetRows(videotable.BuildRowsStyled(toLocalVideoRows(t.videos), t.cols, localStyler))
	case localLoadedMsg:
		t.videos = m.videos
		feed.SortLocalVideos(t.videos, t.sort.mode)
		t.loaded = true
		t.nav.SetRows(videotable.BuildRowsStyled(toLocalVideoRows(t.videos), t.cols, localStyler))
		if m.status != "" {
			return t, func() tea.Msg { return tuipkg.StatusMsg{Text: m.status} }
		}
	case tuipkg.LocalVideosChangedMsg:
		return t, t.localLoadCmd("")
	case api.Event:
		if m.Kind == api.EventDownloadDone {
			return t, t.localLoadCmd("")
		}
	case tea.KeyPressMsg:
		return t.localHandleKey(m)
	}
	return t, nil
}

func (t Local) View() tea.View {
	header := styles.SectionTitle.Render("Local Library")
	if !t.loaded {
		return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, styles.Dim.PaddingLeft(1).Render("Loading…")))
	}
	if len(t.videos) == 0 {
		return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header,
			styles.Dim.PaddingLeft(1).Render("No local videos. Download some with d.")))
	}
	parts := []string{header, t.nav.View()}
	if s := t.nav.NumBufView(); s != "" {
		parts = append(parts, s)
	}
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

func (t Local) localHandleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if t.sort.handleChord(msg, t.keys.Sort, func(mode int) {
		feed.SortLocalVideos(t.videos, mode)
		t.nav.SetRows(videotable.BuildRowsStyled(toLocalVideoRows(t.videos), t.cols, localStyler))
	}) {
		return t, nil
	}

	if t.nav.HandleNav(msg, t.keys, len(t.videos)) {
		return t, nil
	}

	keys := t.keys
	idx := t.nav.Index()

	switch {
	case key.Matches(msg, keys.Play):
		if idx < len(t.videos) {
			lv := t.videos[idx]
			return t, func() tea.Msg { return tuipkg.LaunchLocalVideoMsg{Video: lv} }
		}
	case key.Matches(msg, keys.Delete):
		if idx < len(t.videos) {
			return t, t.localDeleteCmd(t.videos[idx])
		}
	case key.Matches(msg, keys.CopyURL):
		if idx < len(t.videos) {
			url := "https://www.youtube.com/watch?v=" + t.videos[idx].ID
			return t, func() tea.Msg { return tuipkg.CopyURLMsg{URL: url} }
		}
	case key.Matches(msg, keys.SortChord):
		t.sort.chordActive = true
	}
	return t, nil
}

func (t Local) localDeleteCmd(lv domain.LocalVideo) tea.Cmd {
	return func() tea.Msg {
		ctx := t.ctx
		if err := t.backend.DeleteLocalVideo(ctx, lv.ID); err != nil {
			return tuipkg.StatusMsg{Text: "delete: " + err.Error(), IsErr: true}
		}
		_ = t.backend.AddHistory(ctx, lv.ID, "delete", "") // history is best-effort
		videos, err := t.backend.LocalVideos(ctx)
		if err != nil {
			return tuipkg.StatusMsg{Text: "local: " + err.Error(), IsErr: true}
		}
		return localLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabLocal}, videos: videos, status: "Deleted: " + render.Truncate(lv.Title, 50)}
	}
}

func (t Local) localLoadCmd(status string) tea.Cmd {
	return func() tea.Msg {
		videos, err := t.backend.LocalVideos(t.ctx)
		if err != nil {
			return tuipkg.StatusMsg{Text: "local: " + err.Error(), IsErr: true}
		}
		return localLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabLocal}, videos: videos, status: status}
	}
}
