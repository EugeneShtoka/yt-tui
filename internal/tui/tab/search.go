package tab

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
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

// ── recentSearches sub-model ─────────────────────────────────────────────────

type recentSearches struct {
	queries  []string
	cursor   int
	vs       int // viewport start
	circular bool
}

func (r *recentSearches) move(delta int) {
	n := len(r.queries)
	if n <= 0 {
		return
	}
	c := r.cursor + delta
	if r.circular {
		c = ((c % n) + n) % n
	} else {
		if c < 0 {
			c = 0
		}
		if c >= n {
			c = n - 1
		}
	}
	r.syncViewport(c, 0)
}

func (r *recentSearches) page(direction, pageH int) {
	n := len(r.queries)
	if n <= 0 {
		return
	}
	relPos := r.cursor - r.vs
	newVS := r.vs + direction*pageH
	if newVS < 0 {
		newVS = 0
	}
	if newVS+pageH > n {
		newVS = n - pageH
		if newVS < 0 {
			newVS = 0
		}
	}
	c := newVS + relPos
	if c < 0 {
		c = 0
	}
	if c >= n {
		c = n - 1
	}
	r.vs = newVS
	r.cursor = c
}

func (r *recentSearches) jumpTo(idx int) {
	n := len(r.queries)
	if n <= 0 {
		return
	}
	r.syncViewport(idx, 0)
}

// syncViewport updates cursor and adjusts vs so cursor stays visible.
func (r *recentSearches) syncViewport(c, pageH int) {
	n := len(r.queries)
	if c < 0 {
		c = 0
	}
	if c >= n {
		c = n - 1
	}
	if pageH > 0 {
		if c < r.vs {
			r.vs = c
		}
		if c >= r.vs+pageH {
			r.vs = c - pageH + 1
		}
		if r.vs < 0 {
			r.vs = 0
		}
	}
	r.cursor = c
}

func (r *recentSearches) window(pageH int) (start, end int) {
	n := len(r.queries)
	if n == 0 || pageH <= 0 {
		return 0, 0
	}
	if pageH >= n {
		return 0, n
	}
	start = r.vs
	end = start + pageH
	if end > n {
		end = n
		start = end - pageH
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

// ── drill sub-model ───────────────────────────────────────────────────────────

type drillState struct {
	ch      *domain.Channel
	videos  []domain.Video
	loading bool
	nav     videotable.TableNav
	cols    []videotable.ColumnDef[videotable.VideoData]
}

func newDrillState(circular bool) drillState {
	cols := []videotable.ColumnDef[videotable.VideoData]{
		videotable.NumCol[videotable.VideoData](), videotable.IndicatorCol[videotable.VideoData](), videotable.TitleFlexCol[videotable.VideoData](),
		videotable.DurationCol[videotable.VideoData](), videotable.ViewsCol[videotable.VideoData](), videotable.DateCol[videotable.VideoData](),
	}
	return drillState{
		nav:  videotable.NewTableNav(videotable.NewVideoTable(cols), circular, 5),
		cols: cols,
	}
}

func (d *drillState) resize(w, h int) {
	d.nav.Resize(w, h)
}

func (d *drillState) setVideos(videos []domain.Video, aux videotable.AuxData) {
	d.videos = videos
	d.nav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(videos, aux), d.cols))
}

func (d *drillState) currentVideo() (domain.Video, bool) {
	idx := d.nav.Index()
	if idx >= 0 && idx < len(d.videos) {
		return d.videos[idx], true
	}
	return domain.Video{}, false
}

// ── Search model ─────────────────────────────────────────────────────────────

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

	drill drillState

	chNav    videotable.TableNav
	chCols   []videotable.ColumnDef[domain.Channel]
	vidNav   videotable.TableNav
	vidCols  []videotable.ColumnDef[videotable.VideoData]
	onVideos bool // false = channel pane focused, true = video pane focused

	spinner spinner.Model

	recent     recentSearches
	histIdx    int
	recentMode bool
}

func NewSearch(backend api.Backend, keys keymap.KeyMap, circular bool) Search {
	ti := textinput.New()
	ti.Placeholder = "Search YouTube…"
	ti.CharLimit = 200
	ti.Focus()

	chCols := []videotable.ColumnDef[domain.Channel]{
		videotable.NumCol[domain.Channel](),
		videotable.BlankIndicatorCol[domain.Channel](),
		videotable.TitleFlexCol[domain.Channel](),
	}
	vidCols := []videotable.ColumnDef[videotable.VideoData]{
		videotable.NumCol[videotable.VideoData](), videotable.IndicatorCol[videotable.VideoData](), videotable.TitleFlexCol[videotable.VideoData](),
		videotable.ChannelCol[videotable.VideoData](), videotable.DurationCol[videotable.VideoData](), videotable.ViewsCol[videotable.VideoData](), videotable.DateCol[videotable.VideoData](),
	}
	return Search{
		backend:  backend,
		keys:     keys,
		circular: circular,
		input:    ti,
		spinner:  spinner.New(),
		histIdx:  -1,
		chNav:    videotable.NewTableNav(videotable.NewTable(chCols), circular, 3),
		vidNav:   videotable.NewTableNav(videotable.NewVideoTable(vidCols), circular, 3),
		drill:    newDrillState(circular),
		chCols:   chCols,
		vidCols:  vidCols,
		recent:   recentSearches{circular: circular},
	}
}

func (t Search) ID() tuipkg.TabID { return tuipkg.TabSearch }
func (t Search) Title() string    { return "Search" }
func (t Search) ShortHelp() []key.Binding {
	if t.input.Focused() {
		return nil
	}
	return []key.Binding{t.keys.Play, t.keys.Download, t.keys.CopyURL, t.keys.VideoInfo, t.keys.DrillDown}
}
func (t Search) InterceptsInput() bool { return t.input.Focused() }
func (t Search) SelectedVideo() (domain.Video, bool) {
	if t.drill.ch != nil {
		return t.drill.currentVideo()
	}
	if t.onVideos {
		return t.srchCurrentVideo()
	}
	return domain.Video{}, false
}

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
		t.chNav.SetTargetHeight(chH)
		t.vidNav.SetTargetHeight(vidH)
	} else {
		avail -= 2
		if avail < 1 {
			avail = 1
		}
		t.chNav.SetTargetHeight(avail)
		t.vidNav.SetTargetHeight(avail)
	}
}

func (t Search) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.input.SetWidth(m.Width - 12)
		t.drill.resize(m.Width, m.Height)
		t.chNav.SetWidth(m.Width)
		t.vidNav.SetWidth(m.Width)
		t.applyResultHeights()
		t.chNav.SetRows(videotable.BuildRows(t.channels, t.chCols))
		t.vidNav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.videos, t.aux), t.vidCols))
		return t, nil

	case tuipkg.SearchFocusInputMsg:
		if t.lastQuery == "" && !t.loading {
			t.recentMode = false
			t.histIdx = -1
			t.recent.cursor = 0
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
		t.chNav.SetRows(nil)
		t.chNav.GotoRow(0)
		t.vidNav.SetRows(nil)
		t.vidNav.GotoRow(0)
		t.recentMode = false
		t.histIdx = -1
		return t, tea.Batch(t.srchCmd(m.Query), t.spinner.Tick)

	case videotable.AuxDataMsg:
		t.aux = m

	case tuipkg.RefreshPositionsMsg:
		return t, videotable.LoadAuxDataCmd(t.backend)

	case srchRecentLoadedMsg:
		t.recent.queries = m.queries
		n := len(t.recent.queries)
		if t.recent.cursor >= n && t.recent.cursor > 0 {
			t.recent.cursor = n - 1
		}
		if t.histIdx >= n {
			t.histIdx = -1
		}
		return t, nil

	case spinner.TickMsg:
		if t.loading || t.drill.loading {
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
		t.chNav.SetRows(videotable.BuildRows(t.channels, t.chCols))
		t.chNav.GotoRow(0)
		t.vidNav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.videos, t.aux), t.vidCols))
		t.vidNav.GotoRow(0)
		t.drill.ch = nil
		t.drill.videos = nil
		t.onVideos = len(t.channels) == 0
		return t, func() tea.Msg { return tuipkg.HistoryChangedMsg{} }

	case srchChannelVideosMsg:
		t.drill.loading = false
		t.drill.setVideos(m.videos, t.aux)
		t.drill.nav.GotoRow(0)
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
	if t.drill.ch != nil {
		body = t.viewDrillDown(prompt, remaining)
	} else {
		body = t.viewResults(prompt, remaining)
	}
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, body))
}

func (t Search) srchHandleKeyInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	pageH := t.srchRecentPageHeight()
	switch msg.Code {
	case tea.KeyUp:
		n := len(t.recent.queries)
		if n > 0 && t.histIdx < n-1 {
			t.histIdx++
			t.input.SetValue(t.recent.queries[t.histIdx])
			t.input.CursorEnd()
			t.recent.cursor = t.histIdx
			t.recent.syncViewport(t.histIdx, pageH)
		}
		return t, nil

	case tea.KeyDown:
		if t.histIdx > 0 {
			t.histIdx--
			t.input.SetValue(t.recent.queries[t.histIdx])
			t.input.CursorEnd()
			t.recent.cursor = t.histIdx
			t.recent.syncViewport(t.histIdx, pageH)
		} else if t.histIdx == 0 {
			t.histIdx = -1
			t.recent.cursor = -1
			t.input.SetValue("")
		}
		return t, nil

	case tea.KeyEnter:
		query := strings.TrimSpace(t.input.Value())
		if query == "" {
			return t, nil
		}
		t.histIdx = -1
		t.recent.cursor = -1
		t.input.Blur()
		t.loading = true
		t.channels = nil
		t.videos = nil
		t.chNav.SetRows(nil)
		t.chNav.GotoRow(0)
		t.vidNav.SetRows(nil)
		t.vidNav.GotoRow(0)
		return t, tea.Batch(t.srchCmd(query), t.spinner.Tick)

	case tea.KeyEscape:
		t.input.Blur()
		if len(t.recent.queries) > 0 {
			t.recentMode = true
			if t.histIdx >= 0 {
				t.recent.cursor = t.histIdx
			} else {
				t.recent.cursor = 0
			}
			t.histIdx = -1
		}
		return t, nil

	default:
		if len(msg.Text) > 0 || msg.Code == tea.KeySpace || msg.Code == tea.KeyBackspace {
			t.histIdx = -1
			t.recent.cursor = -1
		}
		var cmd tea.Cmd
		t.input, cmd = t.input.Update(msg)
		return t, cmd
	}
}

func (t Search) srchHandleKeyRecentMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := len(t.recent.queries)
	pageH := t.srchRecentPageHeight()

	switch {
	case key.Matches(msg, keys.Up):
		t.recent.move(-1)
	case key.Matches(msg, keys.Down):
		t.recent.move(+1)
	case key.Matches(msg, keys.PageUp):
		t.recent.page(-1, pageH)
	case key.Matches(msg, keys.PageDown):
		t.recent.page(+1, pageH)
	case key.Matches(msg, keys.GotoBottom):
		t.recent.jumpTo(n - 1)
	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Play):
		c := t.recent.cursor
		if c >= 0 && c < n {
			return t.srchExecuteRecent(t.recent.queries[c])
		}
	case msg.Code == tea.KeyEnter:
		c := t.recent.cursor
		if c >= 0 && c < n {
			return t.srchExecuteRecent(t.recent.queries[c])
		}
	case key.Matches(msg, keys.Delete):
		c := t.recent.cursor
		if c >= 0 && c < n {
			query := t.recent.queries[c]
			t.recent.queries = append(t.recent.queries[:c], t.recent.queries[c+1:]...)
			if t.recent.cursor >= len(t.recent.queries) && t.recent.cursor > 0 {
				t.recent.cursor--
			}
			if len(t.recent.queries) == 0 {
				t.recentMode = false
				t.input.Focus()
				return t, tea.Batch(textinput.Blink, t.srchDeleteRecentCmd(query))
			}
			return t, t.srchDeleteRecentCmd(query)
		}
	case key.Matches(msg, keys.Escape), key.Matches(msg, keys.Filter):
		t.recentMode = false
		c := t.recent.cursor
		if c >= 0 && c < n {
			t.histIdx = c
			t.input.SetValue(t.recent.queries[c])
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
		t.chNav.SetRows(nil)
		t.chNav.GotoRow(0)
		t.vidNav.SetRows(nil)
		t.vidNav.GotoRow(0)
		t.drill.ch = nil
		t.drill.videos = nil
		t.histIdx = -1
		t.recent.cursor = 0
		t.recent.vs = 0
		t.input.SetValue("")
		t.input.Focus()
		return t, textinput.Blink
	}

	if t.drill.ch != nil {
		return t.srchHandleKeyDrill(msg)
	}

	if key.Matches(msg, keys.ToggleMode) {
		if len(t.channels) > 0 && len(t.videos) > 0 {
			t.onVideos = !t.onVideos
		}
		return t, nil
	}

	if t.onVideos {
		return t.srchHandleKeyVideos(msg)
	}
	return t.srchHandleKeyChannels(msg)
}

func (t Search) srchHandleKeyChannels(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := len(t.channels)

	if t.chNav.HandleNav(msg, keys, n) {
		return t, nil
	}

	idx := t.chNav.Index()
	switch {
	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right), msg.Code == tea.KeyEnter:
		if idx >= 0 && idx < n {
			ch := t.channels[idx]
			t.drill.ch = &ch
			t.drill.videos = nil
			t.drill.loading = true
			t.drill.nav.GotoRow(0)
			return t, tea.Batch(t.srchChannelVideosCmd(ch), t.spinner.Tick)
		}
	}
	return t, nil
}

func (t Search) srchHandleKeyVideos(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := len(t.videos)

	if t.vidNav.HandleNav(msg, keys, n) {
		return t, nil
	}

	switch {
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
	n := len(t.drill.videos)

	numBufBefore := t.drill.nav.NumBufView() != ""
	if t.drill.nav.HandleNav(msg, keys, n) {
		return t, nil
	}

	switch {
	case key.Matches(msg, keys.Left), key.Matches(msg, keys.Escape):
		if numBufBefore {
			t.drill.nav.ClearNumBuf()
			return t, nil
		}
		t.drill.ch = nil
		t.drill.videos = nil
	default:
		if v, ok := t.drill.currentVideo(); ok {
			if cmd, ok := HandleVideoAction(msg, v, keys); ok {
				return t, cmd
			}
		}
	}
	return t, nil
}

func (t Search) viewDrillDown(prompt string, _ int) string {
	header := styles.SectionTitle.Render("← " + render.Truncate(t.drill.ch.Name, t.width-4))
	if t.drill.loading {
		return lipgloss.JoinVertical(lipgloss.Left, prompt, header, t.spinner.View()+" Loading…")
	}
	if len(t.drill.videos) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, prompt, header, styles.Dim.Render("No videos found."))
	}
	parts := []string{prompt, header, t.drill.nav.View()}
	if s := t.drill.nav.NumBufView(); s != "" {
		parts = append(parts, s)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (t Search) viewResults(prompt string, remaining int) string {
	_ = remaining
	showRecent := (t.input.Focused() || t.recentMode) && t.lastQuery == "" && len(t.recent.queries) > 0

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
		parts = append(parts, t.srchPaneLabel("Channels", !t.onVideos), t.chNav.View())
	}
	if len(t.videos) > 0 {
		parts = append(parts, t.srchPaneLabel("Videos", t.onVideos), t.vidNav.View())
	}
	if s := t.chNav.NumBufView(); s != "" && !t.onVideos {
		parts = append(parts, s)
	}
	if s := t.vidNav.NumBufView(); s != "" && t.onVideos {
		parts = append(parts, s)
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
	pageH := t.srchRecentPageHeight()
	if pageH > height-1 {
		pageH = height - 1
	}
	start, end := t.recent.window(pageH)

	highlighted := -1
	if t.recentMode {
		highlighted = t.recent.cursor
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
		q := t.recent.queries[i]
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
	t.recent.cursor = 0
	t.loading = true
	t.channels = nil
	t.videos = nil
	t.chNav.SetRows(nil)
	t.chNav.GotoRow(0)
	t.vidNav.SetRows(nil)
	t.vidNav.GotoRow(0)
	t.input.SetValue(query)
	t.input.Blur()
	return t, tea.Batch(t.srchCmd(query), t.spinner.Tick)
}

func (t Search) srchCurrentVideo() (domain.Video, bool) {
	idx := t.vidNav.Index()
	if idx >= 0 && idx < len(t.videos) {
		return t.videos[idx], true
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
