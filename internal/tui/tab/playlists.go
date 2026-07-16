package tab

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
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

const ytWatchLaterID = "WL"

// plCreateStage tracks the inline playlist-creation flow.
type plCreateStage int

const (
	plCreateNone        plCreateStage = iota
	plCreateTypeSelect                // picking local vs YT
	plCreateNameInput                 // typing playlist name
)

// ── tab-private messages ──────────────────────────────────────────────────────

type plLocalLoadedMsg struct{ playlists []domain.Playlist }

type plYTLoadedMsg struct {
	playlists  []domain.YTPlaylist
	err        error
	background bool
}

type plVideosLoadedMsg struct {
	playlistID string
	videos     []domain.Video
	err        error
}

type plYTCreatedMsg struct {
	name string
	id   string
	err  error
}

type plLocalCreatedMsg struct {
	name string
	id   int64
	err  error
}

type plDeletedMsg struct{ err error }
type plRemovedMsg struct{ err error }

// ── Playlists ─────────────────────────────────────────────────────────────────

// Playlists is the Playlists tab: two panes — playlist list and per-playlist video list.
type Playlists struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	width, height int

	localPlaylists []domain.Playlist
	ytPlaylists    []domain.YTPlaylist
	ytPlLoading    bool
	ytPlLoaded     bool

	vidCache   map[string][]domain.Video
	vidLoading bool

	cursor    int
	vs        int
	vidCursor int
	vidVS     int
	pane      int // 0 = playlist list, 1 = video list
	vidSort   int // feed.Sort* constant

	createStage   plCreateStage
	createTypeSel int // 0 = local, 1 = YT
	createModeYT  bool
	createInput   textinput.Model

	spinner spinner.Model
}

func NewPlaylists(backend api.Backend, keys keymap.KeyMap, circular bool) Playlists {
	ti := textinput.New()
	ti.Placeholder = "Playlist name…"
	return Playlists{
		backend:     backend,
		keys:        keys,
		circular:    circular,
		vidCache:    make(map[string][]domain.Video),
		spinner:     spinner.New(),
		createInput: ti,
	}
}

// ── tui.Tab interface ─────────────────────────────────────────────────────────

func (t Playlists) ID() tuipkg.TabID          { return tuipkg.TabPlaylists }
func (t Playlists) Title() string             { return "Playlists" }
func (t Playlists) ShortHelp() []key.Binding { return nil }
func (t Playlists) InterceptsInput() bool     { return t.createStage == plCreateNameInput }

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (t Playlists) Init() tea.Cmd {
	return tea.Batch(t.localLoadCmd(), t.ytLoadCmd(false), t.spinner.Tick)
}

func (t Playlists) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height

	case spinner.TickMsg:
		if t.ytPlLoading || t.vidLoading {
			var cmd tea.Cmd
			t.spinner, cmd = t.spinner.Update(m)
			return t, cmd
		}

	case plLocalLoadedMsg:
		t.localPlaylists = m.playlists

	case plYTLoadedMsg:
		t.ytPlLoading = false
		if m.err != nil {
			if !m.background {
				return t, errMsg("playlists: " + m.err.Error())
			}
			return t, nil
		}
		t.ytPlLoaded = true
		if ytPlaylistSetChanged(t.ytPlaylists, m.playlists) {
			t.ytPlaylists = m.playlists
			t.vidCache = make(map[string][]domain.Video)
		}

	case plVideosLoadedMsg:
		t.vidLoading = false
		if m.err != nil {
			if len(t.vidCache[m.playlistID]) == 0 {
				return t, errMsg("playlist: " + m.err.Error())
			}
			return t, nil
		}
		vids := m.videos
		feed.SortVideos(vids, t.vidSort)
		t.vidCache[m.playlistID] = vids

	case plYTCreatedMsg:
		if m.err != nil {
			return t, errMsg("create playlist: " + m.err.Error())
		}
		t.ytPlaylists = append(t.ytPlaylists, domain.YTPlaylist{ID: m.id, Title: m.name})
		return t, statusMsg("Created playlist: " + m.name)

	case plLocalCreatedMsg:
		if m.err != nil {
			return t, errMsg("create playlist: " + m.err.Error())
		}
		return t, tea.Batch(
			t.localLoadCmd(),
			statusMsg("Created playlist: "+m.name),
		)

	case plDeletedMsg:
		if m.err != nil {
			return t, errMsg("delete playlist: " + m.err.Error())
		}

	case plRemovedMsg:
		if m.err != nil {
			return t, errMsg("remove from playlist: " + m.err.Error())
		}

	case tuipkg.NavigateToPlaylistMsg:
		t.scrollToPlaylist(m)

	case tea.KeyMsg:
		return t.handleKey(m)
	}
	return t, nil
}

func (t Playlists) View() string {
	header := styles.SectionTitle.Render("Playlists")
	headerH := lipgloss.Height(header)
	bodyH := t.height - headerH

	switch t.createStage {
	case plCreateTypeSelect:
		opt0, opt1 := "  Local playlist", "  YouTube playlist"
		if t.createTypeSel == 0 {
			opt0 = styles.Selected.Render("▶ Local playlist")
		} else {
			opt1 = styles.Selected.Render("▶ YouTube playlist")
		}
		prompt := styles.Bold.Render("New playlist: ") + "\n" + opt0 + "\n" + opt1
		body := t.renderPlaylistRows(bodyH-3) + "\n\n\n"
		return lipgloss.JoinVertical(lipgloss.Left, header, body+prompt)

	case plCreateNameInput:
		label := "New local playlist: "
		if t.createModeYT {
			label = "New YouTube playlist: "
		}
		prompt := styles.Bold.Render(label) + t.createInput.View()
		body := t.renderPlaylistRows(bodyH-2) + "\n\n"
		return lipgloss.JoinVertical(lipgloss.Left, header, body+prompt)
	}

	if t.pane == 1 && t.cursor < t.plCount() {
		plName := t.selectedPlaylistName()
		subHeader := styles.SectionTitle.Render("← " + plName)
		subH := lipgloss.Height(subHeader)

		plKey := t.selectedPlaylistKey()
		vids := t.vidCache[plKey]
		var body string
		switch {
		case len(vids) > 0:
			ctx := VideoListCtx{Width: t.width}
			body = renderVideoRows(ctx, vids, t.vidCursor, t.vidVS, bodyH-subH)
		case t.vidLoading:
			body = t.spinner.View() + " Loading from YouTube…"
		default:
			body = styles.Dim.Render("Empty playlist.")
		}
		return lipgloss.JoinVertical(lipgloss.Left, header, subHeader, body)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, t.renderPlaylistRows(bodyH))
}

// ── key handling ──────────────────────────────────────────────────────────────

func (t Playlists) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch t.createStage {
	case plCreateTypeSelect:
		return t.handleTypeSelect(msg)
	case plCreateNameInput:
		return t.handleNameInput(msg)
	}
	if t.pane == 1 {
		return t.handleVideoPaneKey(msg)
	}
	return t.handleListPaneKey(msg)
}

func (t Playlists) handleListPaneKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := t.plCount()
	keys := t.keys
	pageH := t.pageHeight()

	switch {
	case key.Matches(msg, keys.Up):
		t.cursor, t.vs = nav.Move(t.cursor, t.vs, n, -1, pageH, t.circular)

	case key.Matches(msg, keys.Down):
		t.cursor, t.vs = nav.Move(t.cursor, t.vs, n, +1, pageH, t.circular)

	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
		if t.cursor < n {
			plKey := t.selectedPlaylistKey()
			t.pane = 1
			t.vidCursor, t.vidVS = 0, 0
			if t.ytPlLoaded && t.cursor < len(t.ytPlaylists) {
				if _, ok := t.vidCache[plKey]; !ok {
					t.vidLoading = true
				}
				return t, t.ytVideosCmd(plKey)
			}
			localID := plLocalID(plKey)
			return t, t.localVideosCmd(localID)
		}

	case key.Matches(msg, keys.NewList):
		if t.ytPlLoaded {
			t.createTypeSel = 0
			t.createStage = plCreateTypeSelect
		} else {
			t.createModeYT = false
			t.createInput.SetValue("")
			t.createInput.Focus()
			t.createStage = plCreateNameInput
			return t, textinput.Blink
		}

	case key.Matches(msg, keys.Refresh):
		t.ytPlLoading = true
		return t, t.ytLoadCmd(true)

	case key.Matches(msg, keys.Delete):
		return t.deleteSelected()
	}
	return t, nil
}

func (t Playlists) handleVideoPaneKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if t.cursor >= t.plCount() {
		t.pane = 0
		return t, nil
	}
	keys := t.keys
	plKey := t.selectedPlaylistKey()
	vids := t.vidCache[plKey]
	n := len(vids)
	pageH := t.pageHeight()

	switch {
	case key.Matches(msg, keys.Left), key.Matches(msg, keys.Escape):
		t.pane = 0

	case key.Matches(msg, keys.Up):
		t.vidCursor, t.vidVS = nav.Move(t.vidCursor, t.vidVS, n, -1, pageH, t.circular)

	case key.Matches(msg, keys.Down):
		t.vidCursor, t.vidVS = nav.Move(t.vidCursor, t.vidVS, n, +1, pageH, t.circular)

	case key.Matches(msg, keys.PageUp):
		t.vidCursor, t.vidVS = nav.Page(t.vidCursor, t.vidVS, n, -1, pageH, t.circular)

	case key.Matches(msg, keys.PageDown):
		t.vidCursor, t.vidVS = nav.Page(t.vidCursor, t.vidVS, n, +1, pageH, t.circular)

	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Play):
		if t.vidCursor < len(vids) {
			v := vids[t.vidCursor]
			return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v} }
		}

	case key.Matches(msg, keys.PlayAudio):
		if t.vidCursor < len(vids) {
			v := vids[t.vidCursor]
			return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v, AudioOnly: true} }
		}

	case key.Matches(msg, keys.Download):
		if t.vidCursor < len(vids) {
			v := vids[t.vidCursor]
			return t, func() tea.Msg { return tuipkg.EnqueueMsg{Video: v} }
		}

	case key.Matches(msg, keys.DownloadAudio):
		if t.vidCursor < len(vids) {
			v := vids[t.vidCursor]
			return t, func() tea.Msg { return tuipkg.EnqueueMsg{Video: v, AudioOnly: true} }
		}

	case key.Matches(msg, keys.CopyURL):
		if t.vidCursor < len(vids) {
			url := vids[t.vidCursor].URL
			return t, func() tea.Msg { return tuipkg.CopyURLMsg{URL: url} }
		}

	case key.Matches(msg, keys.VideoInfo):
		if t.vidCursor < len(vids) {
			v := vids[t.vidCursor]
			return t, func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: "video_detail", Video: v} }
		}

	case key.Matches(msg, keys.AddList):
		if t.vidCursor < len(vids) {
			v := vids[t.vidCursor]
			return t, func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: "add_to_playlist", Video: v} }
		}

	case key.Matches(msg, keys.Delete):
		return t.removeCurrentVideo(plKey, vids)
	}
	return t, nil
}

func (t Playlists) handleTypeSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	switch {
	case key.Matches(msg, keys.Up), key.Matches(msg, keys.Down):
		if t.createTypeSel == 0 {
			t.createTypeSel = 1
		} else {
			t.createTypeSel = 0
		}
	case key.Matches(msg, keys.DrillDown):
		t.createModeYT = t.createTypeSel == 1
		t.createInput.SetValue("")
		t.createInput.Focus()
		t.createStage = plCreateNameInput
		return t, textinput.Blink
	case key.Matches(msg, keys.Escape):
		t.createStage = plCreateNone
	}
	return t, nil
}

func (t Playlists) handleNameInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	switch {
	case key.Matches(msg, keys.DrillDown):
		name := strings.TrimSpace(t.createInput.Value())
		isYT := t.createModeYT
		t.createInput.Blur()
		t.createStage = plCreateNone
		t.createModeYT = false
		if name == "" {
			return t, nil
		}
		if isYT {
			return t, t.createYTPlaylistCmd(name)
		}
		return t, t.createLocalPlaylistCmd(name)
	case key.Matches(msg, keys.Escape):
		t.createInput.Blur()
		t.createStage = plCreateNone
		t.createModeYT = false
	default:
		var cmd tea.Cmd
		t.createInput, cmd = t.createInput.Update(msg)
		return t, cmd
	}
	return t, nil
}

// ── action helpers ────────────────────────────────────────────────────────────

func (t Playlists) deleteSelected() (Playlists, tea.Cmd) {
	n := t.plCount()
	if t.cursor >= n {
		return t, nil
	}
	plKey := t.selectedPlaylistKey()
	if plKey == ytWatchLaterID {
		return t, func() tea.Msg { return tuipkg.StatusMsg{Text: "Cannot delete Watch Later", IsErr: true} }
	}
	cursor := t.cursor
	if t.ytPlLoaded && cursor < len(t.ytPlaylists) {
		pl := t.ytPlaylists[cursor]
		delete(t.vidCache, pl.ID)
		t.ytPlaylists = append(t.ytPlaylists[:cursor], t.ytPlaylists[cursor+1:]...)
		t.cursor, t.vs = nav.Move(plClamp(cursor, t.plCount()), t.vs, t.plCount(), 0, t.pageHeight(), false)
		id := pl.ID
		return t, func() tea.Msg {
			return plDeletedMsg{err: t.backend.DeleteYTPlaylist(context.Background(), id)}
		}
	}
	localIdx := cursor
	if t.ytPlLoaded {
		localIdx -= len(t.ytPlaylists)
	}
	if localIdx < 0 || localIdx >= len(t.localPlaylists) {
		return t, nil
	}
	pl := t.localPlaylists[localIdx]
	delete(t.vidCache, fmt.Sprintf("local:%d", pl.ID))
	t.localPlaylists = append(t.localPlaylists[:localIdx], t.localPlaylists[localIdx+1:]...)
	t.cursor, t.vs = nav.Move(plClamp(cursor, t.plCount()), t.vs, t.plCount(), 0, t.pageHeight(), false)
	id := pl.ID
	return t, func() tea.Msg {
		return plDeletedMsg{err: t.backend.DeletePlaylist(context.Background(), id)}
	}
}

func (t Playlists) removeCurrentVideo(plKey string, vids []domain.Video) (Playlists, tea.Cmd) {
	if t.vidCursor >= len(vids) {
		return t, nil
	}
	vid := vids[t.vidCursor]
	updated := make([]domain.Video, 0, len(vids)-1)
	for _, v := range vids {
		if v.ID != vid.ID {
			updated = append(updated, v)
		}
	}
	t.vidCache[plKey] = updated
	t.vidCursor, t.vidVS = nav.Move(plClamp(t.vidCursor, len(updated)), t.vidVS, len(updated), 0, t.pageHeight(), false)

	vidID := vid.ID
	if localID := plLocalID(plKey); localID != 0 {
		return t, func() tea.Msg {
			return plRemovedMsg{err: t.backend.RemoveFromPlaylist(context.Background(), localID, vidID)}
		}
	}
	return t, func() tea.Msg {
		return plRemovedMsg{err: t.backend.RemoveFromYTPlaylist(context.Background(), plKey, vidID)}
	}
}

func (t *Playlists) scrollToPlaylist(m tuipkg.NavigateToPlaylistMsg) {
	if m.PlaylistLocalID != 0 {
		offset := 0
		if t.ytPlLoaded {
			offset = len(t.ytPlaylists)
		}
		for i, pl := range t.localPlaylists {
			if pl.ID == m.PlaylistLocalID {
				t.cursor = offset + i
				t.pane = 1
				return
			}
		}
		return
	}
	if m.PlaylistID != "" && t.ytPlLoaded {
		for i, pl := range t.ytPlaylists {
			if pl.ID == m.PlaylistID {
				t.cursor = i
				t.pane = 1
				return
			}
		}
	}
}

// ── background commands ───────────────────────────────────────────────────────

func (t Playlists) localLoadCmd() tea.Cmd {
	return func() tea.Msg {
		pls, err := t.backend.LocalPlaylists(context.Background())
		if err != nil {
			return tuipkg.StatusMsg{Text: "local playlists: " + err.Error(), IsErr: true}
		}
		return plLocalLoadedMsg{playlists: pls}
	}
}

func (t Playlists) ytLoadCmd(background bool) tea.Cmd {
	t.ytPlLoading = true
	return func() tea.Msg {
		pls, err := t.backend.YTPlaylists(context.Background())
		return plYTLoadedMsg{playlists: pls, err: err, background: background}
	}
}

func (t Playlists) ytVideosCmd(playlistID string) tea.Cmd {
	return func() tea.Msg {
		vids, err := t.backend.YTPlaylistVideos(context.Background(), playlistID)
		return plVideosLoadedMsg{playlistID: playlistID, videos: vids, err: err}
	}
}

func (t Playlists) localVideosCmd(playlistID int64) tea.Cmd {
	key := fmt.Sprintf("local:%d", playlistID)
	return func() tea.Msg {
		vids, err := t.backend.LocalPlaylistVideos(context.Background(), playlistID)
		return plVideosLoadedMsg{playlistID: key, videos: vids, err: err}
	}
}

func (t Playlists) createYTPlaylistCmd(name string) tea.Cmd {
	return func() tea.Msg {
		id, err := t.backend.CreateYTPlaylist(context.Background(), name)
		return plYTCreatedMsg{name: name, id: id, err: err}
	}
}

func (t Playlists) createLocalPlaylistCmd(name string) tea.Cmd {
	return func() tea.Msg {
		id, err := t.backend.CreatePlaylist(context.Background(), name)
		return plLocalCreatedMsg{name: name, id: id, err: err}
	}
}

// ── render ────────────────────────────────────────────────────────────────────

func (t Playlists) renderPlaylistRows(height int) string {
	if height <= 0 {
		return ""
	}
	n := t.plCount()
	labelW := t.width - render.ColNum - 1 - 4
	selW := t.width - render.ColNum - 1
	if labelW < 10 {
		labelW = 10
	}

	start, end := nav.Window(t.vs, n, height)
	rows := make([]string, 0, end-start)
	for i := start; i < end && i < n; i++ {
		label := t.playlistLabel(i)
		label = render.Truncate(label, labelW)
		if i == t.cursor {
			numStr := styles.RowNum.Background(styles.ColorBgSelect).Render(fmt.Sprintf("%*d ", render.ColNum, i+1))
			rows = append(rows, numStr+styles.Selected.Width(selW).Render("▶ "+label))
		} else {
			numStr := styles.RowNum.Render(fmt.Sprintf("%*d ", render.ColNum, i+1))
			rows = append(rows, numStr+"  "+label)
		}
	}
	if t.ytPlLoading {
		rows = append(rows, styles.Dim.Render("  "+t.spinner.View()+" syncing playlists…"))
	}
	return strings.Join(rows, "\n")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (t Playlists) plCount() int {
	if t.ytPlLoaded {
		return len(t.ytPlaylists) + len(t.localPlaylists)
	}
	return len(t.localPlaylists)
}

func (t Playlists) selectedPlaylistKey() string {
	if t.ytPlLoaded && t.cursor < len(t.ytPlaylists) {
		return t.ytPlaylists[t.cursor].ID
	}
	localIdx := t.cursor
	if t.ytPlLoaded {
		localIdx -= len(t.ytPlaylists)
	}
	if localIdx >= 0 && localIdx < len(t.localPlaylists) {
		return fmt.Sprintf("local:%d", t.localPlaylists[localIdx].ID)
	}
	return ""
}

func (t Playlists) selectedPlaylistName() string {
	if t.ytPlLoaded && t.cursor < len(t.ytPlaylists) {
		return t.ytPlaylists[t.cursor].Title
	}
	localIdx := t.cursor
	if t.ytPlLoaded {
		localIdx -= len(t.ytPlaylists)
	}
	if localIdx >= 0 && localIdx < len(t.localPlaylists) {
		return t.localPlaylists[localIdx].Name
	}
	return ""
}

func (t Playlists) playlistLabel(i int) string {
	if t.ytPlLoaded && i < len(t.ytPlaylists) {
		return t.ytPlaylists[i].Title
	}
	localIdx := i
	if t.ytPlLoaded {
		localIdx -= len(t.ytPlaylists)
	}
	if localIdx >= 0 && localIdx < len(t.localPlaylists) {
		return t.localPlaylists[localIdx].Name
	}
	return ""
}

func (t Playlists) pageHeight() int {
	h := t.height - 2
	if h < 1 {
		return 1
	}
	return h
}

// plLocalID extracts the int64 DB id from a "local:<id>" cache key.
func plLocalID(cacheKey string) int64 {
	if !strings.HasPrefix(cacheKey, "local:") {
		return 0
	}
	id, _ := strconv.ParseInt(strings.TrimPrefix(cacheKey, "local:"), 10, 64)
	return id
}

// plClamp returns v clamped to [0, max-1].
func plClamp(v, max int) int {
	if max <= 0 {
		return 0
	}
	if v >= max {
		return max - 1
	}
	if v < 0 {
		return 0
	}
	return v
}

// ytPlaylistSetChanged reports whether two YT playlist lists differ by ID set.
func ytPlaylistSetChanged(a, b []domain.YTPlaylist) bool {
	if len(a) != len(b) {
		return true
	}
	ids := make(map[string]bool, len(a))
	for _, pl := range a {
		ids[pl.ID] = true
	}
	for _, pl := range b {
		if !ids[pl.ID] {
			return true
		}
	}
	return false
}

func statusMsg(text string) tea.Cmd {
	return func() tea.Msg { return tuipkg.StatusMsg{Text: text} }
}

func errMsg(text string) tea.Cmd {
	return func() tea.Msg { return tuipkg.StatusMsg{Text: text, IsErr: true} }
}
