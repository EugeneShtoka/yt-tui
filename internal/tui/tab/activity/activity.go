package activity

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

type loadedMsg struct{ entries []domain.ActivityEntry }

// Tab is the Activity tab: a scrollable log of subscription and playlist actions.
type Tab struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	width, height int

	entries  []domain.ActivityEntry
	cursor   int
	vs       int
	loaded   bool
}

func New(backend api.Backend, keys keymap.KeyMap, circular bool) Tab {
	return Tab{backend: backend, keys: keys, circular: circular}
}

// ── tui.Tab interface ─────────────────────────────────────────────────────────

func (t Tab) Title() string              { return "Activity" }
func (t Tab) ShortHelp() []key.Binding  { return nil }
func (t Tab) Context() tuipkg.ContextID { return tuipkg.CtxVideoList }

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (t Tab) Init() tea.Cmd { return t.loadCmd() }

func (t Tab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
	case loadedMsg:
		t.entries = m.entries
		t.loaded = true
		if t.cursor >= len(t.entries) && t.cursor > 0 {
			t.cursor = len(t.entries) - 1
		}
	case tea.KeyMsg:
		return t.handleKey(m)
	}
	return t, nil
}

func (t Tab) View() string {
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

// ── key handling ──────────────────────────────────────────────────────────────

func (t Tab) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := len(t.entries)
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
	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
		if t.cursor < n {
			return t, t.navigateCmd(t.entries[t.cursor])
		}
	}
	return t, nil
}

// navigateCmd emits the appropriate cross-root navigation message for an entry.
func (t Tab) navigateCmd(e domain.ActivityEntry) tea.Cmd {
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

func (t Tab) pageHeight() int {
	h := t.height - 1 // section header
	if h < 1 {
		h = 1
	}
	return h
}

func (t Tab) loadCmd() tea.Cmd {
	return func() tea.Msg {
		entries, err := t.backend.ActivityLog(context.Background(), 200)
		if err != nil {
			return tuipkg.StatusMsg{Text: "activity: " + err.Error(), IsErr: true}
		}
		return loadedMsg{entries}
	}
}
