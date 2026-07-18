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
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── tab-private messages ──────────────────────────────────────────────────────

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

// ── Search ────────────────────────────────────────────────────────────────────

// Search is the Search tab: a YouTube search with combined channel+video results,
// optional channel drill-down, and shell-history navigation of recent searches.
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

	cursor int // position in combined channels+videos list
	vs     int // viewport start for the videos sub-list only

	drillCh   *domain.Channel
	chVideos  []domain.Video
	chLoading bool
	vidCursor int
	vidVS     int

	spinner spinner.Model

	// recent searches
	recent      []string // ordered newest-first
	histIdx     int      // index into recent[] for Up/Down shell-history; -1 = not navigating
	recentCursor int     // cursor row when in recentMode
	recentVS    int     // viewport start when in recentMode
	recentMode  bool    // browsing recent list with keyboard (input blurred)
}

func NewSearch(backend api.Backend, keys keymap.KeyMap, circular bool) Search {
	ti := textinput.New()
	ti.Placeholder = "Search YouTube…"
	ti.CharLimit = 200
	ti.Focus()
	return Search{
		backend:  backend,
		keys:     keys,
		circular: circular,
		input:    ti,
		spinner:  spinner.New(),
		histIdx:  -1,
	}
}

// ── tui.Tab interface ─────────────────────────────────────────────────────────

func (t Search) ID() tuipkg.TabID          { return tuipkg.TabSearch }
func (t Search) Title() string             { return "Search" }
func (t Search) ShortHelp() []key.Binding { return nil }
func (t Search) InterceptsInput() bool    { return t.input.Focused() }

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (t Search) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, t.spinner.Tick, t.srchLoadRecentCmd())
}

func (t Search) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.input.Width = m.Width - 12
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
		t.cursor, t.vs = 0, 0
		t.recentMode = false
		t.histIdx = -1
		return t, tea.Batch(t.srchCmd(m.Query), t.spinner.Tick)

	case srchRecentLoadedMsg:
		t.recent = m.queries
		// clamp cursors in case list shrank
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
		t.cursor, t.vs = 0, 0
		t.drillCh = nil
		t.chVideos = nil
		return t, func() tea.Msg { return tuipkg.HistoryChangedMsg{} }

	case srchChannelVideosMsg:
		t.chLoading = false
		t.chVideos = m.videos
		t.vidCursor, t.vidVS = 0, 0
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

	// Forward ticks/blink to the text input when focused.
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

// ── key handling ──────────────────────────────────────────────────────────────

func (t Search) srchHandleKeyInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		// Shell-history: go back one entry.
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
		// Shell-history: go forward (toward current).
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
		t.cursor, t.vs = 0, 0
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
		// Any typing resets history navigation.
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
		// Return to input with the highlighted item pre-filled.
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

	// Clear results and return to recent searches.
	if key.Matches(msg, keys.Escape) && (len(t.channels) > 0 || len(t.videos) > 0) {
		t.lastQuery = ""
		t.channels = nil
		t.videos = nil
		t.cursor, t.vs = 0, 0
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

	nCh := len(t.channels)
	nVid := len(t.videos)
	total := nCh + nVid
	pageH := t.srchPageHeight()

	switch {
	case key.Matches(msg, keys.Up):
		if t.cursor > 0 {
			t.cursor--
			t.syncVS(nCh, nVid, pageH)
		}
	case key.Matches(msg, keys.Down):
		if t.cursor < total-1 {
			t.cursor++
			t.syncVS(nCh, nVid, pageH)
		}
	case key.Matches(msg, keys.PageUp):
		t.cursor -= pageH
		if t.cursor < 0 {
			t.cursor = 0
		}
		t.syncVS(nCh, nVid, pageH)
	case key.Matches(msg, keys.PageDown):
		t.cursor += pageH
		if t.cursor >= total {
			if total > 0 {
				t.cursor = total - 1
			} else {
				t.cursor = 0
			}
		}
		t.syncVS(nCh, nVid, pageH)
	case key.Matches(msg, keys.GotoBottom):
		if total > 0 {
			t.cursor = total - 1
			t.syncVS(nCh, nVid, pageH)
		}

	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
		if t.cursor < nCh {
			ch := t.channels[t.cursor]
			t.drillCh = &ch
			t.chVideos = nil
			t.chLoading = true
			t.vidCursor, t.vidVS = 0, 0
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
	pageH := t.srchPageHeight()

	switch {
	case key.Matches(msg, keys.Left), key.Matches(msg, keys.Escape):
		t.drillCh = nil
		t.chVideos = nil
	case key.Matches(msg, keys.Up):
		t.vidCursor, t.vidVS = nav.Move(t.vidCursor, t.vidVS, n, -1, pageH, t.circular)
	case key.Matches(msg, keys.Down):
		t.vidCursor, t.vidVS = nav.Move(t.vidCursor, t.vidVS, n, +1, pageH, t.circular)
	case key.Matches(msg, keys.PageUp):
		t.vidCursor, t.vidVS = nav.Page(t.vidCursor, t.vidVS, n, -1, pageH, t.circular)
	case key.Matches(msg, keys.PageDown):
		t.vidCursor, t.vidVS = nav.Page(t.vidCursor, t.vidVS, n, +1, pageH, t.circular)
	case key.Matches(msg, keys.GotoBottom):
		t.vidCursor, t.vidVS = nav.Jump(n-1, n, pageH)
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

// ── rendering ─────────────────────────────────────────────────────────────────

func (t Search) viewDrillDown(prompt string, remaining int) string {
	header := styles.SectionTitle.Render("← " + render.Truncate(t.drillCh.Name, t.width-4))
	headerH := lipgloss.Height(header)

	if t.chLoading {
		return lipgloss.JoinVertical(lipgloss.Left, prompt, header,
			t.spinner.View()+" Loading…")
	}
	if len(t.chVideos) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, prompt, header,
			styles.Dim.Render("No videos found."))
	}

	ctx := VideoListCtx{Width: t.width, ShowChannel: false}
	body := renderVideoRows(ctx, t.chVideos, t.vidCursor, t.vidVS, remaining-headerH)
	return lipgloss.JoinVertical(lipgloss.Left, prompt, header, body)
}

func (t Search) viewResults(prompt string, remaining int) string {
	// Show recent searches when there are no results and input/browse mode is active.
	showRecent := (t.input.Focused() || t.recentMode) && t.lastQuery == "" && len(t.recent) > 0

	if t.loading {
		return lipgloss.JoinVertical(lipgloss.Left, prompt,
			t.spinner.View()+" Searching…")
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
		return lipgloss.JoinVertical(lipgloss.Left, prompt,
			styles.Dim.PaddingLeft(1).Render(hint))
	}

	header := styles.SectionTitle.Render("Results for: " + t.lastQuery)
	headerH := lipgloss.Height(header)
	listH := remaining - headerH

	nCh := len(t.channels)
	nVid := len(t.videos)
	var rows []string
	usedH := 0

	if nCh > 0 {
		rows = append(rows, styles.Dim.PaddingLeft(1).Render("Channels"))
		usedH++
		nameW := t.width - render.ColNum - 1 - 4
		if nameW < 10 {
			nameW = 10
		}
		for i, ch := range t.channels {
			numStyle := styles.RowNum
			nameStyle := styles.Normal.Width(nameW)
			indicator := "  "
			sep := " "
			if i == t.cursor {
				indicator = styles.Selected.Render("▶ ")
				numStyle = numStyle.Background(styles.ColorBgSelect)
				sep = lipgloss.NewStyle().Background(styles.ColorBgSelect).Render(" ")
				nameStyle = styles.Selected.Width(nameW)
			}
			numStr := numStyle.Render(fmt.Sprintf("%*d", render.ColNum, i+1))
			name := ch.DisplayName()
			rows = append(rows, numStr+sep+indicator+nameStyle.Render(render.Truncate(name, nameW)))
			usedH++
		}
	}

	if nVid > 0 {
		if nCh > 0 {
			rows = append(rows, styles.Dim.PaddingLeft(1).Render("Videos"))
			usedH++
		}
		ctx := VideoListCtx{Width: t.width, ShowChannel: true}
		vidCursor := t.cursor - nCh
		rows = append(rows, renderVideoRows(ctx, t.videos, vidCursor, t.vs, listH-usedH))
	}

	return lipgloss.JoinVertical(lipgloss.Left, prompt, header, strings.Join(rows, "\n"))
}

func (t Search) viewRecentSearches(height int) string {
	n := len(t.recent)
	pageH := t.srchRecentPageHeight()
	if pageH > height-1 {
		pageH = height - 1
	}
	start, end := nav.Window(t.recentVS, n, pageH)

	// Which row is highlighted.
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

// ── background commands ───────────────────────────────────────────────────────

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
	t.cursor, t.vs = 0, 0
	t.input.SetValue(query)
	t.input.Blur()
	return t, tea.Batch(t.srchCmd(query), t.spinner.Tick)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (t Search) srchCurrentVideo() (domain.Video, bool) {
	nCh := len(t.channels)
	idx := t.cursor - nCh
	if idx >= 0 && idx < len(t.videos) {
		return t.videos[idx], true
	}
	return domain.Video{}, false
}

func (t Search) srchCurrentDrillVideo() (domain.Video, bool) {
	if t.vidCursor >= 0 && t.vidCursor < len(t.chVideos) {
		return t.chVideos[t.vidCursor], true
	}
	return domain.Video{}, false
}

// syncVS keeps vs in sync after cursor moves in the combined list.
// vs only applies to the videos sub-list; channels are always fully visible.
func (t *Search) syncVS(nCh, nVid, pageH int) {
	if t.cursor >= nCh && nVid > 0 {
		vidCursor := t.cursor - nCh
		_, t.vs = nav.Move(vidCursor, t.vs, nVid, 0, pageH, false)
	} else {
		t.vs = 0
	}
}

func (t Search) srchPageHeight() int {
	h := t.height - 5 // section title (2) + prompt (1) + results header (2)
	if h < 1 {
		h = 1
	}
	return h
}

func (t Search) srchRecentPageHeight() int {
	h := t.height - 4 // section title (2) + prompt (1) + "Recent searches" header (1)
	if h < 1 {
		h = 1
	}
	return h
}
