package tab

import (
	"context"
	"fmt"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type dlItemsMsg struct{ items []api.DownloadItem }
type dlEventsReadyMsg struct{ ch <-chan api.Event }

var (
	dlStylePending  = lipgloss.NewStyle().Faint(true)
	dlStyleActive   = styles.Warning
	dlStyleComplete = lipgloss.NewStyle().Foreground(lipgloss.Color("#5fd75f"))
	dlStyleFailed   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f"))
	dlStyleFill     = lipgloss.NewStyle().Foreground(lipgloss.Color("#5fd75f"))
	dlStyleEmpty    = lipgloss.NewStyle().Faint(true)
)

const colDlStatus = 52

type Downloading struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	width, height int

	items  []api.DownloadItem
	events <-chan api.Event
	table  table.Model
	numBuf string

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
		table:    newTable(),
	}
}

func (t Downloading) ID() tuipkg.TabID          { return tuipkg.TabDownloading }
func (t Downloading) Title() string             { return "Downloading" }
func (t Downloading) ShortHelp() []key.Binding { return nil }
func (t Downloading) InterceptsInput() bool     { return false }

func (t Downloading) Init() tea.Cmd {
	return tea.Batch(t.fetchItemsCmd(), t.subscribeEventsCmd(), t.spinner.Tick)
}

func (t Downloading) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.table.SetColumns(t.dlColumns())
		t.table.SetHeight(t.height - 2)
		t.table.SetRows(t.toDownloadRows())
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
		t.items = m.items
		t.table.SetRows(t.toDownloadRows())
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
	if t.loading {
		return lipgloss.JoinVertical(lipgloss.Left, header, " "+t.spinner.View()+" Loading…")
	}
	if len(t.items) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header,
			styles.Dim.PaddingLeft(1).Render("No active downloads. Press "+t.keys.Download.Help().Key+" on any video to start."))
	}
	parts := []string{header, t.table.View()}
	if t.numBuf != "" {
		parts = append(parts, gotoLineView(t.numBuf))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (t Downloading) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if checkGotoNum(&t.numBuf, msg) {
		return t, nil
	}
	numBuf := t.numBuf
	t.numBuf = ""

	keys := t.keys
	n := len(t.items)

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
	return "[" + dlStyleFill.Render(strings.Repeat("█", filled)) +
		dlStyleEmpty.Render(strings.Repeat("░", width-filled)) + "]"
}

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

func (t Downloading) current() (api.DownloadItem, bool) {
	c := t.table.Cursor()
	if c >= 0 && c < len(t.items) {
		return t.items[c], true
	}
	return api.DownloadItem{}, false
}

func (t Downloading) dlColumns() []table.Column {
	titleW := t.width - render.ColNum - colIndicator - render.ColChannel - render.ColDuration - colDlStatus
	if titleW < 20 {
		titleW = 20
	}
	return []table.Column{
		{Title: "#", Width: render.ColNum},
		{Title: " ", Width: colIndicator},
		{Title: "Title", Width: titleW},
		{Title: "Channel", Width: render.ColChannel},
		{Title: "Duration", Width: render.ColDuration},
		{Title: "Status", Width: colDlStatus},
	}
}

func (t Downloading) toDownloadRows() []table.Row {
	rows := make([]table.Row, len(t.items))
	for i := range t.items {
		item := &t.items[i]
		title := item.Title
		if item.AudioOnly {
			title += " [audio]"
		}
		rows[i] = table.Row{rowNum(i), "  ", title, item.Channel, ralign(item.Duration, render.ColDuration), t.renderStatus(*item)}
	}
	return rows
}
