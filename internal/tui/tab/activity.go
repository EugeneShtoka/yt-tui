package tab

import (
	"context"
	"fmt"
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

type actLoadedMsg struct{ entries []domain.ActivityEntry }

// Activity is the Activity tab: a scrollable log of subscription and playlist actions.
type Activity struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	width, height int

	entries []domain.ActivityEntry
	cursor  int
	vs      int
	loaded  bool
}

func NewActivity(backend api.Backend, keys keymap.KeyMap, circular bool) Activity {
	return Activity{backend: backend, keys: keys, circular: circular}
}

func (t Activity) Title() string             { return "Activity" }
func (t Activity) ShortHelp() []key.Binding { return nil }
func (t Activity) Context() tuipkg.ContextID { return tuipkg.CtxVideoList }

func (t Activity) Init() tea.Cmd { return t.actLoadCmd() }

func (t Activity) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
	case actLoadedMsg:
		t.entries = m.entries
		t.loaded = true
		if t.cursor >= len(t.entries) && t.cursor > 0 {
			t.cursor = len(t.entries) - 1
		}
	case tea.KeyMsg:
		return t.actHandleKey(m)
	}
	return t, nil
}

func (t Activity) View() string {
	width, height := t.width, t.height
	header := styles.SectionTitle.Render("Activity")
	headerH := lipgloss.Height(header)

	if !t.loaded {
		return lipgloss.JoinVertical(lipgloss.Left, header, styles.Dim.Render("Loading…"))
	}
	if len(t.entries) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header, styles.Dim.Render("No activity yet."))
	}

	const colType = 16
	colMeta := width - render.ColNum - 1 - 2 - colType - 1
	if colMeta < 20 {
		colMeta = 20
	}

	start, end := nav.Window(t.vs, len(t.entries), height-headerH)
	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		e := t.entries[i]

		indicator := "  "
		sep := " "
		numStyle := styles.RowNum
		typeStyle := styles.Warning.Width(colType)
		if i == t.cursor {
			indicator = styles.Selected.Render("▶ ")
			numStyle = numStyle.Background(styles.ColorBgSelect)
			sep = lipgloss.NewStyle().Background(styles.ColorBgSelect).Render(" ")
			typeStyle = typeStyle.Background(styles.ColorBgSelect)
		}
		numStr := numStyle.Render(fmt.Sprintf("%*d", render.ColNum, i+1))

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
			meta = fmt.Sprintf("%s → %s (%s)", render.Truncate(e.VideoTitle, colMeta/2), e.PlaylistName, locality)
		default:
			meta = e.Type
		}

		metaStyle := styles.Normal.Width(colMeta)
		if i == t.cursor {
			metaStyle = styles.Selected.Width(colMeta)
		}
		rows = append(rows,
			numStr+sep+indicator+typeStyle.Render(e.Type)+sep+
				metaStyle.Render(render.Truncate(meta, colMeta)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(rows, "\n"))
}

func (t Activity) actHandleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := len(t.entries)
	pageH := t.actPageHeight()

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
	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
		if t.cursor < n {
			return t, t.actNavigateCmd(t.entries[t.cursor])
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

func (t Activity) actPageHeight() int {
	h := t.height - 1
	if h < 1 {
		h = 1
	}
	return h
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
