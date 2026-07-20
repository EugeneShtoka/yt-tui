package tab

import (
	"context"
	"fmt"
	"os"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	etable "github.com/evertras/bubble-table/table"
)

type localLoadedMsg struct {
	videos []domain.LocalVideo
	status string
}

type Local struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	width, height int

	videos []domain.LocalVideo
	loaded bool
	nav    videotable.TableNav
	cols   []videotable.ColumnDef[domain.LocalVideo]

	sortMode        int
	sortChordActive bool
}

func localColumns(durW int) []videotable.ColumnDef[domain.LocalVideo] {
	return []videotable.ColumnDef[domain.LocalVideo]{
		{
			Col:  etable.NewColumn(videotable.KeyNum, fmt.Sprintf("%4s", "#"), 4),
			Cell: func(lv domain.LocalVideo, i int) any { return fmt.Sprintf("%4d", i+1) },
		},
		{
			Col: etable.NewColumn(videotable.KeyIndicator, " ", 3),
			Cell: func(lv domain.LocalVideo, _ int) any {
				switch lv.Status {
				case domain.StatusNew:
					return " ● "
				case domain.StatusStarted, domain.StatusWatched:
					return " ○ "
				}
				return "   "
			},
		},
		{
			Col: etable.NewFlexColumn(videotable.KeyTitle, "Title", 1),
			Cell: func(lv domain.LocalVideo, _ int) any {
				t := lv.Title
				if lv.DownloadType == "audio" {
					t += " ♪"
				}
				return t
			},
		},
		{
			Col:  etable.NewColumn(videotable.KeyChannel, "Channel", render.ColChannel),
			Cell: func(lv domain.LocalVideo, _ int) any { return lv.Channel },
		},
		{
			Col: etable.NewColumn(videotable.KeyDuration, "Duration", durW),
			Cell: func(lv domain.LocalVideo, _ int) any {
				dur := render.Duration(lv.Duration)
				if lv.Status == domain.StatusStarted && lv.LastPositionMs > 0 {
					dur = render.Duration(int(lv.LastPositionMs / 1000))
				}
				return fmt.Sprintf("%*s", durW, dur)
			},
		},
		{
			Col:  etable.NewColumn(videotable.KeyViews, "Views", render.ColViews),
			Cell: func(lv domain.LocalVideo, _ int) any { return fmt.Sprintf("%8s", render.Views(lv.ViewCount)) },
		},
		{
			Col:  etable.NewColumn(videotable.KeyDate, "Date", render.ColDate),
			Cell: func(lv domain.LocalVideo, _ int) any { return render.Date(lv.UploadDate) },
		},
	}
}

func localStyler(lv domain.LocalVideo) *lipgloss.Style {
	if lv.Status == domain.StatusStarted || lv.Status == domain.StatusWatched {
		return &styles.Dim
	}
	return nil
}

func NewLocal(backend api.Backend, keys keymap.KeyMap, circular bool) Local {
	cols := localColumns(render.ColDuration)
	return Local{
		backend:  backend,
		keys:     keys,
		circular: circular,
		nav:      videotable.NewTableNav(videotable.NewTable(cols), circular, 2),
		cols:     cols,
	}
}

func (t Local) ID() tuipkg.TabID         { return tuipkg.TabLocal }
func (t Local) Title() string            { return "Local" }
func (t Local) InterceptsInput() bool    { return false }
func (t Local) ShortHelp() []key.Binding {
	return []key.Binding{t.keys.Play, t.keys.Download, t.keys.Delete, t.keys.CopyURL, t.keys.VideoInfo, t.keys.SortChord}
}

func (t Local) Init() tea.Cmd { return t.localLoadCmd("") }

func (t Local) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.nav.Resize(m.Width, m.Height)
		t.nav.SetRows(videotable.BuildRowsStyled(t.videos, t.cols, localStyler))
	case localLoadedMsg:
		t.videos = m.videos
		feed.SortLocalVideos(t.videos, t.sortMode)
		t.loaded = true
		t.nav.SetRows(videotable.BuildRowsStyled(t.videos, t.cols, localStyler))
		if m.status != "" {
			return t, func() tea.Msg { return tuipkg.StatusMsg{Text: m.status} }
		}
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
		feed.SortLocalVideos(t.videos, t.sortMode)
		t.nav.SetRows(videotable.BuildRowsStyled(t.videos, t.cols, localStyler))
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
		t.sortChordActive = true
	}
	return t, nil
}

func (t Local) localDeleteCmd(lv domain.LocalVideo) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		_ = os.Remove(lv.FilePath)
		_ = t.backend.DeleteLocalVideo(ctx, lv.ID)
		_ = t.backend.AddHistory(ctx, lv.ID, "delete", "")
		videos, err := t.backend.LocalVideos(ctx)
		if err != nil {
			return tuipkg.StatusMsg{Text: "local: " + err.Error(), IsErr: true}
		}
		return localLoadedMsg{videos: videos, status: "Deleted: " + render.Truncate(lv.Title, 50)}
	}
}

func (t Local) localLoadCmd(status string) tea.Cmd {
	return func() tea.Msg {
		videos, err := t.backend.LocalVideos(context.Background())
		if err != nil {
			return tuipkg.StatusMsg{Text: "local: " + err.Error(), IsErr: true}
		}
		return localLoadedMsg{videos: videos, status: status}
	}
}
