package tab

import (
	"context"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
)

type actLoadedMsg struct {
	tuipkg.TabTarget
	entries []domain.ActivityEntry
}

// activityBackend is the narrow slice of the backend the Activity tab needs:
// just the activity log. Declared consumer-side so the tab depends only on what
// it uses (ISP), not the full api.Backend surface.
type activityBackend interface {
	api.HistoryBackend
}

type Activity struct {
	ctx      context.Context
	backend  activityBackend
	keys     keymap.KeyMap
	circular bool

	height int

	entries []domain.ActivityEntry
	loaded  bool
	nav     videotable.TableNav
	cols    []videotable.ColumnDef[domain.ActivityEntry]
}

// activityColumns is the full, natural-order column set for the Activity list.
// Extracted so the per-panel column selector and tab.PanelColumnKeys catalog
// share one source of truth.
func activityColumns() []videotable.ColumnDef[domain.ActivityEntry] {
	return []videotable.ColumnDef[domain.ActivityEntry]{
		videotable.NumCol[domain.ActivityEntry](),
		videotable.BlankIndicatorCol[domain.ActivityEntry](),
		videotable.StyledLabelCol[domain.ActivityEntry]("Type", videotable.ColActType, styles.Warning),
		videotable.ActDetailCol[domain.ActivityEntry](),
	}
}

func NewActivity(ctx context.Context, backend activityBackend, keys keymap.KeyMap, circular bool, wantCols ...string) Activity {
	cols := videotable.SelectColumns(activityColumns(), wantCols)
	return Activity{
		ctx:      ctx,
		backend:  backend,
		keys:     keys,
		circular: circular,
		nav:      videotable.NewTableNav(cols, circular, 2),
		cols:     cols,
	}
}

func (t Activity) ID() tuipkg.TabID         { return tuipkg.TabActivity }
func (t Activity) Title() string            { return "Activity" }
func (t Activity) ShortHelp() []key.Binding { return []key.Binding{t.keys.DrillDown, t.keys.Refresh} }
func (t Activity) InterceptsInput() bool    { return false }
func (t Activity) SelectedVideo() (domain.Video, bool) {
	return domain.Video{}, false
}
func (t Activity) Loading() bool { return false }

func (t Activity) Init() tea.Cmd { return t.actLoadCmd() }

func (t Activity) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tuipkg.ContentSizeMsg:
		t.height = m.Height
		t.nav.Resize(m.Width, m.Height)
		t.nav.SetRows(videotable.BuildRows(t.entries, t.cols))
	case actLoadedMsg:
		t.entries = m.entries
		t.loaded = true
		t.nav.SetRows(videotable.BuildRows(t.entries, t.cols))
		t.nav.GotoRow(0)
	case tea.KeyPressMsg:
		return t.actHandleKey(m)
	}
	return t, nil
}

func (t Activity) View() tea.View {
	header := styles.SectionTitle.Render("Activity")
	if !t.loaded {
		return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, styles.Dim.PaddingLeft(1).Render("Loading…")))
	}
	if len(t.entries) == 0 {
		return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, styles.Dim.PaddingLeft(1).Render("No activity yet.")))
	}
	parts := []string{header, t.nav.View()}
	if s := t.nav.NumBufView(); s != "" {
		parts = append(parts, s)
	}
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

func (t Activity) actHandleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if t.nav.HandleNav(msg, t.keys, len(t.entries)) {
		return t, nil
	}

	keys := t.keys
	idx := t.nav.Index()

	switch {
	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
		if idx < len(t.entries) {
			return t, t.actNavigateCmd(t.entries[idx])
		}
	case key.Matches(msg, keys.Refresh):
		return t, t.actLoadCmd()
	}
	return t, nil
}
