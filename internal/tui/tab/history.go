package tab

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
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

const histColStatus = 14

const (
	colKeyHistNum   = "hnum"
	colKeyHistInd   = "hind"
	colKeyHistType  = "htype"
	colKeyHistTitle = "htitle"
	colKeyHistCh    = "hch"
	colKeyHistDur   = "hdur"
	colKeyHistViews = "hviews"
	colKeyHistDate  = "hdate"

	colKeyDetailType = "dtype"
	colKeyDetailTs   = "dts"
	colKeyDetailInfo = "dinfo"
)

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

	nav         videotable.TableNav
	detailNav   videotable.TableNav
	histCols    []videotable.ColumnDef[domain.HistoryEntry]
	detailCols  []videotable.ColumnDef[domain.HistoryEntry]

	sortMode        int
	sortChordActive bool
}

func histColumns(durW int) []videotable.ColumnDef[domain.HistoryEntry] {
	return []videotable.ColumnDef[domain.HistoryEntry]{
		{
			Col:  etable.NewColumn(colKeyHistNum, ralign("#", render.ColNum), render.ColNum),
			Cell: func(e domain.HistoryEntry, i int) any { return fmt.Sprintf("%4d", i+1) },
		},
		{
			Col: etable.NewColumn(colKeyHistInd, " ", colIndicator),
			Cell: func(e domain.HistoryEntry, _ int) any {
				switch e.EventType {
				case "download video", "download audio":
					return " ● "
				default:
					return " ○ "
				}
			},
		},
		{
			Col: etable.NewColumn(colKeyHistType, "Type", histColStatus),
			Cell: func(e domain.HistoryEntry, _ int) any {
				return etable.NewStyledCell(render.FormatEvent(e.EventType), styles.Warning)
			},
		},
		{
			Col:  etable.NewFlexColumn(colKeyHistTitle, "Title", 1),
			Cell: func(e domain.HistoryEntry, _ int) any { return e.Title },
		},
		{
			Col:  etable.NewColumn(colKeyHistCh, "Channel", render.ColChannel),
			Cell: func(e domain.HistoryEntry, _ int) any { return e.Channel },
		},
		{
			Col: etable.NewColumn(colKeyHistDur, ralign("Duration", durW+1), durW+1),
			Cell: func(e domain.HistoryEntry, _ int) any {
				return fmt.Sprintf("%*s ", durW, render.Duration(e.Duration))
			},
		},
		{
			Col: etable.NewColumn(colKeyHistViews, ralign("Views", render.ColViews+1), render.ColViews+1),
			Cell: func(e domain.HistoryEntry, _ int) any {
				return fmt.Sprintf("%*s ", render.ColViews, render.Views(e.ViewCount))
			},
		},
		{
			Col:  etable.NewColumn(colKeyHistDate, "Date", render.ColDate),
			Cell: func(e domain.HistoryEntry, _ int) any { return render.Date(e.UploadDate) },
		},
	}
}

func histDetailColumns() []videotable.ColumnDef[domain.HistoryEntry] {
	return []videotable.ColumnDef[domain.HistoryEntry]{
		{
			Col: etable.NewColumn(colKeyDetailType, "Type", histColStatus),
			Cell: func(e domain.HistoryEntry, _ int) any {
				return etable.NewStyledCell(render.FormatEvent(e.EventType), styles.Warning)
			},
		},
		{
			Col:  etable.NewColumn(colKeyDetailTs, "Timestamp", 19),
			Cell: func(e domain.HistoryEntry, _ int) any { return e.Timestamp.Format("2006-01-02 15:04:05") },
		},
		{
			Col:  etable.NewFlexColumn(colKeyDetailInfo, "Details", 1),
			Cell: func(e domain.HistoryEntry, _ int) any { return strings.TrimSpace(e.Details) },
		},
	}
}

func NewHistory(backend api.Backend, keys keymap.KeyMap, circular bool) History {
	durW := render.ColDuration
	hCols := histColumns(durW)
	dCols := histDetailColumns()
	return History{
		backend:    backend,
		keys:       keys,
		circular:   circular,
		nav:        videotable.NewTableNav(videotable.NewTable(hCols), circular, 2),
		detailNav:  videotable.NewTableNav(videotable.NewTable(dCols), false, 2),
		histCols:   hCols,
		detailCols: dCols,
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
		t.nav.Resize(m.Width, m.Height)
		t.nav.SetRows(videotable.BuildRows(t.entries, t.histCols))
		t.detailNav.Resize(m.Width, m.Height)
	case tuipkg.HistoryChangedMsg:
		return t, t.loadCmd()
	case histLoadedMsg:
		t.entries = m.entries
		feed.SortHistoryEntries(t.entries, t.sortMode)
		t.loaded = true
		t.nav.SetRows(videotable.BuildRows(t.entries, t.histCols))
		t.nav.GotoRow(0)
		t.detailVideoID = ""
	case histDetailLoadedMsg:
		t.detailVideoID = m.videoID
		t.detail = m.entries
		t.detailNav.SetRows(videotable.BuildRows(t.detail, t.detailCols))
		t.detailNav.GotoRow(0)
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
		t.nav.SetRows(videotable.BuildRows(t.entries, t.histCols))
		return t, nil
	}

	// ── detail pane ───────────────────────────────────────────────────────────
	if t.detailVideoID != "" {
		n := len(t.detail)
		numBufBefore := t.detailNav.NumBufView() != ""
		if t.detailNav.HandleNav(msg, keys, n) {
			return t, nil
		}
		if key.Matches(msg, keys.Escape) || key.Matches(msg, keys.Left) {
			if numBufBefore {
				return t, nil
			}
			t.detailVideoID = ""
			t.detail = nil
		}
		return t, nil
	}

	// ── list pane ─────────────────────────────────────────────────────────────
	n := len(t.entries)
	if t.nav.HandleNav(msg, keys, n) {
		return t, nil
	}

	idx := t.nav.Index()

	switch {
	case key.Matches(msg, keys.Play):
		if idx < n {
			e := t.entries[idx]
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
		if idx < n {
			return t, t.histLoadDetailCmd(t.entries[idx].VideoID)
		}
	case key.Matches(msg, keys.Delete):
		if idx < n {
			e := t.entries[idx]
			t.entries = append(t.entries[:idx], t.entries[idx+1:]...)
			t.nav.SetRows(videotable.BuildRows(t.entries, t.histCols))
			return t, t.histDeleteCmd(e)
		}
	case key.Matches(msg, keys.HideChannel):
		if idx < n {
			ch := domain.Channel{ID: t.entries[idx].ChannelID, Name: t.entries[idx].Channel}
			return t, func() tea.Msg { return tuipkg.HideChannelMsg{Channel: ch} }
		}
	case key.Matches(msg, keys.CopyURL):
		if idx < n {
			url := "https://www.youtube.com/watch?v=" + t.entries[idx].VideoID
			return t, func() tea.Msg { return tuipkg.CopyURLMsg{URL: url} }
		}
	case key.Matches(msg, keys.Refresh):
		t.loaded = false
		return t, t.loadCmd()
	case key.Matches(msg, keys.SortChord):
		t.sortChordActive = true
	case key.Matches(msg, keys.Escape):
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
	parts := []string{header, t.nav.View()}
	if s := t.nav.NumBufView(); s != "" {
		parts = append(parts, s)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (t History) renderDetail() string {
	title := ""
	if len(t.detail) > 0 {
		title = t.detail[0].Title
	}
	header := styles.SectionTitle.Render("← " + render.Truncate(title, t.width-4))
	parts := []string{header, t.detailNav.View()}
	if s := t.detailNav.NumBufView(); s != "" {
		parts = append(parts, s)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
