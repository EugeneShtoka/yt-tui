package tab

import (
	"context"
	"os"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	table  table.Model
	numBuf string

	sortMode        int
	sortChordActive bool
	gotoTopActive   bool
}

func NewLocal(backend api.Backend, keys keymap.KeyMap, circular bool) Local {
	return Local{backend: backend, keys: keys, circular: circular, table: newTable()}
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
		t.table.SetColumns(t.localColumns())
		t.table.SetHeight(t.height - 2)
		t.table.SetRows(t.toLocalRows())
	case localLoadedMsg:
		t.videos = m.videos
		feed.SortLocalVideos(t.videos, t.sortMode)
		t.loaded = true
		t.table.SetRows(t.toLocalRows())
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
	parts := []string{header, t.table.View()}
	if t.numBuf != "" {
		parts = append(parts, gotoLineView(t.numBuf))
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
		t.table.SetRows(t.toLocalRows())
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
	n := len(t.videos)

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
	case key.Matches(msg, keys.Play):
		if t.table.Cursor() < n {
			lv := t.videos[t.table.Cursor()]
			return t, func() tea.Msg { return tuipkg.LaunchLocalVideoMsg{Video: lv} }
		}
	case key.Matches(msg, keys.Delete):
		if t.table.Cursor() < n {
			lv := t.videos[t.table.Cursor()]
			return t, t.localDeleteCmd(lv)
		}
	case key.Matches(msg, keys.CopyURL):
		if t.table.Cursor() < n {
			lv := t.videos[t.table.Cursor()]
			url := "https://www.youtube.com/watch?v=" + lv.ID
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

func (t Local) localColumns() []table.Column {
	titleW := t.width - render.ColNum - colIndicator - render.ColChannel - render.ColDuration - render.ColViews - render.ColDate
	if titleW < 20 {
		titleW = 20
	}
	return []table.Column{
		{Title: ralign("#", render.ColNum), Width: render.ColNum},
		{Title: " ", Width: colIndicator},
		{Title: "Title", Width: titleW},
		{Title: "Channel", Width: render.ColChannel},
		{Title: "Duration", Width: render.ColDuration},
		{Title: "Views", Width: render.ColViews},
		{Title: "Date", Width: render.ColDate},
	}
}

func (t Local) toLocalRows() []table.Row {
	titleW := t.width - render.ColNum - colIndicator - render.ColChannel - render.ColDuration - render.ColViews - render.ColDate
	if titleW < 20 {
		titleW = 20
	}
	rows := make([]table.Row, len(t.videos))
	for i := range t.videos {
		lv := &t.videos[i]
		dur := render.Duration(lv.Duration)
		if lv.Status == domain.StatusStarted && lv.LastPositionMs > 0 {
			dur = render.DurationWithPos(lv.LastPositionMs, lv.Duration)
		}
		title := lv.Title
		if lv.DownloadType == "audio" {
			title += " ♪"
		}
		title = render.Truncate(title, titleW)
		var ind string
		switch lv.Status {
		case domain.StatusNew:
			ind = " ● "
		case domain.StatusStarted, domain.StatusWatched:
			ind = " ○ "
		default:
			ind = "   "
		}
		rows[i] = table.Row{
			rowNum(i), ind, title, lv.Channel,
			ralign(dur, render.ColDuration), ralign(render.Views(lv.ViewCount), render.ColViews-1)+" ", render.Date(lv.UploadDate),
		}
	}
	return rows
}
