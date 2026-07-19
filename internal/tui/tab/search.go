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
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

	// split result panes
	chTable       table.Model
	vidTable      table.Model
	onVideos      bool // false = channel pane focused, true = video pane focused
	numBuf        string
	gotoTopActive bool

	spinner spinner.Model

	recent       []string
	histIdx      int
	recentCursor int
	recentVS     int
	recentMode   bool
}

func computeSearchChannelColumns(width int) []table.Column {
	nameW := width - render.ColNum - colIndicator
	if nameW < 20 {
		nameW = 20
	}
	return []table.Column{
		{Title: ralign("#", render.ColNum), Width: render.ColNum},
		{Title: " ", Width: colIndicator},
		{Title: "Name", Width: nameW},
	}
}

func computeSearchVideoColumns(width int) []table.Column {
	return computeVideoColumns(width, true)
}

func NewSearch(backend api.Backend, keys keymap.KeyMap, circular bool) Search {
	ti := textinput.New()
	ti.Placeholder = "Search YouTube…"
	ti.CharLimit = 200
	ti.Focus()
	return Search{
		backend:    backend,
		keys:       keys,
		circular:   circular,
		input:      ti,
		spinner:    spinner.New(),
		histIdx:    -1,
		drillTable: newTable(),
		chTable:    newTable(),
		vidTable:   newTable(),
	}
}

func (t Search) ID() tuipkg.TabID          { return tuipkg.TabSearch }
func (t Search) Title() string             { return "Search" }
func (t Search) ShortHelp() []key.Binding {
	if t.input.Focused() {
		return nil
	}
	return []key.Binding{t.keys.Play, t.keys.Download, t.keys.CopyURL, t.keys.VideoInfo, t.keys.DrillDown}
}
func (t Search) InterceptsInput() bool { return t.input.Focused() }

func (t Search) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, t.spinner.Tick, t.srchLoadRecentCmd(), t.srchAuxLoadCmd())
}

func (t *Search) applyResultHeights() {
	nCh := len(t.channels)
	nVid := len(t.videos)
	// prompt(1) + "Results for"(2) = 3 used in viewResults before tables
	// each pane: label(1) + table-header(1) = 2 overhead
	avail := t.height - 3
	if nCh > 0 && nVid > 0 {
		avail -= 4 // two labels + two table headers
		chH := nCh
		if chH > avail/3 {
			chH = avail / 3
		}
		if chH < 1 {
			chH = 1
		}
		vidH := avail - chH
		if vidH < 1 {
			vidH = 1
		}
		t.chTable.SetHeight(chH)
		t.vidTable.SetHeight(vidH)
	} else {
		avail -= 2 // one label + one table header
		if avail < 1 {
			avail = 1
		}
		t.chTable.SetHeight(avail)
		t.vidTable.SetHeight(avail)
	}
}

func (t Search) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.input.SetWidth(m.Width - 12)
		t.drillTable.SetColumns(computeVideoColumns(t.width, false))
		t.drillTable.SetHeight(t.height - 5)
		t.chTable.SetColumns(computeSearchChannelColumns(t.width))
		t.vidTable.SetColumns(computeSearchVideoColumns(t.width))
		t.applyResultHeights()
		t.chTable.SetRows(t.toSearchChannelRows())
		t.vidTable.SetRows(toVideoRows(t.videos, t.positions, t.watched, t.localStatus, true, t.width))
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
		t.chTable.SetRows(nil)
		t.vidTable.SetRows(nil)
		t.chTable.GotoTop()
		t.vidTable.GotoTop()
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
		t.applyResultHeights()
		t.chTable.SetRows(t.toSearchChannelRows())
		t.chTable.GotoTop()
		t.vidTable.SetRows(toVideoRows(t.videos, t.positions, t.watched, t.localStatus, true, t.width))
		t.vidTable.GotoTop()
		t.drillCh = nil
		t.chVideos = nil
		// focus channels if any, else videos
		t.onVideos = len(t.channels) == 0
		return t, func() tea.Msg { return tuipkg.HistoryChangedMsg{} }

	case srchChannelVideosMsg:
		t.chLoading = false
		t.chVideos = m.videos
		t.drillTable.SetRows(toVideoRows(t.chVideos, t.positions, t.watched, t.localStatus, false, t.width))
		t.drillTable.GotoTop()
		return t, nil

	case tea.KeyPressMsg:
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

func (t Search) View() tea.View {
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
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, body))
}

func (t Search) srchHandleKeyInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.Code {
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
		t.chTable.SetRows(nil)
		t.vidTable.SetRows(nil)
		t.chTable.GotoTop()
		t.vidTable.GotoTop()
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
		if len(msg.Text) > 0 || msg.Code == tea.KeySpace || msg.Code == tea.KeyBackspace {
			t.histIdx = -1
			t.recentCursor = -1
		}
		var cmd tea.Cmd
		t.input, cmd = t.input.Update(msg)
		return t, cmd
	}
}

func (t Search) srchHandleKeyRecentMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
	case msg.Code == tea.KeyEnter:
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

func (t Search) srchHandleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys

	if key.Matches(msg, keys.Filter) {
		t.input.Focus()
		return t, textinput.Blink
	}

	if key.Matches(msg, keys.Escape) && (len(t.channels) > 0 || len(t.videos) > 0) {
		t.lastQuery = ""
		t.channels = nil
		t.videos = nil
		t.chTable.SetRows(nil)
		t.vidTable.SetRows(nil)
		t.chTable.GotoTop()
		t.vidTable.GotoTop()
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

	if consumed, doTop := handleGotoPrefix(&t.gotoTopActive, t.keys, msg); consumed {
		if doTop {
			t.numBuf = ""
			if t.onVideos {
				t.vidTable.GotoTop()
			} else {
				t.chTable.GotoTop()
			}
		}
		return t, nil
	}

	// digit accumulation for goto-line
	if checkGotoNum(&t.numBuf, msg) {
		return t, nil
	}
	numBuf := t.numBuf
	t.numBuf = ""

	// toggle between channel and video panes
	if key.Matches(msg, keys.ToggleMode) {
		nCh := len(t.channels)
		nVid := len(t.videos)
		if nCh > 0 && nVid > 0 {
			t.onVideos = !t.onVideos
		}
		return t, nil
	}

	if t.onVideos {
		return t.srchHandleKeyVideos(msg, numBuf)
	}
	return t.srchHandleKeyChannels(msg, numBuf)
}

func (t Search) srchHandleKeyChannels(msg tea.KeyPressMsg, numBuf string) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := len(t.channels)

	switch {
	case key.Matches(msg, keys.GotoLine):
		if numBuf != "" {
			applyGoto(numBuf, &t.chTable)
		} else {
			t.chTable.GotoBottom()
		}
	case key.Matches(msg, keys.GotoBottom):
		t.chTable.GotoBottom()
	case key.Matches(msg, keys.Up):
		if t.circular && n > 0 && t.chTable.Cursor() == 0 {
			t.chTable.GotoBottom()
		} else {
			t.chTable.MoveUp(1)
		}
	case key.Matches(msg, keys.Down):
		if t.circular && n > 0 && t.chTable.Cursor() == n-1 {
			t.chTable.GotoTop()
		} else {
			t.chTable.MoveDown(1)
		}
	case key.Matches(msg, keys.PageUp):
		t.chTable.MoveUp(t.chTable.Height())
	case key.Matches(msg, keys.PageDown):
		t.chTable.MoveDown(t.chTable.Height())

	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right), msg.Code == tea.KeyEnter:
		c := t.chTable.Cursor()
		if c >= 0 && c < n {
			ch := t.channels[c]
			t.drillCh = &ch
			t.chVideos = nil
			t.chLoading = true
			t.drillTable.GotoTop()
			return t, tea.Batch(t.srchChannelVideosCmd(ch), t.spinner.Tick)
		}
	}
	return t, nil
}

func (t Search) srchHandleKeyVideos(msg tea.KeyPressMsg, numBuf string) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := len(t.videos)

	switch {
	case key.Matches(msg, keys.GotoLine):
		if numBuf != "" {
			applyGoto(numBuf, &t.vidTable)
		} else {
			t.vidTable.GotoBottom()
		}
	case key.Matches(msg, keys.GotoBottom):
		t.vidTable.GotoBottom()
	case key.Matches(msg, keys.Up):
		if t.circular && n > 0 && t.vidTable.Cursor() == 0 {
			t.vidTable.GotoBottom()
		} else {
			t.vidTable.MoveUp(1)
		}
	case key.Matches(msg, keys.Down):
		if t.circular && n > 0 && t.vidTable.Cursor() == n-1 {
			t.vidTable.GotoTop()
		} else {
			t.vidTable.MoveDown(1)
		}
	case key.Matches(msg, keys.PageUp):
		t.vidTable.MoveUp(t.vidTable.Height())
	case key.Matches(msg, keys.PageDown):
		t.vidTable.MoveDown(t.vidTable.Height())

	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
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

func (t Search) srchHandleKeyDrill(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
	_ = remaining
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

	resultsHeader := styles.SectionTitle.Render("Results for: " + t.lastQuery)
	parts := []string{prompt, resultsHeader}

	if len(t.channels) > 0 {
		parts = append(parts, t.srchPaneLabel("Channels", !t.onVideos), t.chTable.View())
	}
	if len(t.videos) > 0 {
		parts = append(parts, t.srchPaneLabel("Videos", t.onVideos), t.vidTable.View())
	}
	if t.numBuf != "" {
		parts = append(parts, gotoLineView(t.numBuf))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (t Search) srchPaneLabel(name string, focused bool) string {
	indicator := "  "
	style := styles.Dim
	if focused {
		indicator = "▶ "
		style = styles.Bold
	}
	return style.Render(indicator + name)
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

func (t Search) toSearchChannelRows() []table.Row {
	rows := make([]table.Row, len(t.channels))
	for i, ch := range t.channels {
		rows[i] = table.Row{
			rowNum(i),
			"   ",
			ch.DisplayName(),
		}
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
	t.chTable.SetRows(nil)
	t.vidTable.SetRows(nil)
	t.chTable.GotoTop()
	t.vidTable.GotoTop()
	t.input.SetValue(query)
	t.input.Blur()
	return t, tea.Batch(t.srchCmd(query), t.spinner.Tick)
}

func (t Search) srchCurrentVideo() (domain.Video, bool) {
	idx := t.vidTable.Cursor()
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
