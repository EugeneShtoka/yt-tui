package tab

import (
	"context"
	"fmt"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/nav"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── tab-private messages ──────────────────────────────────────────────────────

type dlItemsMsg struct{ items []api.DownloadItem }
type dlEventsReadyMsg struct{ ch <-chan api.Event }

// ── styles ────────────────────────────────────────────────────────────────────

var (
	dlStylePending  = lipgloss.NewStyle().Faint(true)
	dlStyleActive   = styles.Warning
	dlStyleComplete = lipgloss.NewStyle().Foreground(lipgloss.Color("#5fd75f"))
	dlStyleFailed   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f"))
	dlStyleFill     = lipgloss.NewStyle().Foreground(lipgloss.Color("#5fd75f"))
	dlStyleEmpty    = lipgloss.NewStyle().Faint(true)
)

// ── Downloading ───────────────────────────────────────────────────────────────

// Downloading is the Downloading tab: a live view of the download queue driven
// by the backend Events channel.
type Downloading struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	width, height int

	items   []api.DownloadItem
	cursor  int
	vs      int
	events  <-chan api.Event

	spinner spinner.Model
	loading bool
}

func NewDownloading(backend api.Backend, keys keymap.KeyMap, circular bool) Downloading {
	return Downloading{
		backend:  backend,
		keys:     keys,
		circular: circular,
		spinner:  spinner.New(),
		loading:  true,
	}
}

// ── tui.Tab interface ─────────────────────────────────────────────────────────

func (t Downloading) ID() tuipkg.TabID          { return tuipkg.TabDownloading }
func (t Downloading) Title() string             { return "Downloading" }
func (t Downloading) ShortHelp() []key.Binding { return nil }
func (t Downloading) InterceptsInput() bool { return false }

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (t Downloading) Init() tea.Cmd {
	return tea.Batch(t.fetchItemsCmd(), t.subscribeEventsCmd(), t.spinner.Tick)
}

func (t Downloading) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		return t, nil

	case tuipkg.DownloadItemsChangedMsg:
		return t, t.fetchItemsCmd()

	case dlEventsReadyMsg:
		t.events = m.ch
		return t, t.waitEventCmd()

	case api.Event:
		return t, tea.Batch(t.fetchItemsCmd(), t.waitEventCmd())

	case dlItemsMsg:
		t.loading = false
		prev := t.cursor
		t.items = m.items
		n := len(t.items)
		if prev >= n && n > 0 {
			t.cursor = n - 1
		}
		t.cursor, t.vs = nav.Move(t.cursor, t.vs, n, 0, t.pageHeight(), false)
		return t, nil

	case spinner.TickMsg:
		if t.loading {
			var cmd tea.Cmd
			t.spinner, cmd = t.spinner.Update(m)
			return t, cmd
		}
		return t, nil

	case tea.KeyMsg:
		return t.handleKey(m)
	}
	return t, nil
}

func (t Downloading) View() string {
	header := styles.SectionTitle.Render("Downloading")
	headerH := lipgloss.Height(header)

	if t.loading {
		return lipgloss.JoinVertical(lipgloss.Left, header, " "+t.spinner.View()+" Loading…")
	}
	if len(t.items) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header,
			styles.Dim.PaddingLeft(1).Render("No active downloads. Press "+t.keys.Download.Help().Key+" on any video to start."))
	}

	titleW := t.width - render.ColNum - 1 - render.ColChannel - render.ColDuration - 42 - 6
	if titleW < 20 {
		titleW = 20
	}
	colHeader := strings.Repeat(" ", render.ColNum) + " " + "  " +
		styles.Dim.Width(titleW).Render("Title") + " " +
		styles.Dim.Width(render.ColChannel).Render("Channel") + " " +
		styles.Dim.Width(render.ColDuration).Render("Duration") + " " +
		styles.Dim.Render("Status")

	start, end := nav.Window(t.vs, len(t.items), t.height-headerH-1)
	var rows []string
	rows = append(rows, colHeader)
	for i := start; i < end; i++ {
		rows = append(rows, t.renderRow(t.items[i], i == t.cursor, i+1, titleW))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(rows, "\n"))
}

// ── key handling ──────────────────────────────────────────────────────────────

func (t Downloading) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := len(t.items)
	pageH := t.pageHeight()

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

	case key.Matches(msg, keys.Delete):
		if item, ok := t.current(); ok {
			id := item.VideoID
			return t, func() tea.Msg {
				_ = t.backend.CancelDownload(context.Background(), id)
				return tuipkg.DownloadItemsChangedMsg{}
			}
		}

	case key.Matches(msg, keys.Play):
		if item, ok := t.current(); ok && item.Status == api.DownloadComplete {
			return t, func() tea.Msg {
				lv, found := t.backend.HasLocalVideo(context.Background(), item.VideoID)
				if !found {
					return tuipkg.StatusMsg{Text: "local file not found", IsErr: true}
				}
				return tuipkg.LaunchLocalVideoMsg{Video: lv}
			}
		}

	case key.Matches(msg, keys.CopyURL):
		if item, ok := t.current(); ok {
			url := item.URL
			return t, func() tea.Msg { return tuipkg.CopyURLMsg{URL: url} }
		}
	}
	return t, nil
}

// ── rendering ─────────────────────────────────────────────────────────────────

func (t Downloading) renderRow(item api.DownloadItem, selected bool, num, titleW int) string {
	numStyle := styles.RowNum
	nameStyle := styles.Normal.Width(titleW)
	indicator := "  "
	if selected {
		indicator = styles.Selected.Render("▶ ")
		numStyle = numStyle.Background(styles.ColorBgSelect)
		nameStyle = styles.Selected.Width(titleW)
	}

	titleSuffix := ""
	if item.AudioOnly {
		titleSuffix = " [audio]"
	}
	title := render.Truncate(item.Title+titleSuffix, titleW)
	channel := render.Truncate(item.Channel, render.ColChannel-2)
	dur := item.Duration

	statusPart := t.renderStatus(item)

	numStr := numStyle.Render(fmt.Sprintf("%*d", render.ColNum, num))
	sep := " "
	if selected {
		sep = lipgloss.NewStyle().Background(styles.ColorBgSelect).Render(" ")
	}

	return numStr + sep + indicator + nameStyle.Render(title) + " " +
		styles.Dim.Width(render.ColChannel).Render(channel) + " " +
		styles.Dim.Width(render.ColDuration).Render(dur) + " " +
		statusPart
}

func (t Downloading) renderStatus(item api.DownloadItem) string {
	switch item.Status {
	case api.DownloadPending:
		return dlStylePending.Render("pending")
	case api.DownloadActive:
		bar := dlProgressBar(item.Progress, 20)
		return fmt.Sprintf("%s %5.1f%%  %s  ETA %s",
			bar, item.Progress, dlStyleActive.Render(item.Speed), item.ETA)
	case api.DownloadComplete:
		return dlStyleComplete.Render("done ✓")
	default:
		msg := "failed"
		if item.Err != nil {
			msg = "failed: " + render.Truncate(item.Err.Error(), 30)
		}
		return dlStyleFailed.Render(msg)
	}
}

func dlProgressBar(pct float64, width int) string {
	if width <= 0 {
		return ""
	}
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	bar := dlStyleFill.Render(strings.Repeat("█", filled))
	bar += dlStyleEmpty.Render(strings.Repeat("░", width-filled))
	return "[" + bar + "]"
}

// ── background commands ───────────────────────────────────────────────────────

func (t Downloading) fetchItemsCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := t.backend.DownloadItems(context.Background())
		if err != nil {
			return tuipkg.StatusMsg{Text: "download queue: " + err.Error(), IsErr: true}
		}
		return dlItemsMsg{items: items}
	}
}

func (t Downloading) subscribeEventsCmd() tea.Cmd {
	return func() tea.Msg {
		ch, err := t.backend.Events(context.Background())
		if err != nil {
			return tuipkg.StatusMsg{Text: "events: " + err.Error(), IsErr: true}
		}
		return dlEventsReadyMsg{ch: ch}
	}
}

func (t Downloading) waitEventCmd() tea.Cmd {
	if t.events == nil {
		return nil
	}
	ch := t.events
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return ev
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (t Downloading) current() (api.DownloadItem, bool) {
	if t.cursor >= 0 && t.cursor < len(t.items) {
		return t.items[t.cursor], true
	}
	return api.DownloadItem{}, false
}

func (t Downloading) pageHeight() int {
	h := t.height - 3
	if h < 1 {
		h = 1
	}
	return h
}
