package tab

import (
	"context"
	"fmt"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
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

const (
	actColType       = 16
	colKeyActNum     = "actnum"
	colKeyActInd     = "actind"
	colKeyActType    = "acttype"
	colKeyActDetail  = "actdetail"
)

type actLoadedMsg struct{ entries []domain.ActivityEntry }

type Activity struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	height int

	entries []domain.ActivityEntry
	loaded  bool
	nav     videotable.TableNav
	cols    []videotable.ColumnDef[domain.ActivityEntry]
}

func activityColumns() []videotable.ColumnDef[domain.ActivityEntry] {
	return []videotable.ColumnDef[domain.ActivityEntry]{
		{
			Col:  etable.NewColumn(colKeyActNum, ralign("#", render.ColNum), render.ColNum),
			Cell: func(e domain.ActivityEntry, i int) any { return fmt.Sprintf("%4d", i+1) },
		},
		{
			Col:  etable.NewColumn(colKeyActInd, " ", colIndicator),
			Cell: func(e domain.ActivityEntry, _ int) any { return "  " },
		},
		{
			Col: etable.NewColumn(colKeyActType, "Type", actColType),
			Cell: func(e domain.ActivityEntry, _ int) any {
				return etable.NewStyledCell(e.Type, styles.Warning)
			},
		},
		{
			Col: etable.NewFlexColumn(colKeyActDetail, "Detail", 1),
			Cell: func(e domain.ActivityEntry, _ int) any {
				locality := "remote"
				if e.IsLocal {
					locality = "local"
				}
				switch e.Type {
				case "subscribe":
					return fmt.Sprintf("%s (%s)", e.ChannelName, locality)
				case "create_playlist":
					return fmt.Sprintf("%s (%s)", e.PlaylistName, locality)
				case "add_to_playlist":
					return fmt.Sprintf("%s → %s (%s)", e.VideoTitle, e.PlaylistName, locality)
				default:
					return e.Type
				}
			},
		},
	}
}

func NewActivity(backend api.Backend, keys keymap.KeyMap, circular bool) Activity {
	cols := activityColumns()
	return Activity{
		backend:  backend,
		keys:     keys,
		circular: circular,
		nav:      videotable.NewTableNav(videotable.NewTable(cols), circular, 2),
		cols:     cols,
	}
}

func (t Activity) ID() tuipkg.TabID         { return tuipkg.TabActivity }
func (t Activity) Title() string            { return "Activity" }
func (t Activity) ShortHelp() []key.Binding { return []key.Binding{t.keys.DrillDown, t.keys.Refresh} }
func (t Activity) InterceptsInput() bool    { return false }

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

func (t Activity) actNavigateCmd(e domain.ActivityEntry) tea.Cmd {
	switch e.Type {
	case "subscribe":
		return func() tea.Msg {
			return tuipkg.NavigateToChannelMsg{ChannelID: e.ChannelID, ChannelName: e.ChannelName}
		}
	case "create_playlist", "add_to_playlist":
		return func() tea.Msg {
			return tuipkg.NavigateToPlaylistMsg{
				PlaylistID:      e.PlaylistID,
				PlaylistLocalID: e.PlaylistLocalID,
				PlaylistName:    e.PlaylistName,
			}
		}
	}
	return nil
}

func (t Activity) actLoadCmd() tea.Cmd {
	return func() tea.Msg {
		entries, err := t.backend.ActivityLog(context.Background(), 200)
		if err != nil {
			return tuipkg.StatusMsg{Text: "activity: " + err.Error(), IsErr: true}
		}
		return actLoadedMsg{entries}
	}
}
