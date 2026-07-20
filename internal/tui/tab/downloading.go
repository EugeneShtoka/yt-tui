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
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	etable "github.com/evertras/bubble-table/table"
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

const (
	colDlStatus    = 52
	colKeyDlNum    = "dlnum"
	colKeyDlInd    = "dlind"
	colKeyDlTitle  = "dltitle"
	colKeyDlCh     = "dlch"
	colKeyDlDur    = "dldur"
	colKeyDlStatus = "dlstatus"
)

type Downloading struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	height int

	items  []api.DownloadItem
	events <-chan api.Event
	nav    videotable.TableNav
	cols   []videotable.ColumnDef[api.DownloadItem]

	spinner spinner.Model
	loading bool
}

func downloadingColumns(durW int) []videotable.ColumnDef[api.DownloadItem] {
	return []videotable.ColumnDef[api.DownloadItem]{
		{
			Col:  etable.NewColumn(colKeyDlNum, ralign("#", render.ColNum), render.ColNum),
			Cell: func(item api.DownloadItem, i int) any { return fmt.Sprintf("%4d", i+1) },
		},
		{
			Col:  etable.NewColumn(colKeyDlInd, " ", colIndicator),
			Cell: func(item api.DownloadItem, _ int) any { return "  " },
		},
		{
			Col: etable.NewFlexColumn(colKeyDlTitle, "Title", 1),
			Cell: func(item api.DownloadItem, _ int) any {
				t := item.Title
				if item.AudioOnly {
					t += " [audio]"
				}
				return t
			},
		},
		{
			Col:  etable.NewColumn(colKeyDlCh, "Channel", render.ColChannel),
			Cell: func(item api.DownloadItem, _ int) any { return item.Channel },
		},
		{
			Col: etable.NewColumn(colKeyDlDur, ralign("Duration", durW+1), durW+1),
			Cell: func(item api.DownloadItem, _ int) any {
				return fmt.Sprintf("%*s ", durW, item.Duration)
			},
		},
		{
			Col:  etable.NewColumn(colKeyDlStatus, "Status", colDlStatus),
			Cell: func(item api.DownloadItem, _ int) any { return dlRenderStatus(item) },
		},
	}
}

func NewDownloading(backend api.Backend, keys keymap.KeyMap, circular bool) Downloading {
	cols := downloadingColumns(render.ColDuration)
	return Downloading{
		backend:  backend,
		keys:     keys,
		circular: circular,
		spinner:  spinner.New(),
		loading:  true,
		nav:      videotable.NewTableNav(videotable.NewTable(cols), circular, 2),
		cols:     cols,
	}
}

func (t Downloading) ID() tuipkg.TabID         { return tuipkg.TabDownloading }
func (t Downloading) Title() string            { return "Downloading" }
func (t Downloading) ShortHelp() []key.Binding { return []key.Binding{t.keys.Play, t.keys.Delete, t.keys.CopyURL} }
func (t Downloading) InterceptsInput() bool    { return false }

func (t Downloading) Init() tea.Cmd {
	return tea.Batch(t.fetchItemsCmd(), t.subscribeEventsCmd(), t.spinner.Tick)
}

func (t Downloading) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.height = m.Height
		t.nav.Resize(m.Width, m.Height)
		t.nav.SetRows(videotable.BuildRows(t.items, t.cols))
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
		t.nav.SetRows(videotable.BuildRows(t.items, t.cols))
		return t, nil

	case spinner.TickMsg:
		if t.loading {
			var cmd tea.Cmd
			t.spinner, cmd = t.spinner.Update(m)
			return t, cmd
		}
		return t, nil

	case tea.KeyPressMsg:
		return t.handleKey(m)
	}
	return t, nil
}

func (t Downloading) View() tea.View {
	header := styles.SectionTitle.Render("Downloading")
	if t.loading {
		return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, " "+t.spinner.View()+" Loading…"))
	}
	if len(t.items) == 0 {
		return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header,
			styles.Dim.PaddingLeft(1).Render("No active downloads. Press "+t.keys.Download.Help().Key+" on any video to start.")))
	}
	parts := []string{header, t.nav.View()}
	if s := t.nav.NumBufView(); s != "" {
		parts = append(parts, s)
	}
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

func (t Downloading) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if t.nav.HandleNav(msg, t.keys, len(t.items)) {
		return t, nil
	}

	keys := t.keys
	idx := t.nav.Index()

	switch {
	case key.Matches(msg, keys.Delete):
		if idx >= 0 && idx < len(t.items) {
			id := t.items[idx].VideoID
			return t, tea.Batch(
				func() tea.Msg {
					_ = t.backend.CancelDownload(context.Background(), id)
					return tuipkg.DownloadItemsChangedMsg{}
				},
				func() tea.Msg { return tuipkg.RefreshPositionsMsg{} },
			)
		}
	case key.Matches(msg, keys.Play):
		if idx >= 0 && idx < len(t.items) && t.items[idx].Status == api.DownloadComplete {
			item := t.items[idx]
			return t, func() tea.Msg {
				lv, found := t.backend.HasLocalVideo(context.Background(), item.VideoID)
				if !found {
					return tuipkg.StatusMsg{Text: "local file not found", IsErr: true}
				}
				return tuipkg.LaunchLocalVideoMsg{Video: lv}
			}
		}
	case key.Matches(msg, keys.CopyURL):
		if idx >= 0 && idx < len(t.items) {
			url := t.items[idx].URL
			return t, func() tea.Msg { return tuipkg.CopyURLMsg{URL: url} }
		}
	}
	return t, nil
}

func dlRenderStatus(item api.DownloadItem) string {
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
