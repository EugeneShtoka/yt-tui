package tab

import (
	"context"
	"fmt"
	"os"
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

type histLoadedMsg struct{ entries []domain.HistoryEntry }
type histDetailLoadedMsg struct {
	videoID string
	entries []domain.HistoryEntry
}
type histDeletedMsg struct {
	title    string
	query    string
	isSearch bool
}

// History is the History tab: a scrollable list of recently watched videos and searches.
type History struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	width, height int

	entries       []domain.HistoryEntry
	cursor, vs    int
	loaded        bool
	detailVideoID string
	detail        []domain.HistoryEntry
}

func NewHistory(backend api.Backend, keys keymap.KeyMap, circular bool) History {
	return History{backend: backend, keys: keys, circular: circular}
}

func (t History) ID() tuipkg.TabID        { return tuipkg.TabHistory }
func (t History) Title() string            { return "History" }
func (t History) ShortHelp() []key.Binding { return nil }
func (t History) Context() tuipkg.ContextID {
	if t.detailVideoID != "" {
		return tuipkg.CtxHistoryVideo
	}
	if t.cursor < len(t.entries) && t.entries[t.cursor].EventType == "search" {
		return tuipkg.CtxHistorySearch
	}
	return tuipkg.CtxHistoryVideo
}

func (t History) Init() tea.Cmd { return t.loadCmd() }

func (t History) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
	case histLoadedMsg:
		t.entries = m.entries
		t.loaded = true
		t.cursor, t.vs = 0, 0
		t.detailVideoID = ""
	case histDetailLoadedMsg:
		t.detailVideoID = m.videoID
		t.detail = m.entries
	case histDeletedMsg:
		var text string
		if m.isSearch {
			text = "Removed search: " + render.Truncate(m.query, 50)
		} else {
			text = "Deleted: " + render.Truncate(m.title, 50)
		}
		return t, func() tea.Msg { return tuipkg.StatusMsg{Text: text} }
	case tea.KeyMsg:
		return t.handleKey(m)
	}
	return t, nil
}

func (t History) View() string {
	if t.detailVideoID != "" {
		return t.renderDetail()
	}
	return t.renderList()
}

func (t History) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := t.keys

	if t.detailVideoID != "" {
		switch {
		case key.Matches(msg, keys.Escape), key.Matches(msg, keys.Left):
			t.detailVideoID = ""
			t.detail = nil
		}
		return t, nil
	}

	n := len(t.entries)
	pageH := t.histPageHeight()

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
	case key.Matches(msg, keys.Play):
		if t.cursor < n {
			e := t.entries[t.cursor]
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
		if t.cursor < n {
			e := t.entries[t.cursor]
			if e.EventType == "search" {
				return t, func() tea.Msg {
					return tuipkg.NavigateMsg{Tab: tuipkg.TabSearch, Query: e.Details}
				}
			}
			return t, t.histLoadDetailCmd(e.VideoID)
		}
	case key.Matches(msg, keys.Delete):
		if t.cursor < n {
			e := t.entries[t.cursor]
			t.entries = append(t.entries[:t.cursor], t.entries[t.cursor+1:]...)
			if t.cursor >= len(t.entries) && t.cursor > 0 {
				t.cursor--
			}
			return t, t.histDeleteCmd(e)
		}
	case key.Matches(msg, keys.HideChannel):
		if t.cursor < n {
			e := t.entries[t.cursor]
			if e.EventType != "search" {
				ch := domain.Channel{ID: e.ChannelID, Name: e.Channel}
				return t, func() tea.Msg { return tuipkg.HideChannelMsg{Channel: ch} }
			}
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
		if e.EventType == "search" {
			_ = t.backend.DeleteSearchHistory(ctx, e.Details)
			return histDeletedMsg{query: e.Details, isSearch: true}
		}
		if lv, ok := t.backend.HasLocalVideo(ctx, e.VideoID); ok {
			_ = os.Remove(lv.FilePath)
			_ = t.backend.DeleteLocalVideo(ctx, lv.ID)
		}
		_ = t.backend.DeleteVideoHistory(ctx, e.VideoID)
		_ = t.backend.DeleteVideoPosition(ctx, e.VideoID)
		return histDeletedMsg{title: e.Title}
	}
}

func (t History) histPageHeight() int {
	h := t.height - 2
	if h < 1 {
		h = 1
	}
	return h
}

func (t History) renderList() string {
	width, height := t.width, t.height

	header := styles.SectionTitle.Render("History")
	headerH := lipgloss.Height(header)

	if !t.loaded {
		return lipgloss.JoinVertical(lipgloss.Left, header, styles.Dim.Render("Loading…"))
	}
	if len(t.entries) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header, styles.Dim.Render("No history yet."))
	}

	const colStatus = 14
	titleW := width - render.ColNum - 1 - 2 - colStatus - 1 - render.ColChannel - 1 - render.ColDuration - 1 - render.ColViews - 1 - render.ColDate
	if titleW < 20 {
		titleW = 20
	}

	colHdr := strings.Repeat(" ", render.ColNum) + " " + "  " +
		styles.ColHeader.Width(colStatus).Render("Type") + " " +
		styles.ColHeader.Width(titleW).Render("Title") + " " +
		styles.ColHeader.Width(render.ColChannel).Render("Channel") + " " +
		styles.ColHeader.Width(render.ColDuration).Render("Duration") + " " +
		styles.ColHeader.Width(render.ColViews).Render("Views") + " " +
		styles.ColHeader.Width(render.ColDate).Render("Date")

	listH := height - headerH - 1
	start, end := nav.Window(t.vs, len(t.entries), listH)

	rows := make([]string, 0, end-start+1)
	rows = append(rows, colHdr)

	for i := start; i < end; i++ {
		e := t.entries[i]

		indicator := "  "
		sep := " "
		numStyle := styles.RowNum
		statusStyle := styles.Warning.Width(colStatus)

		if i == t.cursor {
			indicator = styles.Selected.Render("▶ ")
			numStyle = numStyle.Background(styles.ColorBgSelect)
			sep = lipgloss.NewStyle().Background(styles.ColorBgSelect).Render(" ")
			statusStyle = statusStyle.Background(styles.ColorBgSelect)
		}
		numStr := numStyle.Render(fmt.Sprintf("%*d", render.ColNum, i+1))

		if e.EventType == "search" {
			queryW := width - render.ColNum - 1 - 2 - colStatus - 1
			queryStyle := styles.Channel.Width(queryW)
			if i == t.cursor {
				queryStyle = styles.Selected.Width(queryW)
			}
			rows = append(rows,
				numStr+sep+indicator+statusStyle.Render("search")+sep+
					queryStyle.Render(render.Truncate(e.Details, queryW)))
			continue
		}

		titleStyle := styles.Normal.Width(titleW)
		chStyle := styles.Channel.Width(render.ColChannel)
		durStyle := styles.Duration.Width(render.ColDuration)
		viewsStyle := styles.Duration.Width(render.ColViews)
		dateStyle := styles.Channel.Width(render.ColDate)

		if i == t.cursor {
			titleStyle = styles.Selected.Width(titleW)
			chStyle = chStyle.Background(styles.ColorBgSelect)
			durStyle = durStyle.Background(styles.ColorBgSelect)
			viewsStyle = viewsStyle.Background(styles.ColorBgSelect)
			dateStyle = dateStyle.Background(styles.ColorBgSelect)
		}

		rows = append(rows,
			numStr+sep+indicator+statusStyle.Render(e.EventType)+sep+
				titleStyle.Render(render.Truncate(e.Title, titleW))+sep+
				chStyle.Render(render.Truncate(e.Channel, render.ColChannel-2))+sep+
				durStyle.Render(render.Duration(e.Duration))+sep+
				viewsStyle.Render(render.Views(e.ViewCount))+sep+
				dateStyle.Render(render.Date(e.UploadDate)))
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(rows, "\n"))
}

func (t History) renderDetail() string {
	width, height := t.width, t.height

	title := ""
	if len(t.detail) > 0 {
		title = t.detail[0].Title
	}
	header := styles.SectionTitle.Render("← " + render.Truncate(title, width-4))
	headerH := lipgloss.Height(header)

	const colEvW = 14
	const colTsW = 19
	var rows []string
	for i, e := range t.detail {
		if i >= height-headerH {
			break
		}
		rows = append(rows, "  "+
			styles.Warning.Width(colEvW).Render(e.EventType)+" "+
			styles.Channel.Width(colTsW).Render(e.Timestamp.Format("2006-01-02 15:04:05"))+" "+
			styles.Dim.Render(e.Details))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(rows, "\n"))
}
