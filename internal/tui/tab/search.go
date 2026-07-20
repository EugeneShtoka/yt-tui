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
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	etable "github.com/evertras/bubble-table/table"
)

const (
	colKeySrchChNum  = "srchnum"
	colKeySrchChInd  = "srchind"
	colKeySrchChName = "srchname"
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

	aux videotable.AuxData

	drillCh    *domain.Channel
	chVideos   []domain.Video
	chLoading  bool
	drillTable etable.Model
	drillCols  []videotable.VideoColumnDef

	chTable  etable.Model
	chCols   []videotable.ColumnDef[domain.Channel]
	vidTable etable.Model
	vidCols  []videotable.VideoColumnDef
	onVideos bool // false = channel pane focused, true = video pane focused

	numBuf        string
	gotoTopActive bool

	spinner spinner.Model

	recent       []string
	histIdx      int
	recentCursor int
	recentVS     int
	recentMode   bool
}

func searchChannelColumns() []videotable.ColumnDef[domain.Channel] {
	return []videotable.ColumnDef[domain.Channel]{
		{
			Col:  etable.NewColumn(colKeySrchChNum, ralign("#", render.ColNum), render.ColNum),
			Cell: func(ch domain.Channel, i int) any { return fmt.Sprintf("%4d", i+1) },
		},
		{
			Col:  etable.NewColumn(colKeySrchChInd, " ", colIndicator),
			Cell: func(ch domain.Channel, _ int) any { return "   " },
		},
		{
			Col:  etable.NewFlexColumn(colKeySrchChName, "Name", 1),
			Cell: func(ch domain.Channel, _ int) any { return ch.DisplayName() },
		},
	}
}

func NewSearch(backend api.Backend, keys keymap.KeyMap, circular bool) Search {
	ti := textinput.New()
	ti.Placeholder = "Search YouTube…"
	ti.CharLimit = 200
	ti.Focus()

	chCols := searchChannelColumns()
	vidCols := []videotable.VideoColumnDef{
		videotable.Num, videotable.Indicator, videotable.Title,
		videotable.Channel, videotable.DurationCol(), videotable.Views, videotable.Date,
	}
	drillCols := []videotable.VideoColumnDef{
		videotable.Num, videotable.Indicator, videotable.Title,
		videotable.DurationCol(), videotable.Views, videotable.Date,
	}
	return Search{
		backend:    backend,
		keys:       keys,
		circular:   circular,
		input:      ti,
		spinner:    spinner.New(),
		histIdx:    -1,
		chTable:    videotable.NewTable(chCols),
		vidTable:   videotable.NewVideoTable(vidCols),
		drillTable: videotable.NewVideoTable(drillCols),
		chCols:     chCols,
		vidCols:    vidCols,
		drillCols:  drillCols,
	}
}

func (t Search) ID() tuipkg.TabID         { return tuipkg.TabSearch }
func (t Search) Title() string            { return "Search" }
func (t Search) ShortHelp() []key.Binding {
	if t.input.Focused() {
		return nil
	}
	return []key.Binding{t.keys.Play, t.keys.Download, t.keys.CopyURL, t.keys.VideoInfo, t.keys.DrillDown}
}
func (t Search) InterceptsInput() bool { return t.input.Focused() }

func (t Search) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, t.spinner.Tick, t.srchLoadRecentCmd(), videotable.LoadAuxDataCmd(t.backend))
}

func (t *Search) applyResultHeights() {
	nCh := len(t.channels)
	nVid := len(t.videos)
	avail := t.height - 3
	if nCh > 0 && nVid > 0 {
		avail -= 4
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
		t.chTable = t.chTable.WithTargetHeight(chH)
		t.vidTable = t.vidTable.WithTargetHeight(vidH)
	} else {
		avail -= 2
		if avail < 1 {
			avail = 1
		}
		t.chTable = t.chTable.WithTargetHeight(avail)
		t.vidTable = t.vidTable.WithTargetHeight(avail)
	}
}

func (t Search) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.input.SetWidth(m.Width - 12)
		t.drillTable = t.drillTable.WithTargetWidth(m.Width).WithTargetHeight(m.Height - 5)
		t.chTable = t.chTable.WithTargetWidth(m.Width)
		t.vidTable = t.vidTable.WithTargetWidth(m.Width)
		t.applyResultHeights()
		t.chTable = t.chTable.WithRows(videotable.BuildRows(t.channels, t.chCols))
		t.vidTable = t.vidTable.WithRows(videotable.BuildVideoRows(t.videos, t.vidCols, t.aux.RenderCtx(nil)))
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
		t.chTable = t.chTable.WithRows(nil).WithHighlightedRow(0)
		t.vidTable = t.vidTable.WithRows(nil).WithHighlightedRow(0)
		t.recentMode = false
		t.histIdx = -1
		return t, tea.Batch(t.srchCmd(m.Query), t.spinner.Tick)

	case videotable.AuxDataMsg:
		t.aux = m

	case tuipkg.RefreshPositionsMsg:
		return t, videotable.LoadAuxDataCmd(t.backend)

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
		t.chTable = t.chTable.WithRows(videotable.BuildRows(t.channels, t.chCols)).WithHighlightedRow(0)
		t.vidTable = t.vidTable.WithRows(videotable.BuildVideoRows(t.videos, t.vidCols, t.aux.RenderCtx(nil))).WithHighlightedRow(0)
		t.drillCh = nil
		t.chVideos = nil
		t.onVideos = len(t.channels) == 0
		return t, func() tea.Msg { return tuipkg.HistoryChangedMsg{} }

	case srchChannelVideosMsg:
		t.chLoading = false
		t.chVideos = m.videos
		t.drillTable = t.drillTable.
			WithRows(videotable.BuildVideoRows(t.chVideos, t.drillCols, t.aux.RenderCtx(nil))).
			WithHighlightedRow(0)
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
		t.chTable = t.chTable.WithRows(nil).WithHighlightedRow(0)
		t.vidTable = t.vidTable.WithRows(nil).WithHighlightedRow(0)
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
		t.chTable = t.chTable.WithRows(nil).WithHighlightedRow(0)
		t.vidTable = t.vidTable.WithRows(nil).WithHighlightedRow(0)
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
				t.vidTable = t.vidTable.WithHighlightedRow(0)
			} else {
				t.chTable = t.chTable.WithHighlightedRow(0)
			}
		}
		return t, nil
	}

	if checkGotoNum(&t.numBuf, msg) {
		return t, nil
	}
	numBuf := t.numBuf
	t.numBuf = ""

	if key.Matches(msg, keys.ToggleMode) {
		if len(t.channels) > 0 && len(t.videos) > 0 {
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
	idx := t.chTable.GetHighlightedRowIndex()

	switch {
	case key.Matches(msg, keys.GotoLine):
		if numBuf != "" {
			if row := gotoRowIndex(numBuf); row >= 0 {
				t.chTable = t.chTable.WithHighlightedRow(row)
			}
		} else if n > 0 {
			t.chTable = t.chTable.WithHighlightedRow(n - 1)
		}
	case key.Matches(msg, keys.GotoBottom):
		if n > 0 {
			t.chTable = t.chTable.WithHighlightedRow(n - 1)
		}
	case key.Matches(msg, keys.Up):
		if idx > 0 {
			t.chTable = t.chTable.WithHighlightedRow(idx - 1)
		} else if t.circular && n > 0 {
			t.chTable = t.chTable.WithHighlightedRow(n - 1)
		}
	case key.Matches(msg, keys.Down):
		if idx < n-1 {
			t.chTable = t.chTable.WithHighlightedRow(idx + 1)
		} else if t.circular && n > 0 {
			t.chTable = t.chTable.WithHighlightedRow(0)
		}
	case key.Matches(msg, keys.PageUp):
		pageH := t.height - 3
		if newIdx := idx - pageH; newIdx > 0 {
			t.chTable = t.chTable.WithHighlightedRow(newIdx)
		} else {
			t.chTable = t.chTable.WithHighlightedRow(0)
		}
	case key.Matches(msg, keys.PageDown):
		pageH := t.height - 3
		if newIdx := idx + pageH; newIdx < n {
			t.chTable = t.chTable.WithHighlightedRow(newIdx)
		} else if n > 0 {
			t.chTable = t.chTable.WithHighlightedRow(n - 1)
		}
	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right), msg.Code == tea.KeyEnter:
		if idx >= 0 && idx < n {
			ch := t.channels[idx]
			t.drillCh = &ch
			t.chVideos = nil
			t.chLoading = true
			t.drillTable = t.drillTable.WithHighlightedRow(0)
			return t, tea.Batch(t.srchChannelVideosCmd(ch), t.spinner.Tick)
		}
	}
	return t, nil
}

func (t Search) srchHandleKeyVideos(msg tea.KeyPressMsg, numBuf string) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := len(t.videos)
	idx := t.vidTable.GetHighlightedRowIndex()

	switch {
	case key.Matches(msg, keys.GotoLine):
		if numBuf != "" {
			if row := gotoRowIndex(numBuf); row >= 0 {
				t.vidTable = t.vidTable.WithHighlightedRow(row)
			}
		} else if n > 0 {
			t.vidTable = t.vidTable.WithHighlightedRow(n - 1)
		}
	case key.Matches(msg, keys.GotoBottom):
		if n > 0 {
			t.vidTable = t.vidTable.WithHighlightedRow(n - 1)
		}
	case key.Matches(msg, keys.Up):
		if idx > 0 {
			t.vidTable = t.vidTable.WithHighlightedRow(idx - 1)
		} else if t.circular && n > 0 {
			t.vidTable = t.vidTable.WithHighlightedRow(n - 1)
		}
	case key.Matches(msg, keys.Down):
		if idx < n-1 {
			t.vidTable = t.vidTable.WithHighlightedRow(idx + 1)
		} else if t.circular && n > 0 {
			t.vidTable = t.vidTable.WithHighlightedRow(0)
		}
	case key.Matches(msg, keys.PageUp):
		pageH := t.height - 3
		if newIdx := idx - pageH; newIdx > 0 {
			t.vidTable = t.vidTable.WithHighlightedRow(newIdx)
		} else {
			t.vidTable = t.vidTable.WithHighlightedRow(0)
		}
	case key.Matches(msg, keys.PageDown):
		pageH := t.height - 3
		if newIdx := idx + pageH; newIdx < n {
			t.vidTable = t.vidTable.WithHighlightedRow(newIdx)
		} else if n > 0 {
			t.vidTable = t.vidTable.WithHighlightedRow(n - 1)
		}
	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
		if v, ok := t.srchCurrentVideo(); ok {
			return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v} }
		}
	default:
		if v, ok := t.srchCurrentVideo(); ok {
			if cmd, ok := HandleVideoAction(msg, v, keys); ok {
				return t, cmd
			}
		}
	}
	return t, nil
}

func (t Search) srchHandleKeyDrill(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := len(t.chVideos)
	idx := t.drillTable.GetHighlightedRowIndex()

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
			if row := gotoRowIndex(numBuf); row >= 0 {
				t.drillTable = t.drillTable.WithHighlightedRow(row)
			}
		} else if n > 0 {
			t.drillTable = t.drillTable.WithHighlightedRow(n - 1)
		}
	case key.Matches(msg, keys.GotoBottom):
		if n > 0 {
			t.drillTable = t.drillTable.WithHighlightedRow(n - 1)
		}
	case key.Matches(msg, keys.Up):
		if idx > 0 {
			t.drillTable = t.drillTable.WithHighlightedRow(idx - 1)
		} else if t.circular && n > 0 {
			t.drillTable = t.drillTable.WithHighlightedRow(n - 1)
		}
	case key.Matches(msg, keys.Down):
		if idx < n-1 {
			t.drillTable = t.drillTable.WithHighlightedRow(idx + 1)
		} else if t.circular && n > 0 {
			t.drillTable = t.drillTable.WithHighlightedRow(0)
		}
	case key.Matches(msg, keys.PageUp):
		pageH := t.height - 5
		if newIdx := idx - pageH; newIdx > 0 {
			t.drillTable = t.drillTable.WithHighlightedRow(newIdx)
		} else {
			t.drillTable = t.drillTable.WithHighlightedRow(0)
		}
	case key.Matches(msg, keys.PageDown):
		pageH := t.height - 5
		if newIdx := idx + pageH; newIdx < n {
			t.drillTable = t.drillTable.WithHighlightedRow(newIdx)
		} else if n > 0 {
			t.drillTable = t.drillTable.WithHighlightedRow(n - 1)
		}
	default:
		if v, ok := t.srchCurrentDrillVideo(); ok {
			if cmd, ok := HandleVideoAction(msg, v, keys); ok {
				return t, cmd
			}
		}
	}
	return t, nil
}

func (t Search) viewDrillDown(prompt string, _ int) string {
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
			return lipgloss.JoinVertical(lipgloss.Left, prompt, t.viewRecentSearches(remaining-1))
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

func (t Search) srchExecuteRecent(query string) (tea.Model, tea.Cmd) {
	t.recentMode = false
	t.histIdx = -1
	t.recentCursor = 0
	t.loading = true
	t.channels = nil
	t.videos = nil
	t.chTable = t.chTable.WithRows(nil).WithHighlightedRow(0)
	t.vidTable = t.vidTable.WithRows(nil).WithHighlightedRow(0)
	t.input.SetValue(query)
	t.input.Blur()
	return t, tea.Batch(t.srchCmd(query), t.spinner.Tick)
}

func (t Search) srchCurrentVideo() (domain.Video, bool) {
	idx := t.vidTable.GetHighlightedRowIndex()
	if idx >= 0 && idx < len(t.videos) {
		return t.videos[idx], true
	}
	return domain.Video{}, false
}

func (t Search) srchCurrentDrillVideo() (domain.Video, bool) {
	c := t.drillTable.GetHighlightedRowIndex()
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
