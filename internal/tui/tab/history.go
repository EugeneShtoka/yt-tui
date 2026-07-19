package tab

import (
	"context"
	"os"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const histColStatus = 14

type histLoadedMsg struct{ entries []domain.HistoryEntry }
type histDetailLoadedMsg struct {
	videoID string
	entries []domain.HistoryEntry
}
type histDeletedMsg struct{ title string }

type History struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	width, height int

	entries       []domain.HistoryEntry
	loaded        bool
	detailVideoID string
	detail        []domain.HistoryEntry

	table       table.Model
	detailTable table.Model
	numBuf      string

	sortMode        int
	sortChordActive bool
	gotoTopActive   bool
}

func NewHistory(backend api.Backend, keys keymap.KeyMap, circular bool) History {
	return History{
		backend:     backend,
		keys:        keys,
		circular:    circular,
		table:       newTable(),
		detailTable: newTable(),
	}
}

func (t History) ID() tuipkg.TabID         { return tuipkg.TabHistory }
func (t History) Title() string            { return "History" }
func (t History) InterceptsInput() bool    { return false }
func (t History) ShortHelp() []key.Binding {
	return []key.Binding{t.keys.Play, t.keys.DrillDown, t.keys.Delete, t.keys.CopyURL, t.keys.SortChord}
}

func (t History) Init() tea.Cmd { return t.loadCmd() }

func (t History) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.table.SetColumns(t.histColumns())
		t.table.SetHeight(t.height - 2)
		t.table.SetRows(t.toHistRows())
		t.detailTable.SetColumns(histDetailColumns(t.width))
		t.detailTable.SetHeight(t.height - 2)
	case tuipkg.HistoryChangedMsg:
		return t, t.loadCmd()
	case histLoadedMsg:
		t.entries = m.entries
		feed.SortHistoryEntries(t.entries, t.sortMode)
		t.loaded = true
		t.table.SetRows(t.toHistRows())
		t.table.SetCursor(0)
		t.detailVideoID = ""
	case histDetailLoadedMsg:
		t.detailVideoID = m.videoID
		t.detail = m.entries
		t.detailTable.SetRows(toDetailRows(t.detail))
		t.detailTable.GotoTop()
	case histDeletedMsg:
		return t, func() tea.Msg { return tuipkg.StatusMsg{Text: "Deleted: " + render.Truncate(m.title, 50)} }
	case tea.KeyPressMsg:
		return t.handleKey(m)
	}
	return t, nil
}

func (t History) View() tea.View {
	if t.detailVideoID != "" {
		return tea.NewView(t.renderDetail())
	}
	return tea.NewView(t.renderList())
}

func (t History) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys

	if t.sortChordActive && t.detailVideoID == "" {
		t.sortChordActive = false
		sk := t.keys.Sort
		switch {
		case key.Matches(msg, sk.Date):
			t.sortMode = feed.SortDate
		case key.Matches(msg, sk.Views):
			t.sortMode = feed.SortViews
		case key.Matches(msg, sk.Name):
			t.sortMode = feed.SortName
		case key.Matches(msg, sk.Channel):
			t.sortMode = feed.SortChannel
		case key.Matches(msg, sk.Duration):
			t.sortMode = feed.SortDuration
		}
		feed.SortHistoryEntries(t.entries, t.sortMode)
		t.table.SetRows(t.toHistRows())
		return t, nil
	}

	if consumed, doTop := handleGotoPrefix(&t.gotoTopActive, t.keys, msg); consumed {
		if doTop {
			t.numBuf = ""
			if t.detailVideoID != "" {
				t.detailTable.GotoTop()
			} else {
				t.table.GotoTop()
			}
		}
		return t, nil
	}

	if checkGotoNum(&t.numBuf, msg) {
		return t, nil
	}
	numBuf := t.numBuf
	t.numBuf = ""

	// ── detail pane ───────────────────────────────────────────────────────────
	if t.detailVideoID != "" {
		switch {
		case key.Matches(msg, keys.Escape), key.Matches(msg, keys.Left):
			if numBuf != "" {
				return t, nil // escape just cleared numBuf
			}
			t.detailVideoID = ""
			t.detail = nil
		case key.Matches(msg, keys.GotoLine):
			if numBuf != "" {
				applyGoto(numBuf, &t.detailTable)
			} else {
				t.detailTable.GotoBottom()
			}
		case key.Matches(msg, keys.Up):
			t.detailTable.MoveUp(1)
		case key.Matches(msg, keys.Down):
			t.detailTable.MoveDown(1)
		case key.Matches(msg, keys.PageUp):
			t.detailTable.MoveUp(t.detailTable.Height())
		case key.Matches(msg, keys.PageDown):
			t.detailTable.MoveDown(t.detailTable.Height())
		}
		return t, nil
	}

	// ── list pane ─────────────────────────────────────────────────────────────
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
	case key.Matches(msg, keys.Play):
		if t.table.Cursor() < n {
			e := t.entries[t.table.Cursor()]
			if e.EventType != "search" {
				v := domain.Video{
					ID:    e.VideoID,
					Title: e.Title,
					URL:   "https://www.youtube.com/watch?v=" + e.VideoID,
				}
				return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v} }
			}
		}
	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
		if t.table.Cursor() < n {
			return t, t.histLoadDetailCmd(t.entries[t.table.Cursor()].VideoID)
		}
	case key.Matches(msg, keys.Delete):
		if t.table.Cursor() < n {
			e := t.entries[t.table.Cursor()]
			t.entries = append(t.entries[:t.table.Cursor()], t.entries[t.table.Cursor()+1:]...)
			t.table.SetRows(t.toHistRows())
			return t, t.histDeleteCmd(e)
		}
	case key.Matches(msg, keys.HideChannel):
		if t.table.Cursor() < n {
			ch := domain.Channel{ID: t.entries[t.table.Cursor()].ChannelID, Name: t.entries[t.table.Cursor()].Channel}
			return t, func() tea.Msg { return tuipkg.HideChannelMsg{Channel: ch} }
		}
	case key.Matches(msg, keys.Refresh):
		t.loaded = false
		return t, t.loadCmd()
	case key.Matches(msg, keys.SortChord):
		t.sortChordActive = true
	case key.Matches(msg, keys.Escape):
		if numBuf != "" {
			return t, nil
		}
	}
	return t, nil
}

func (t History) loadCmd() tea.Cmd {
	return func() tea.Msg {
		entries, err := t.backend.HistoryVideos(context.Background(), 200)
		if err != nil {
			return tuipkg.StatusMsg{Text: "history: " + err.Error(), IsErr: true}
		}
		return histLoadedMsg{entries}
	}
}

func (t History) histLoadDetailCmd(videoID string) tea.Cmd {
	return func() tea.Msg {
		entries, err := t.backend.VideoHistory(context.Background(), videoID)
		if err != nil {
			return tuipkg.StatusMsg{Text: "history detail: " + err.Error(), IsErr: true}
		}
		return histDetailLoadedMsg{videoID: videoID, entries: entries}
	}
}

func (t History) histDeleteCmd(e domain.HistoryEntry) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if lv, ok := t.backend.HasLocalVideo(ctx, e.VideoID); ok {
			_ = os.Remove(lv.FilePath)
			_ = t.backend.DeleteLocalVideo(ctx, lv.ID)
		}
		_ = t.backend.DeleteVideoHistory(ctx, e.VideoID)
		_ = t.backend.DeleteVideoPosition(ctx, e.VideoID)
		return histDeletedMsg{title: e.Title}
	}
}

func (t History) renderList() string {
	header := styles.SectionTitle.Render("History")
	if !t.loaded {
		return lipgloss.JoinVertical(lipgloss.Left, header, styles.Dim.Render("Loading…"))
	}
	if len(t.entries) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header, styles.Dim.Render("No history yet."))
	}
	parts := []string{header, t.table.View()}
	if t.numBuf != "" {
		parts = append(parts, gotoLineView(t.numBuf))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (t History) renderDetail() string {
	title := ""
	if len(t.detail) > 0 {
		title = t.detail[0].Title
	}
	header := styles.SectionTitle.Render("← " + render.Truncate(title, t.width-4))
	parts := []string{header, t.detailTable.View()}
	if t.numBuf != "" {
		parts = append(parts, gotoLineView(t.numBuf))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (t History) histColumns() []table.Column {
	titleW := t.width - render.ColNum - colIndicator - histColStatus - render.ColChannel - render.ColDuration - render.ColViews - render.ColDate
	if titleW < 20 {
		titleW = 20
	}
	return []table.Column{
		{Title: ralign("#", render.ColNum), Width: render.ColNum},
		{Title: " ", Width: colIndicator},
		{Title: "Type", Width: histColStatus},
		{Title: "Title", Width: titleW},
		{Title: "Channel", Width: render.ColChannel},
		{Title: "Duration", Width: render.ColDuration},
		{Title: "Views", Width: render.ColViews},
		{Title: "Date", Width: render.ColDate},
	}
}

func (t History) toHistRows() []table.Row {
	rows := make([]table.Row, len(t.entries))
	for i := range t.entries {
		e := &t.entries[i]
		var ind string
		switch e.EventType {
		case "download video", "download audio":
			ind = " ● "
		default:
			ind = " ○ "
		}
		rows[i] = table.Row{
			rowNum(i),
			ind,
			swapReset(styles.Warning.Render(render.FormatEvent(e.EventType))),
			e.Title,
			e.Channel,
			ralign(render.Duration(e.Duration), render.ColDuration),
			ralign(render.Views(e.ViewCount), render.ColViews-1)+" ",
			render.Date(e.UploadDate),
		}
	}
	return rows
}

func histDetailColumns(width int) []table.Column {
	const colEvW = 14
	const colTsW = 19
	detailW := width - colEvW - colTsW
	if detailW < 20 {
		detailW = 20
	}
	return []table.Column{
		{Title: "Type", Width: colEvW},
		{Title: "Timestamp", Width: colTsW},
		{Title: "Details", Width: detailW},
	}
}

func toDetailRows(entries []domain.HistoryEntry) []table.Row {
	rows := make([]table.Row, len(entries))
	for i := range entries {
		e := &entries[i]
		rows[i] = table.Row{
			styles.Warning.Render(render.FormatEvent(e.EventType)),
			e.Timestamp.Format("2006-01-02 15:04:05"),
			strings.TrimSpace(e.Details),
		}
	}
	return rows
}
