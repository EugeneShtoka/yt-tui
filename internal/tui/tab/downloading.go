package tab

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
)

type downloadingBackend interface {
	DownloadItems(ctx context.Context) ([]api.DownloadItem, error)
	Events(ctx context.Context) (<-chan api.Event, error)
	CancelDownload(ctx context.Context, videoID string) error
	HasLocalVideo(ctx context.Context, videoID string) (domain.LocalVideo, bool, error)
}

type dlItemsMsg struct {
	tuipkg.TabTarget
	items []api.DownloadItem
}
type dlEventsReadyMsg struct {
	tuipkg.TabTarget
	ch     <-chan api.Event
	cancel context.CancelFunc
}
type dlEventsClosedMsg struct{ retryDelay time.Duration }
type dlResubscribeMsg struct{ delay time.Duration }

// Theme-independent styles are captured once; the theme-derived colors
// (Warning/Success/Error) are read from styles.* at render time so they track a
// user theme reloaded after startup via styles.Init (a package-level capture
// would freeze the default-theme value).
var (
	dlStylePending = lipgloss.NewStyle().Faint(true)
	dlStyleEmpty   = lipgloss.NewStyle().Faint(true)
)

type Downloading struct {
	ctx      context.Context
	backend  downloadingBackend
	keys     keymap.KeyMap
	circular bool

	height int

	items  []api.DownloadItem
	events <-chan api.Event
	// cancelEvents cancels the context backing the current events
	// subscription. It must be called before starting a new subscription (on
	// resubscribe) so the old one unregisters from the downloader's
	// broadcaster and its goroutine exits instead of leaking (C-2).
	cancelEvents context.CancelFunc
	nav          videotable.TableNav
	cols         []videotable.ColumnDef[api.DownloadItem]

	spinnerFrame string
	loading      bool
}

// downloadingColumns is the full, natural-order column set for the Downloading
// queue list. Extracted so the per-panel column selector and the
// tab.PanelColumnKeys catalog share one source of truth.
func downloadingColumns() []videotable.ColumnDef[api.DownloadItem] {
	return []videotable.ColumnDef[api.DownloadItem]{
		videotable.NumCol[api.DownloadItem](),
		videotable.BlankIndicatorCol[api.DownloadItem](),
		videotable.AudioTitleFlexCol[api.DownloadItem](),
		videotable.ChannelCol[api.DownloadItem](),
		videotable.DlDurationCol[api.DownloadItem](),
		videotable.DlStatusCol[api.DownloadItem](func(item api.DownloadItem) any { return dlRenderStatus(item) }),
	}
}

func NewDownloading(ctx context.Context, backend downloadingBackend, keys keymap.KeyMap, circular bool, wantCols ...string) Downloading {
	cols := videotable.SelectColumns(downloadingColumns(), wantCols)
	return Downloading{
		ctx:      ctx,
		backend:  backend,
		keys:     keys,
		circular: circular,
		loading:  true,
		nav:      videotable.NewTableNav(cols, circular, 2),
		cols:     cols,
	}
}

func (t Downloading) ID() tuipkg.TabID { return tuipkg.TabDownloading }
func (t Downloading) Title() string    { return "Downloading" }
func (t Downloading) ShortHelp() []key.Binding {
	return []key.Binding{t.keys.Play, t.keys.Delete, t.keys.CopyURL}
}
func (t Downloading) InterceptsInput() bool { return false }
func (t Downloading) SelectedVideo() (domain.Video, bool) {
	return domain.Video{}, false
}
func (t Downloading) Loading() bool { return t.loading }

func (t Downloading) Init() tea.Cmd {
	return tea.Batch(t.fetchItemsCmd(), t.subscribeEventsCmd())
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
		if t.cancelEvents != nil {
			t.cancelEvents()
		}
		t.events = m.ch
		t.cancelEvents = m.cancel
		return t, t.waitEventCmd()

	case dlEventsClosedMsg:
		if t.cancelEvents != nil {
			t.cancelEvents()
			t.cancelEvents = nil
		}
		t.events = nil
		// Wait out the backoff via tea.Tick (off the bounded worker pool) instead
		// of sleeping inside a command, then attempt to resubscribe.
		delay := m.retryDelay
		return t, tea.Tick(delay, func(_ time.Time) tea.Msg {
			return dlResubscribeMsg{delay: delay}
		})

	case dlResubscribeMsg:
		return t, t.resubscribeCmd(m.delay)

	case api.Event:
		return t, tea.Batch(t.fetchItemsCmd(), t.waitEventCmd())

	case dlItemsMsg:
		t.loading = false
		t.items = m.items
		t.nav.SetRows(videotable.BuildRows(t.items, t.cols))
		return t, nil

	case tuipkg.SpinnerFrameMsg:
		t.spinnerFrame = m.Frame

	case tea.KeyPressMsg:
		return t.handleKey(m)
	}
	return t, nil
}

func (t Downloading) View() tea.View {
	header := styles.SectionTitle.Render("Downloading")
	if t.loading {
		return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, " "+t.spinnerFrame+" Loading…"))
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
					_ = t.backend.CancelDownload(t.ctx, id)
					return tuipkg.DownloadItemsChangedMsg{}
				},
				func() tea.Msg { return tuipkg.RefreshPositionsMsg{} },
			)
		}
	case key.Matches(msg, keys.Play):
		if idx >= 0 && idx < len(t.items) && t.items[idx].Status == api.DownloadComplete {
			item := t.items[idx]
			return t, func() tea.Msg {
				lv, found, err := t.backend.HasLocalVideo(t.ctx, item.VideoID)
				if err != nil {
					return tuipkg.StatusMsg{Text: "local file lookup failed: " + err.Error(), IsErr: true}
				}
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
			bar, item.Progress, styles.Warning.Render(item.Speed), item.ETA)
	case api.DownloadComplete:
		return styles.Success.Render("done ✓")
	default:
		msg := "failed"
		if item.Err != nil {
			msg = "failed: " + render.Truncate(item.Err.Error(), 30)
		}
		return styles.Error.Render(msg)
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
	return "[" + styles.Success.Render(strings.Repeat("█", filled)) +
		dlStyleEmpty.Render(strings.Repeat("░", width-filled)) + "]"
}
