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
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type srchResultMsg struct {
	query    string
	channels []domain.Channel
	videos   []domain.Video
}
type srchChannelVideosMsg struct {
	channelID string
	videos    []domain.Video
}
type srchRecentLoadedMsg struct{ queries []string }
type srchAuxLoadedMsg struct {
	positions   map[string]int64
	watched     map[string]bool
	localStatus map[string]domain.VideoStatus
}

type Search struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	width, height int

	input     textinput.Model
	loading   bool
	lastQuery string
	channels  []domain.Channel
	videos    []domain.Video

	positions   map[string]int64
	watched     map[string]bool
	localStatus map[string]domain.VideoStatus

	drillCh    *domain.Channel
	chVideos   []domain.Video
	chLoading  bool
	drillTable table.Model

	resultsTable table.Model
	numBuf       string

	spinner spinner.Model

	recent       []string
	histIdx      int
	recentCursor int
	recentVS     int
	recentMode   bool
}

func NewSearch(backend api.Backend, keys keymap.KeyMap, circular bool) Search {
	ti := textinput.New()
	ti.Placeholder = "Search YouTube…"
	ti.CharLimit = 200
	ti.Focus()
	return Search{
		backend:      backend,
		keys:         keys,
		circular:     circular,
		input:        ti,
		spinner:      spinner.New(),
		histIdx:      -1,
		drillTable:   newTable(),
		resultsTable: newTable(),
	}
}

func (t Search) ID() tuipkg.TabID          { return tuipkg.TabSearch }
func (t Search) Title() string             { return "Search" }
func (t Search) ShortHelp() []key.Binding { return nil }
func (t Search) InterceptsInput() bool    { return t.input.Focused() }

func (t Search) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, t.spinner.Tick, t.srchLoadRecentCmd(), t.srchAuxLoadCmd())
}

func (t Search) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.input.Width = m.Width - 12
		t.drillTable.SetColumns(computeVideoColumns(t.width, false))
		// section(2) + prompt(1) + drill-sub-header(2)
		t.drillTable.SetHeight(t.height - 5)
		t.resultsTable.SetColumns(computeSearchResultColumns(t.width))
		// section(2) + prompt(1) + results-header(2)
		t.resultsTable.SetHeight(t.height - 5)
		t.resultsTable.SetRows(t.toSearchResultRows())
		return t, nil

	case tuipkg.SearchFocusInputMsg:
		if t.lastQuery == "" && !t.loading {
			t.recentMode = false
			t.histIdx = -1
			t.recentCursor = 0
			t.input.SetValue("")
			t.input.Focus()
			return t, textinput.Blink
		}
		return t, nil

	case tuipkg.HistoryChangedMsg:
		return t, t.srchLoadRecentCmd()

	case tuipkg.SearchActivateMsg:
		t.input.SetValue(m.Query)
		t.input.Blur()
		t.loading = true
		t.channels = nil
		t.videos = nil
		t.resultsTable.SetRows(nil)
		t.resultsTable.GotoTop()
		t.recentMode = false
		t.histIdx = -1
		return t, tea.Batch(t.srchCmd(m.Query), t.spinner.Tick)

	case srchAuxLoadedMsg:
		t.positions = m.positions
		t.watched = m.watched
		t.localStatus = m.localStatus

	case tuipkg.RefreshPositionsMsg:
		return t, t.srchAuxLoadCmd()

	case srchRecentLoadedMsg:
		t.recent = m.queries
		if t.recentCursor >= len(t.recent) && t.recentCursor > 0 {
			t.recentCursor = len(t.recent) - 1
		}
		if t.histIdx >= len(t.recent) {
			t.histIdx = -1
		}
		return t, nil

	case spinner.TickMsg:
		if t.loading || t.chLoading {
			var cmd tea.Cmd
			t.spinner, cmd = t.spinner.Update(m)
			return t, cmd
		}
		return t, nil

	case srchResultMsg:
		t.loading = false
		t.lastQuery = m.query
		t.channels = m.channels
		t.videos = m.videos
		t.resultsTable.SetRows(t.toSearchResultRows())
		t.resultsTable.GotoTop()
		t.drillCh = nil
		t.chVideos = nil
		return t, func() tea.Msg { return tuipkg.HistoryChangedMsg{} }

	case srchChannelVideosMsg:
		t.chLoading = false
		t.chVideos = m.videos
		t.drillTable.SetRows(toVideoRows(t.chVideos, t.positions, t.watched, t.localStatus, false, t.width))
		t.drillTable.GotoTop()
		return t, nil

	case tea.KeyMsg:
		if t.recentMode {
			return t.srchHandleKeyRecentMode(m)
		}
		if t.input.Focused() {
			return t.srchHandleKeyInput(m)
		}
		return t.srchHandleKey(m)
	}

	if t.input.Focused() {
		var cmd tea.Cmd
		t.input, cmd = t.input.Update(msg)
		return t, cmd
	}
	return t, nil
}

func (t Search) View() string {
	header := styles.SectionTitle.Render("Search")
	headerH := lipgloss.Height(header)
	label := styles.Warning.Render("Search:")
	prompt := " " + label + " " + t.input.View()
	remaining := t.height - headerH - 1

	var body string
	if t.drillCh != nil {
		body = t.viewDrillDown(prompt, remaining)
	} else {
		body = t.viewResults(prompt, remaining)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}

func (t Search) srchHandleKeyInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		n := len(t.recent)
		if n > 0 && t.histIdx < n-1 {
			t.histIdx++
			t.input.SetValue(t.recent[t.histIdx])
			t.input.CursorEnd()
			t.recentCursor = t.histIdx
			pageH := t.srchRecentPageHeight()
			_, t.recentVS = nav.Move(t.histIdx, t.recentVS, n, 0, pageH, false)
		}
		return t, nil

	case tea.KeyDown:
		if t.histIdx > 0 {
			t.histIdx--
			t.input.SetValue(t.recent[t.histIdx])
			t.input.CursorEnd()
			t.recentCursor = t.histIdx
			pageH := t.srchRecentPageHeight()
			_, t.recentVS = nav.Move(t.histIdx, t.recentVS, len(t.recent), 0, pageH, false)
		} else if t.histIdx == 0 {
			t.histIdx = -1
			t.recentCursor = -1
			t.input.SetValue("")
		}
		return t, nil

	case tea.KeyEnter:
		query := strings.TrimSpace(t.input.Value())
		if query == "" {
			return t, nil
		}
		t.histIdx = -1
		t.recentCursor = -1
		t.input.Blur()
		t.loading = true
		t.channels = nil
		t.videos = nil
		t.resultsTable.SetRows(nil)
		t.resultsTable.GotoTop()
		return t, tea.Batch(t.srchCmd(query), t.spinner.Tick)

	case tea.KeyEscape:
		t.input.Blur()
		if len(t.recent) > 0 {
			t.recentMode = true
			if t.histIdx >= 0 {
				t.recentCursor = t.histIdx
			} else {
				t.recentCursor = 0
			}
			t.histIdx = -1
		}
		return t, nil

	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace || msg.Type == tea.KeyBackspace {
			t.histIdx = -1
			t.recentCursor = -1
		}
		var cmd tea.Cmd
		t.input, cmd = t.input.Update(msg)
		return t, cmd
	}
}

func (t Search) srchHandleKeyRecentMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := len(t.recent)
	pageH := t.srchRecentPageHeight()

	switch {
	case key.Matches(msg, keys.Up):
		t.recentCursor, t.recentVS = nav.Move(t.recentCursor, t.recentVS, n, -1, pageH, t.circular)
	case key.Matches(msg, keys.Down):
		t.recentCursor, t.recentVS = nav.Move(t.recentCursor, t.recentVS, n, +1, pageH, t.circular)
	case key.Matches(msg, keys.PageUp):
		t.recentCursor, t.recentVS = nav.Page(t.recentCursor, t.recentVS, n, -1, pageH, t.circular)
	case key.Matches(msg, keys.PageDown):
		t.recentCursor, t.recentVS = nav.Page(t.recentCursor, t.recentVS, n, +1, pageH, t.circular)
	case key.Matches(msg, keys.GotoBottom):
		t.recentCursor, t.recentVS = nav.Jump(n-1, n, pageH)
	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Play):
		if t.recentCursor >= 0 && t.recentCursor < n {
			return t.srchExecuteRecent(t.recent[t.recentCursor])
		}
	case msg.Type == tea.KeyEnter:
		if t.recentCursor >= 0 && t.recentCursor < n {
			return t.srchExecuteRecent(t.recent[t.recentCursor])
		}
	case key.Matches(msg, keys.Delete):
		if t.recentCursor >= 0 && t.recentCursor < n {
			query := t.recent[t.recentCursor]
			t.recent = append(t.recent[:t.recentCursor], t.recent[t.recentCursor+1:]...)
			if t.recentCursor >= len(t.recent) && t.recentCursor > 0 {
				t.recentCursor--
			}
			if len(t.recent) == 0 {
				t.recentMode = false
				t.input.Focus()
				return t, tea.Batch(textinput.Blink, t.srchDeleteRecentCmd(query))
			}
			return t, t.srchDeleteRecentCmd(query)
		}
	case key.Matches(msg, keys.Escape), key.Matches(msg, keys.Filter):
		t.recentMode = false
		if t.recentCursor >= 0 && t.recentCursor < n {
			t.histIdx = t.recentCursor
			t.input.SetValue(t.recent[t.recentCursor])
			t.input.CursorEnd()
		}
		t.input.Focus()
		return t, textinput.Blink
	}
	return t, nil
}

func (t Search) srchHandleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := t.keys

	if key.Matches(msg, keys.Filter) {
		t.input.Focus()
		return t, textinput.Blink
	}

	if key.Matches(msg, keys.Escape) && (len(t.channels) > 0 || len(t.videos) > 0) {
		t.lastQuery = ""
		t.channels = nil
		t.videos = nil
		t.resultsTable.SetRows(nil)
		t.resultsTable.GotoTop()
		t.drillCh = nil
		t.chVideos = nil
		t.histIdx = -1
		t.recentCursor = 0
		t.recentVS = 0
		t.input.SetValue("")
		t.input.Focus()
		return t, textinput.Blink
	}

	if t.drillCh != nil {
		return t.srchHandleKeyDrill(msg)
	}

	// digit accumulation for goto-line (only in results mode, input not focused)
	if checkGotoNum(&t.numBuf, msg) {
		return t, nil
	}
	numBuf := t.numBuf
	t.numBuf = ""

	nCh := len(t.channels)
	nVid := len(t.videos)
	total := nCh + nVid

	switch {
	case key.Matches(msg, keys.GotoLine):
		if numBuf != "" {
			applyGoto(numBuf, &t.resultsTable)
		} else {
			t.resultsTable.GotoBottom()
		}
	case key.Matches(msg, keys.GotoBottom):
		t.resultsTable.GotoBottom()
	case key.Matches(msg, keys.Up):
		if t.circular && total > 0 && t.resultsTable.Cursor() == 0 {
			t.resultsTable.GotoBottom()
		} else {
			t.resultsTable.MoveUp(1)
		}
	case key.Matches(msg, keys.Down):
		if t.circular && total > 0 && t.resultsTable.Cursor() == total-1 {
			t.resultsTable.GotoTop()
		} else {
			t.resultsTable.MoveDown(1)
		}
	case key.Matches(msg, keys.PageUp):
		t.resultsTable.MoveUp(t.resultsTable.Height())
	case key.Matches(msg, keys.PageDown):
		t.resultsTable.MoveDown(t.resultsTable.Height())

	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
		c := t.resultsTable.Cursor()
		if c < nCh {
			ch := t.channels[c]
			t.drillCh = &ch
			t.chVideos = nil
			t.chLoading = true
			t.drillTable.GotoTop()
			return t, tea.Batch(t.srchChannelVideosCmd(ch), t.spinner.Tick)
		}
		if v, ok := t.srchCurrentVideo(); ok {
			return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v} }
		}

	case key.Matches(msg, keys.Play):
		if v, ok := t.srchCurrentVideo(); ok {
			return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v} }
		}
	case key.Matches(msg, keys.PlayAudio):
		if v, ok := t.srchCurrentVideo(); ok {
			return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v, AudioOnly: true} }
		}
	case key.Matches(msg, keys.Download):
		if v, ok := t.srchCurrentVideo(); ok {
			return t, func() tea.Msg { return tuipkg.EnqueueMsg{Video: v} }
		}
	case key.Matches(msg, keys.DownloadAudio):
		if v, ok := t.srchCurrentVideo(); ok {
			return t, func() tea.Msg { return tuipkg.EnqueueMsg{Video: v, AudioOnly: true} }
		}
	case key.Matches(msg, keys.CopyURL):
		if v, ok := t.srchCurrentVideo(); ok {
			return t, func() tea.Msg { return tuipkg.CopyURLMsg{URL: v.URL} }
		}
	case key.Matches(msg, keys.VideoInfo):
		if v, ok := t.srchCurrentVideo(); ok {
			return t, func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: "video_detail", Video: v} }
		}
	case key.Matches(msg, keys.AddList):
		if v, ok := t.srchCurrentVideo(); ok {
			return t, func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: "add_to_playlist", Video: v} }
		}
	}
	return t, nil
}

func (t Search) srchHandleKeyDrill(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := len(t.chVideos)

	if checkGotoNum(&t.numBuf, msg) {
		return t, nil
	}
	numBuf := t.numBuf
	t.numBuf = ""

	switch {
	case key.Matches(msg, keys.Left), key.Matches(msg, keys.Escape):
		if numBuf != "" {
			return t, nil
		}
		t.drillCh = nil
		t.chVideos = nil
	case key.Matches(msg, keys.GotoLine):
		if numBuf != "" {
			applyGoto(numBuf, &t.drillTable)
		} else {
			t.drillTable.GotoBottom()
		}
	case key.Matches(msg, keys.Up):
		if t.circular && n > 0 && t.drillTable.Cursor() == 0 {
			t.drillTable.GotoBottom()
		} else {
			t.drillTable.MoveUp(1)
		}
	case key.Matches(msg, keys.Down):
		if t.circular && n > 0 && t.drillTable.Cursor() == n-1 {
			t.drillTable.GotoTop()
		} else {
			t.drillTable.MoveDown(1)
		}
	case key.Matches(msg, keys.PageUp):
		t.drillTable.MoveUp(t.drillTable.Height())
	case key.Matches(msg, keys.PageDown):
		t.drillTable.MoveDown(t.drillTable.Height())
	case key.Matches(msg, keys.GotoBottom):
		t.drillTable.GotoBottom()
	case key.Matches(msg, keys.Play):
		if v, ok := t.srchCurrentDrillVideo(); ok {
			return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v} }
		}
	case key.Matches(msg, keys.PlayAudio):
		if v, ok := t.srchCurrentDrillVideo(); ok {
			return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v, AudioOnly: true} }
		}
	case key.Matches(msg, keys.Download):
		if v, ok := t.srchCurrentDrillVideo(); ok {
			return t, func() tea.Msg { return tuipkg.EnqueueMsg{Video: v} }
		}
	case key.Matches(msg, keys.DownloadAudio):
		if v, ok := t.srchCurrentDrillVideo(); ok {
			return t, func() tea.Msg { return tuipkg.EnqueueMsg{Video: v, AudioOnly: true} }
		}
	case key.Matches(msg, keys.CopyURL):
		if v, ok := t.srchCurrentDrillVideo(); ok {
			return t, func() tea.Msg { return tuipkg.CopyURLMsg{URL: v.URL} }
		}
	case key.Matches(msg, keys.VideoInfo):
		if v, ok := t.srchCurrentDrillVideo(); ok {
			return t, func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: "video_detail", Video: v} }
		}
	case key.Matches(msg, keys.AddList):
		if v, ok := t.srchCurrentDrillVideo(); ok {
			return t, func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: "add_to_playlist", Video: v} }
		}
	}
	return t, nil
}

func (t Search) viewDrillDown(prompt string, remaining int) string {
	_ = remaining
	header := styles.SectionTitle.Render("← " + render.Truncate(t.drillCh.Name, t.width-4))
	if t.chLoading {
		return lipgloss.JoinVertical(lipgloss.Left, prompt, header, t.spinner.View()+" Loading…")
	}
	if len(t.chVideos) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, prompt, header, styles.Dim.Render("No videos found."))
	}
	parts := []string{prompt, header, t.drillTable.View()}
	if t.numBuf != "" {
		parts = append(parts, gotoLineView(t.numBuf))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (t Search) viewResults(prompt string, remaining int) string {
	showRecent := (t.input.Focused() || t.recentMode) && t.lastQuery == "" && len(t.recent) > 0

	if t.loading {
		return lipgloss.JoinVertical(lipgloss.Left, prompt, t.spinner.View()+" Searching…")
	}
	if len(t.channels) == 0 && len(t.videos) == 0 {
		if showRecent {
			return lipgloss.JoinVertical(lipgloss.Left, prompt,
				t.viewRecentSearches(remaining-1))
		}
		var hint string
		if t.lastQuery != "" {
			hint = "No results for: " + t.lastQuery
		} else {
			hint = "Type to search YouTube  (" + t.keys.Filter.Help().Key + " to focus)"
		}
		return lipgloss.JoinVertical(lipgloss.Left, prompt, styles.Dim.PaddingLeft(1).Render(hint))
	}

	header := styles.SectionTitle.Render("Results for: " + t.lastQuery)
	parts := []string{prompt, header, t.resultsTable.View()}
	if t.numBuf != "" {
		parts = append(parts, gotoLineView(t.numBuf))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (t Search) viewRecentSearches(height int) string {
	n := len(t.recent)
	pageH := t.srchRecentPageHeight()
	if pageH > height-1 {
		pageH = height - 1
	}
	start, end := nav.Window(t.recentVS, n, pageH)

	highlighted := -1
	if t.recentMode {
		highlighted = t.recentCursor
	} else if t.histIdx >= 0 {
		highlighted = t.histIdx
	}

	nameW := t.width - render.ColNum - 1 - 2
	if nameW < 10 {
		nameW = 10
	}

	rows := make([]string, 0, end-start+2)
	rows = append(rows, styles.Dim.PaddingLeft(render.ColNum+3).Render("Recent searches"))
	for i := start; i < end; i++ {
		q := t.recent[i]
		numStyle := styles.RowNum
		indicator := "  "
		sep := " "
		nameStyle := styles.Normal.Width(nameW)
		if i == highlighted {
			indicator = styles.Selected.Render("▶ ")
			numStyle = numStyle.Background(styles.ColorBgSelect)
			sep = lipgloss.NewStyle().Background(styles.ColorBgSelect).Render(" ")
			nameStyle = styles.Selected.Width(nameW)
		}
		numStr := numStyle.Render(fmt.Sprintf("%*d", render.ColNum, i+1))
		rows = append(rows, numStr+sep+indicator+nameStyle.Render(render.Truncate(q, nameW)))
	}
	return strings.Join(rows, "\n")
}

func (t Search) toSearchResultRows() []table.Row {
	nCh := len(t.channels)
	rows := make([]table.Row, 0, nCh+len(t.videos))
	for i, ch := range t.channels {
		rows = append(rows, table.Row{
			rowNum(i),
			styles.Warning.Render("ch"),
			ch.DisplayName(),
			"", "", "", "",
		})
	}
	for i := range t.videos {
		v := &t.videos[i]
		rows = append(rows, table.Row{
			rowNum(nCh + i),
			videoIndicator(*v, t.positions, t.watched, t.localStatus),
			v.Title,
			v.Channel,
			v.DurationStr(),
			v.ViewsStr(),
			v.DateStr(),
		})
	}
	return rows
}

func (t Search) srchCmd(query string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		_ = t.backend.AddHistory(ctx, "", "search", query)
		channels, videos, err := t.backend.Search(ctx, query)
		if err != nil {
			return tuipkg.StatusMsg{Text: "search: " + err.Error(), IsErr: true}
		}
		return srchResultMsg{query: query, channels: channels, videos: videos}
	}
}

func (t Search) srchChannelVideosCmd(ch domain.Channel) tea.Cmd {
	return func() tea.Msg {
		videos, err := t.backend.ChannelVideos(context.Background(), ch.URL, ch.ID)
		if err != nil {
			return tuipkg.StatusMsg{Text: "channel videos: " + err.Error(), IsErr: true}
		}
		return srchChannelVideosMsg{channelID: ch.ID, videos: videos}
	}
}

func (t Search) srchLoadRecentCmd() tea.Cmd {
	return func() tea.Msg {
		queries, err := t.backend.SearchQueries(context.Background())
		if err != nil {
			return nil
		}
		return srchRecentLoadedMsg{queries}
	}
}

func (t Search) srchDeleteRecentCmd(query string) tea.Cmd {
	return func() tea.Msg {
		_ = t.backend.DeleteSearchHistory(context.Background(), query)
		return nil
	}
}

func (t Search) srchAuxLoadCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		positions, _ := t.backend.AllVideoPositions(ctx)
		watched, _ := t.backend.WatchedVideoIDs(ctx)
		localVids, _ := t.backend.LocalVideos(ctx)
		localStatus := make(map[string]domain.VideoStatus, len(localVids))
		for i := range localVids {
			localStatus[localVids[i].ID] = localVids[i].Status
		}
		return srchAuxLoadedMsg{positions: positions, watched: watched, localStatus: localStatus}
	}
}

func (t Search) srchExecuteRecent(query string) (tea.Model, tea.Cmd) {
	t.recentMode = false
	t.histIdx = -1
	t.recentCursor = 0
	t.loading = true
	t.channels = nil
	t.videos = nil
	t.resultsTable.SetRows(nil)
	t.resultsTable.GotoTop()
	t.input.SetValue(query)
	t.input.Blur()
	return t, tea.Batch(t.srchCmd(query), t.spinner.Tick)
}

func (t Search) srchCurrentVideo() (domain.Video, bool) {
	idx := t.resultsTable.Cursor() - len(t.channels)
	if idx >= 0 && idx < len(t.videos) {
		return t.videos[idx], true
	}
	return domain.Video{}, false
}

func (t Search) srchCurrentDrillVideo() (domain.Video, bool) {
	c := t.drillTable.Cursor()
	if c >= 0 && c < len(t.chVideos) {
		return t.chVideos[c], true
	}
	return domain.Video{}, false
}

func (t Search) srchRecentPageHeight() int {
	h := t.height - 4
	if h < 1 {
		h = 1
	}
	return h
}
