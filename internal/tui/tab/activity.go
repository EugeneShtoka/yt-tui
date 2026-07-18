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
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const actColType = 16

type actLoadedMsg struct{ entries []domain.ActivityEntry }

type Activity struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	width, height int

	entries []domain.ActivityEntry
	loaded  bool
	table   table.Model
	numBuf  string
}

func NewActivity(backend api.Backend, keys keymap.KeyMap, circular bool) Activity {
	return Activity{backend: backend, keys: keys, circular: circular, table: newTable()}
}

func (t Activity) ID() tuipkg.TabID          { return tuipkg.TabActivity }
func (t Activity) Title() string             { return "Activity" }
func (t Activity) ShortHelp() []key.Binding { return nil }
func (t Activity) InterceptsInput() bool     { return false }

func (t Activity) Init() tea.Cmd { return t.actLoadCmd() }

func (t Activity) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.table.SetColumns(t.actColumns())
		t.table.SetHeight(t.height - 2)
		t.table.SetRows(t.toActivityRows())
	case actLoadedMsg:
		t.entries = m.entries
		t.loaded = true
		t.table.SetRows(t.toActivityRows())
		t.table.SetCursor(0)
	case tea.KeyMsg:
		return t.actHandleKey(m)
	}
	return t, nil
}

func (t Activity) View() string {
	header := styles.SectionTitle.Render("Activity")
	if !t.loaded {
		return lipgloss.JoinVertical(lipgloss.Left, header, styles.Dim.PaddingLeft(1).Render("Loading…"))
	}
	if len(t.entries) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header, styles.Dim.PaddingLeft(1).Render("No activity yet."))
	}
	parts := []string{header, t.table.View()}
	if t.numBuf != "" {
		parts = append(parts, gotoLineView(t.numBuf))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (t Activity) actHandleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if checkGotoNum(&t.numBuf, msg) {
		return t, nil
	}
	numBuf := t.numBuf
	t.numBuf = ""

	keys := t.keys
	n := len(t.entries)

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
	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
		if t.table.Cursor() < n {
			return t, t.actNavigateCmd(t.entries[t.table.Cursor()])
		}
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

func (t Activity) actColumns() []table.Column {
	metaW := t.width - render.ColNum - colIndicator - actColType
	if metaW < 20 {
		metaW = 20
	}
	return []table.Column{
		{Title: "#", Width: render.ColNum},
		{Title: " ", Width: colIndicator},
		{Title: "Type", Width: actColType},
		{Title: "Detail", Width: metaW},
	}
}

func (t Activity) toActivityRows() []table.Row {
	rows := make([]table.Row, len(t.entries))
	for i := range t.entries {
		e := &t.entries[i]
		locality := "remote"
		if e.IsLocal {
			locality = "local"
		}
		var meta string
		switch e.Type {
		case "subscribe":
			meta = fmt.Sprintf("%s (%s)", e.ChannelName, locality)
		case "create_playlist":
			meta = fmt.Sprintf("%s (%s)", e.PlaylistName, locality)
		case "add_to_playlist":
			metaW := t.width - render.ColNum - colIndicator - actColType
			if metaW < 20 {
				metaW = 20
			}
			meta = fmt.Sprintf("%s → %s (%s)", render.Truncate(e.VideoTitle, metaW/2), e.PlaylistName, locality)
		default:
			meta = e.Type
		}
		rows[i] = table.Row{rowNum(i), "  ", styles.Warning.Render(e.Type), meta}
	}
	return rows
}
