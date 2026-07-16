package tab

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/nav"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type localLoadedMsg struct {
	videos []domain.LocalVideo
	status string // non-empty → emit StatusMsg after applying
}

// Local is the Local Library tab: downloaded videos stored on disk.
type Local struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	width, height int

	videos    []domain.LocalVideo
	cursor    int
	vs        int
	loaded    bool
}

func NewLocal(backend api.Backend, keys keymap.KeyMap, circular bool) Local {
	return Local{backend: backend, keys: keys, circular: circular}
}

func (t Local) ID() tuipkg.TabID          { return tuipkg.TabLocal }
func (t Local) Title() string             { return "Local" }
func (t Local) ShortHelp() []key.Binding { return nil }
func (t Local) InterceptsInput() bool { return false }

func (t Local) Init() tea.Cmd { return t.localLoadCmd("") }

func (t Local) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
	case localLoadedMsg:
		t.videos = m.videos
		t.loaded = true
		// clamp cursor after reload
		if t.cursor >= len(t.videos) && t.cursor > 0 {
			t.cursor = len(t.videos) - 1
		}
		if m.status != "" {
			return t, func() tea.Msg { return tuipkg.StatusMsg{Text: m.status} }
		}
	case tea.KeyMsg:
		return t.localHandleKey(m)
	}
	return t, nil
}

func (t Local) View() string {
	width, height := t.width, t.height
	header := styles.SectionTitle.Render("Local Library")
	headerH := lipgloss.Height(header)

	if !t.loaded {
		return lipgloss.JoinVertical(lipgloss.Left, header, styles.Dim.Render("Loading…"))
	}
	if len(t.videos) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header,
			styles.Dim.Render("No local videos. Download some with d."))
	}

	titleW := width - render.ColNum - 1 - 2 - render.ColChannel - 1 - render.ColDuration - 1 - render.ColViews - 1 - render.ColDate
	if titleW < 20 {
		titleW = 20
	}

	colHdr := strings.Repeat(" ", render.ColNum) + " " + "  " +
		styles.ColHeader.Width(titleW).Render("Title") + " " +
		styles.ColHeader.Width(render.ColChannel).Render("Channel") + " " +
		styles.ColHeader.Width(render.ColDuration).Render("Duration") + " " +
		styles.ColHeader.Width(render.ColViews).Render("Views") + " " +
		styles.ColHeader.Width(render.ColDate).Render("Date")

	listH := height - headerH - 1
	start, end := nav.Window(t.vs, len(t.videos), listH)

	rows := make([]string, 0, end-start+1)
	rows = append(rows, colHdr)
	for i := start; i < end; i++ {
		rows = append(rows, renderLocalRow(t.videos[i], i == t.cursor, i+1, titleW))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(rows, "\n"))
}

func renderLocalRow(lv domain.LocalVideo, selected bool, num, titleW int) string {
	dur := render.Duration(lv.Duration)
	if lv.Status == domain.StatusStarted && lv.LastPositionMs > 0 {
		dur = render.DurationWithPos(lv.LastPositionMs, lv.Duration)
	}
	dlType := ""
	if lv.DownloadType == "audio" {
		dlType = " ♪"
	}

	indicator := "  "
	sep := " "
	numStyle := styles.RowNum
	chStyle := styles.Channel.Width(render.ColChannel)
	durStyle := styles.Duration.Width(render.ColDuration)
	viewsStyle := styles.Duration.Width(render.ColViews)
	dateStyle := styles.Channel.Width(render.ColDate)
	var ts lipgloss.Style

	switch {
	case selected:
		indicator = styles.Selected.Render("▶ ")
		ts = styles.Selected.Width(titleW)
		numStyle = numStyle.Background(styles.ColorBgSelect)
		sep = lipgloss.NewStyle().Background(styles.ColorBgSelect).Render(" ")
		chStyle = chStyle.Background(styles.ColorBgSelect)
		durStyle = durStyle.Background(styles.ColorBgSelect)
		viewsStyle = viewsStyle.Background(styles.ColorBgSelect)
		dateStyle = dateStyle.Background(styles.ColorBgSelect)
	case lv.Status == domain.StatusNew:
		ts = styles.Bold.Width(titleW)
		indicator = styles.Success.Render("● ")
	case lv.Status == domain.StatusStarted:
		ts = styles.Normal.Width(titleW)
		indicator = styles.Dim.Render("○ ")
	case lv.Status == domain.StatusWatched:
		ts = styles.Dim.Width(titleW)
	default:
		ts = styles.Normal.Width(titleW)
	}

	numStr := numStyle.Render(fmt.Sprintf("%*d", render.ColNum, num))
	return numStr + sep + indicator +
		ts.Render(render.Truncate(lv.Title, titleW)+dlType) + sep +
		chStyle.Render(render.Truncate(lv.Channel, render.ColChannel-2)) + sep +
		durStyle.Render(dur) + sep +
		viewsStyle.Render(render.Views(lv.ViewCount)) + sep +
		dateStyle.Render(render.Date(lv.UploadDate))
}

func (t Local) localHandleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := len(t.videos)
	pageH := t.localPageHeight()

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
	case key.Matches(msg, keys.Play):
		if t.cursor < n {
			lv := t.videos[t.cursor]
			return t, func() tea.Msg { return tuipkg.LaunchLocalVideoMsg{Video: lv} }
		}
	case key.Matches(msg, keys.Delete):
		if t.cursor < n {
			lv := t.videos[t.cursor]
			return t, t.localDeleteCmd(lv)
		}
	case key.Matches(msg, keys.CopyURL):
		if t.cursor < n {
			lv := t.videos[t.cursor]
			url := "https://www.youtube.com/watch?v=" + lv.ID
			return t, func() tea.Msg { return tuipkg.CopyURLMsg{URL: url} }
		}
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

func (t Local) localPageHeight() int {
	h := t.height - 2
	if h < 1 {
		h = 1
	}
	return h
}
