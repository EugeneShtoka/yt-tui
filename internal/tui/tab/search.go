package tab

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
)

type srchResultMsg struct {
	tuipkg.TabTarget
	query    string
	channels []domain.Channel
	videos   []domain.Video
	err      error
}
type srchChannelVideosMsg struct {
	tuipkg.TabTarget
	channelID string
	videos    []domain.Video
	err       error
}
type srchRecentLoadedMsg struct {
	tuipkg.TabTarget
	queries []string
}

// ── recentSearches sub-model ─────────────────────────────────────────────────

// recentSearches is the Search tab's recent-query list: the queries plus an
// embedded listCursor that owns the scroll/cursor math. The wrapper methods below
// forward the current item count (len(queries)) to the cursor so existing call
// sites keep passing only pageH.
type recentSearches struct {
	listCursor
	queries []string
}

func (r *recentSearches) move(delta, pageH int) { r.listCursor.move(delta, len(r.queries), pageH) }
func (r *recentSearches) page(direction, pageH int) {
	r.listCursor.page(direction, len(r.queries), pageH)
}
func (r *recentSearches) jumpTo(idx, pageH int) { r.listCursor.jumpTo(idx, len(r.queries), pageH) }
func (r *recentSearches) syncViewport(c, pageH int) {
	r.listCursor.syncViewport(c, len(r.queries), pageH)
}
func (r *recentSearches) window(pageH int) (start, end int) {
	return r.listCursor.window(len(r.queries), pageH)
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
		videotable.WatchedCol[videotable.VideoData](), videotable.DurationCol[videotable.VideoData](), videotable.ViewsCol[videotable.VideoData](), videotable.DateCol[videotable.VideoData](),
	}
	return drillState{
		nav:  videotable.NewTableNav(cols, circular, 5),
		cols: cols,
	}
}

func (d *drillState) resize(w, h int) {
	d.nav.Resize(w, h)
}

func (d *drillState) setVideos(videos []domain.Video, aux videotable.AuxData) {
	d.videos = videos
	videotable.SetVideoRows(&d.nav, videos, aux, d.cols)
}

func (d *drillState) currentVideo() (domain.Video, bool) {
	idx := d.nav.Index()
	if idx >= 0 && idx < len(d.videos) {
		return d.videos[idx], true
	}
	return domain.Video{}, false
}

// ── Search model ─────────────────────────────────────────────────────────────

// searchBackend is the narrow slice of the backend the Search tab needs: channel/
// video search, search-history, and the shared aux data. Declared consumer-side (ISP).
type searchBackend interface {
	api.ChannelBackend
	api.HistoryBackend
	videotable.AuxBackend
}

type Search struct {
	ctx      context.Context
	backend  searchBackend
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

	spinnerFrame string

	recent     recentSearches
	histIdx    int
	recentMode bool
}

// searchVideoColumns is the full, natural-order column set for the video-results
// list — the configurable one exposed to per-panel column config and the
// tab.PanelColumnKeys catalog. The channel-results list stays fixed. Extracted
// so config and the catalog share one source of truth.
func searchVideoColumns() []videotable.ColumnDef[videotable.VideoData] {
	return []videotable.ColumnDef[videotable.VideoData]{
		videotable.NumCol[videotable.VideoData](), videotable.IndicatorCol[videotable.VideoData](), videotable.TitleFlexCol[videotable.VideoData](),
		videotable.ChannelCol[videotable.VideoData](), videotable.WatchedCol[videotable.VideoData](), videotable.DurationCol[videotable.VideoData](), videotable.ViewsCol[videotable.VideoData](), videotable.DateCol[videotable.VideoData](),
	}
}

func NewSearch(ctx context.Context, backend searchBackend, keys keymap.KeyMap, circular bool, wantCols ...string) Search {
	ti := textinput.New()
	ti.Placeholder = "Search YouTube…"
	ti.CharLimit = 200
	ti.Focus()

	chCols := []videotable.ColumnDef[domain.Channel]{
		videotable.NumCol[domain.Channel](),
		videotable.BlankIndicatorCol[domain.Channel](),
		videotable.TitleFlexCol[domain.Channel](),
	}
	vidCols := videotable.SelectColumns(searchVideoColumns(), wantCols)
	return Search{
		ctx:      ctx,
		backend:  backend,
		keys:     keys,
		circular: circular,
		input:    ti,
		histIdx:  -1,
		chNav:    videotable.NewTableNav(chCols, circular, 3),
		vidNav:   videotable.NewTableNav(vidCols, circular, 3),
		drill:    newDrillState(circular),
		chCols:   chCols,
		vidCols:  vidCols,
		recent:   recentSearches{listCursor: listCursor{circular: circular}},
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
func (t Search) Loading() bool { return t.loading || t.drill.loading }

func (t Search) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, t.srchLoadRecentCmd(), videotable.LoadAuxDataCmd(t.ctx, t.backend))
}

// chanPaneMaxFraction caps the channels pane to 1/N of the available split
// height, leaving the rest for the (usually longer) video results.
const chanPaneMaxFraction = 3

func (t *Search) applyResultHeights() {
	nCh := len(t.channels)
	nVid := len(t.videos)
	avail := t.height - 3 // header + prompt + status line
	if nCh > 0 && nVid > 0 {
		avail -= 4 // two pane labels + separators in the split layout
		chH := nCh
		if chH > avail/chanPaneMaxFraction {
			chH = avail / chanPaneMaxFraction
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
		avail -= 3
		if avail < 1 {
			avail = 1
		}
		t.chNav.SetTargetHeight(avail)
		t.vidNav.SetTargetHeight(avail)
	}
}

// resetResults clears the channel/video result panes, e.g. before issuing a
// new query. Consolidates 4 copy-pasted reset blocks (M-15).
func (t *Search) resetResults() {
	t.channels = nil
	t.videos = nil
	t.chNav.SetRows(nil)
	t.chNav.GotoRow(0)
	t.vidNav.SetRows(nil)
	t.vidNav.GotoRow(0)
}

func (t Search) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		return t.onContentSize(m)

	case tuipkg.SearchFocusInputMsg:
		return t.onFocusInput()

	case tuipkg.HistoryChangedMsg:
		return t, t.srchLoadRecentCmd()

	case tuipkg.SearchActivateMsg:
		return t.onSearchActivate(m)

	case tuipkg.SpinnerFrameMsg:
		t.spinnerFrame = m.Frame

	case videotable.AuxDataMsg:
		t.aux = m
		t.setVidNavRows()
		if t.drill.ch != nil {
			t.drill.setVideos(t.drill.videos, t.aux)
		}

	case srchRecentLoadedMsg:
		return t.onRecentLoaded(m)

	case srchResultMsg:
		return t.onSearchResult(m)

	case srchChannelVideosMsg:
		return t.onChannelVideos(m)

	case tea.KeyPressMsg:
		return t.routeKey(m)
	}

	if t.input.Focused() {
		var cmd tea.Cmd
		t.input, cmd = t.input.Update(msg)
		return t, cmd
	}
	return t, nil
}

// setVidNavRows rebuilds the results video table from the enriched slice.
func (t *Search) setVidNavRows() {
	videotable.SetVideoRows(&t.vidNav, t.videos, t.aux, t.vidCols)
}

func (t Search) onContentSize(m tuipkg.ContentSizeMsg) (tea.Model, tea.Cmd) {
	t.width, t.height = m.Width, m.Height
	t.input.SetWidth(m.Width - 12)
	t.drill.resize(m.Width, m.Height)
	t.chNav.SetWidth(m.Width)
	t.vidNav.SetWidth(m.Width)
	t.applyResultHeights()
	t.chNav.SetRows(videotable.BuildRows(t.channels, t.chCols))
	t.setVidNavRows()
	return t, nil
}

func (t Search) onSearchActivate(m tuipkg.SearchActivateMsg) (tea.Model, tea.Cmd) {
	t.input.SetValue(m.Query)
	t.input.Blur()
	t.loading = true
	t.resetResults()
	t.recentMode = false
	t.histIdx = -1
	return t, t.srchCmd(m.Query)
}

// routeKey dispatches a key press to the active input/recent/result handler.
func (t Search) routeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if t.recentMode {
		return t.srchHandleKeyRecentMode(msg)
	}
	if t.input.Focused() {
		return t.srchHandleKeyInput(msg)
	}
	return t.srchHandleKey(msg)
}

func (t Search) onFocusInput() (tea.Model, tea.Cmd) {
	if t.lastQuery == "" && !t.loading {
		t.recentMode = false
		t.histIdx = -1
		t.recent.cursor = 0
		t.input.SetValue("")
		t.input.Focus()
		return t, textinput.Blink
	}
	return t, nil
}

func (t Search) onRecentLoaded(m srchRecentLoadedMsg) (tea.Model, tea.Cmd) {
	t.recent.queries = m.queries
	n := len(t.recent.queries)
	if t.recent.cursor >= n && t.recent.cursor > 0 {
		t.recent.cursor = n - 1
	}
	if t.histIdx >= n {
		t.histIdx = -1
	}
	return t, nil
}

func (t Search) onSearchResult(m srchResultMsg) (tea.Model, tea.Cmd) {
	t.loading = false
	if m.err != nil {
		return t, errMsg("search: " + m.err.Error())
	}
	t.lastQuery = m.query
	t.channels = m.channels
	t.videos = m.videos
	t.applyResultHeights()
	t.chNav.SetRows(videotable.BuildRows(t.channels, t.chCols))
	t.chNav.GotoRow(0)
	t.setVidNavRows()
	t.vidNav.GotoRow(0)
	t.drill.ch = nil
	t.drill.videos = nil
	t.onVideos = len(t.channels) == 0
	return t, func() tea.Msg { return tuipkg.HistoryChangedMsg{} }
}

func (t Search) onChannelVideos(m srchChannelVideosMsg) (tea.Model, tea.Cmd) {
	if t.drill.ch == nil || t.drill.ch.ID != m.channelID {
		return t, nil
	}
	t.drill.loading = false
	if m.err != nil {
		return t, errMsg("channel videos: " + m.err.Error())
	}
	t.drill.setVideos(m.videos, t.aux)
	t.drill.nav.GotoRow(0)
	return t, nil
}

func (t Search) srchHandleKeyInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.Code {
	case tea.KeyUp:
		t.inputHistoryUp()
		return t, nil

	case tea.KeyDown:
		t.inputHistoryDown()
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
		t.resetResults()
		return t, t.srchCmd(query)

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

// inputHistoryUp walks the recent-query history toward older entries, mirroring
// the selection into the input field and the recent-list viewport.
func (t *Search) inputHistoryUp() {
	pageH := t.srchRecentPageHeight()
	n := len(t.recent.queries)
	if n > 0 && t.histIdx < n-1 {
		t.histIdx++
		t.input.SetValue(t.recent.queries[t.histIdx])
		t.input.CursorEnd()
		t.recent.cursor = t.histIdx
		t.recent.syncViewport(t.histIdx, pageH)
	}
}

// inputHistoryDown walks the recent-query history toward newer entries, clearing
// the input once it steps past the most-recent entry.
func (t *Search) inputHistoryDown() {
	pageH := t.srchRecentPageHeight()
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
}

func (t Search) srchHandleKeyRecentMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := len(t.recent.queries)
	pageH := t.srchRecentPageHeight()

	switch {
	case key.Matches(msg, keys.Up):
		t.recent.move(-1, pageH)
	case key.Matches(msg, keys.Down):
		t.recent.move(+1, pageH)
	case key.Matches(msg, keys.PageUp):
		t.recent.page(-1, pageH)
	case key.Matches(msg, keys.PageDown):
		t.recent.page(+1, pageH)
	case key.Matches(msg, keys.GotoBottom):
		t.recent.jumpTo(n-1, pageH)
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
		return t.recentDeleteCurrent(n)
	case key.Matches(msg, keys.Escape), key.Matches(msg, keys.Filter):
		return t.recentExitToInput(n)
	}
	return t, nil
}

// recentDeleteCurrent removes the highlighted recent query, refocusing the input
// once the list empties. n is the query count captured before mutation.
func (t Search) recentDeleteCurrent(n int) (tea.Model, tea.Cmd) {
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
	return t, nil
}

// recentExitToInput leaves recent-list mode, seeding the input with the
// highlighted query. n is the query count captured before mutation.
func (t Search) recentExitToInput(n int) (tea.Model, tea.Cmd) {
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

func (t Search) srchHandleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys

	if key.Matches(msg, keys.Filter) {
		t.input.Focus()
		return t, textinput.Blink
	}

	if key.Matches(msg, keys.Escape) && (len(t.channels) > 0 || len(t.videos) > 0) {
		t.lastQuery = ""
		t.resetResults()
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
			return t, t.srchChannelVideosCmd(ch)
		}
	}
	return t, nil
}

func (t Search) srchHandleKeyVideos(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	if t.vidNav.HandleNav(msg, keys, len(t.videos)) {
		return t, nil
	}
	v, ok := t.srchCurrentVideo()
	if !ok {
		return t, nil
	}
	if key.Matches(msg, keys.DrillDown) || key.Matches(msg, keys.Right) {
		return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v} }
	}
	if cmd, ok := HandleVideoAction(msg, v, keys); ok {
		return t, cmd
	}
	return t, nil
}

func (t Search) srchHandleKeyDrill(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := len(t.drill.videos)

	if handled, back := handleDrillBackKey(&t.drill.nav, msg, keys, n); handled {
		if back {
			t.drill.ch = nil
			t.drill.videos = nil
		}
		return t, nil
	}

	if v, ok := t.drill.currentVideo(); ok {
		if cmd, ok := HandleVideoAction(msg, v, keys); ok {
			return t, cmd
		}
	}
	return t, nil
}

func (t Search) srchExecuteRecent(query string) (tea.Model, tea.Cmd) {
	t.recentMode = false
	t.histIdx = -1
	t.recent.cursor = 0
	t.loading = true
	t.resetResults()
	t.input.SetValue(query)
	t.input.Blur()
	return t, t.srchCmd(query)
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
