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
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const ytWatchLaterID = "WL"

type plCreateStage int

const (
	plCreateNone        plCreateStage = iota
	plCreateTypeSelect
	plCreateNameInput
)

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
type plAuxLoadedMsg struct {
	positions   map[string]int64
	watched     map[string]bool
	localStatus map[string]domain.VideoStatus
}

type Playlists struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	width, height int

	localPlaylists []domain.Playlist
	ytPlaylists    []domain.YTPlaylist
	ytPlLoading    bool
	ytPlLoaded     bool

	vidCache        map[string][]domain.Video
	vidLoading      bool
	vidSort         int
	sortChordActive bool
	gotoTopActive   bool

	positions   map[string]int64
	watched     map[string]bool
	localStatus map[string]domain.VideoStatus

	pane int // 0 = playlist list, 1 = video list

	createStage   plCreateStage
	createTypeSel int
	createModeYT  bool
	createInput   textinput.Model

	spinner  spinner.Model
	plTable  table.Model
	vidTable table.Model
	numBuf   string
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
		plTable:     newTable(),
		vidTable:    newTable(),
	}
}

func (t Playlists) ID() tuipkg.TabID          { return tuipkg.TabPlaylists }
func (t Playlists) Title() string             { return "Playlists" }
func (t Playlists) ShortHelp() []key.Binding {
	if t.pane == 1 {
		return []key.Binding{t.keys.Play, t.keys.Download, t.keys.CopyURL, t.keys.VideoInfo, t.keys.SortChord}
	}
	return []key.Binding{t.keys.DrillDown, t.keys.NewList, t.keys.Delete}
}
func (t Playlists) InterceptsInput() bool     { return t.createStage == plCreateNameInput }

func (t Playlists) Init() tea.Cmd {
	return tea.Batch(t.localLoadCmd(), t.ytLoadCmd(false), t.plAuxLoadCmd(), t.spinner.Tick)
}

func (t Playlists) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.plTable.SetColumns(t.plColumns())
		t.plTable.SetHeight(t.plTableHeight())
		t.plTable.SetRows(t.toPlaylistRows())
		t.vidTable.SetColumns(computeVideoColumns(t.width, false))
		t.vidTable.SetHeight(t.height - 4)

	case spinner.TickMsg:
		if t.ytPlLoading || t.vidLoading {
			var cmd tea.Cmd
			t.spinner, cmd = t.spinner.Update(m)
			return t, cmd
		}

	case plLocalLoadedMsg:
		t.localPlaylists = m.playlists
		t.plTable.SetRows(t.toPlaylistRows())

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
		t.plTable.SetRows(t.toPlaylistRows())

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
		t.vidTable.SetRows(toVideoRows(vids, t.positions, t.watched, t.localStatus, false, t.width))

	case plAuxLoadedMsg:
		t.positions = m.positions
		t.watched = m.watched
		t.localStatus = m.localStatus

	case tuipkg.RefreshPositionsMsg:
		return t, t.plAuxLoadCmd()

	case plYTCreatedMsg:
		if m.err != nil {
			return t, errMsg("create playlist: " + m.err.Error())
		}
		t.ytPlaylists = append(t.ytPlaylists, domain.YTPlaylist{ID: m.id, Title: m.name})
		t.plTable.SetRows(t.toPlaylistRows())
		return t, statusMsg("Created playlist: " + m.name)

	case plLocalCreatedMsg:
		if m.err != nil {
			return t, errMsg("create playlist: " + m.err.Error())
		}
		return t, tea.Batch(t.localLoadCmd(), statusMsg("Created playlist: "+m.name))

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

	case tea.KeyPressMsg:
		return t.handleKey(m)
	}
	return t, nil
}

func (t Playlists) View() tea.View {
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
		return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header,
			t.plTable.View()+"\n\n\n"+prompt))

	case plCreateNameInput:
		label := "New local playlist: "
		if t.createModeYT {
			label = "New YouTube playlist: "
		}
		prompt := styles.Bold.Render(label) + t.createInput.View()
		return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header,
			t.plTable.View()+"\n\n"+prompt))
	}

	if t.pane == 1 && t.plTable.Cursor() < t.plCount() {
		subHeader := styles.SectionTitle.Render("← " + t.selectedPlaylistName())
		plKey := t.selectedPlaylistKey()
		vids := t.vidCache[plKey]
		var body string
		switch {
		case len(vids) > 0:
			body = t.vidTable.View()
		case t.vidLoading:
			body = t.spinner.View() + " Loading from YouTube…"
		default:
			body = styles.Dim.Render("Empty playlist.")
		}
		parts := []string{header, subHeader, body}
		if t.numBuf != "" {
			parts = append(parts, gotoLineView(t.numBuf))
		}
		return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
	}

	body := t.plTable.View()
	if t.ytPlLoading {
		body += "\n" + styles.Dim.Render("  "+t.spinner.View()+" syncing playlists…")
	}
	parts := []string{header, body}
	_ = bodyH
	if t.numBuf != "" {
		parts = append(parts, gotoLineView(t.numBuf))
	}
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

func (t Playlists) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch t.createStage {
	case plCreateTypeSelect:
		return t.handleTypeSelect(msg)
	case plCreateNameInput:
		return t.handleNameInput(msg)
	}

	if consumed, doTop := handleGotoPrefix(&t.gotoTopActive, t.keys, msg); consumed {
		if doTop {
			t.numBuf = ""
			if t.pane == 1 {
				t.vidTable.GotoTop()
			} else {
				t.plTable.GotoTop()
			}
		}
		return t, nil
	}

	if checkGotoNum(&t.numBuf, msg) {
		return t, nil
	}
	numBuf := t.numBuf
	t.numBuf = ""

	keys := t.keys
	if key.Matches(msg, keys.GotoLine) {
		tbl := &t.plTable
		if t.pane == 1 {
			tbl = &t.vidTable
		}
		if numBuf != "" {
			applyGoto(numBuf, tbl)
		} else {
			tbl.GotoBottom()
		}
		return t, nil
	}

	if t.pane == 1 {
		return t.handleVideoPaneKey(msg, numBuf)
	}
	return t.handleListPaneKey(msg, numBuf)
}

func (t Playlists) handleListPaneKey(msg tea.KeyPressMsg, numBuf string) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := t.plCount()

	switch {
	case key.Matches(msg, keys.Up):
		if t.circular && n > 0 && t.plTable.Cursor() == 0 {
			t.plTable.GotoBottom()
		} else {
			t.plTable.MoveUp(1)
		}
	case key.Matches(msg, keys.Down):
		if t.circular && n > 0 && t.plTable.Cursor() == n-1 {
			t.plTable.GotoTop()
		} else {
			t.plTable.MoveDown(1)
		}
	case key.Matches(msg, keys.PageUp):
		t.plTable.MoveUp(t.plTable.Height())
	case key.Matches(msg, keys.PageDown):
		t.plTable.MoveDown(t.plTable.Height())
	case key.Matches(msg, keys.GotoBottom):
		t.plTable.GotoBottom()

	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
		if t.plTable.Cursor() < n {
			plKey := t.selectedPlaylistKey()
			t.pane = 1
			t.vidTable.GotoTop()
			if t.ytPlLoaded && t.plTable.Cursor() < len(t.ytPlaylists) {
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
			t.plTable.SetHeight(t.plTableHeight())
		} else {
			t.createModeYT = false
			t.createInput.SetValue("")
			t.createInput.Focus()
			t.createStage = plCreateNameInput
			t.plTable.SetHeight(t.plTableHeight())
			return t, textinput.Blink
		}

	case key.Matches(msg, keys.Refresh):
		t.ytPlLoading = true
		return t, t.ytLoadCmd(true)

	case key.Matches(msg, keys.Delete):
		return t.deleteSelected()

	case key.Matches(msg, keys.Escape):
		if numBuf != "" {
			return t, nil
		}
	}
	_ = numBuf
	return t, nil
}

func (t Playlists) handleVideoPaneKey(msg tea.KeyPressMsg, numBuf string) (tea.Model, tea.Cmd) {
	if t.plTable.Cursor() >= t.plCount() {
		t.pane = 0
		return t, nil
	}
	keys := t.keys
	plKey := t.selectedPlaylistKey()
	vids := t.vidCache[plKey]
	n := len(vids)

	if t.sortChordActive {
		t.sortChordActive = false
		sk := keys.Sort
		switch {
		case key.Matches(msg, sk.Date):
			t.vidSort = feed.SortDate
		case key.Matches(msg, sk.Views):
			t.vidSort = feed.SortViews
		case key.Matches(msg, sk.Name):
			t.vidSort = feed.SortName
		case key.Matches(msg, sk.Channel):
			t.vidSort = feed.SortChannel
		case key.Matches(msg, sk.Duration):
			t.vidSort = feed.SortDuration
		}
		feed.SortVideos(vids, t.vidSort)
		t.vidCache[plKey] = vids
		t.vidTable.SetRows(toVideoRows(vids, t.positions, t.watched, t.localStatus, false, t.width))
		return t, nil
	}

	switch {
	case key.Matches(msg, keys.Left), key.Matches(msg, keys.Escape):
		if numBuf != "" {
			return t, nil
		}
		t.pane = 0
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
	case key.Matches(msg, keys.GotoBottom):
		t.vidTable.GotoBottom()

	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Play):
		if t.vidTable.Cursor() < len(vids) {
			v := vids[t.vidTable.Cursor()]
			return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v} }
		}
	case key.Matches(msg, keys.PlayAudio):
		if t.vidTable.Cursor() < len(vids) {
			v := vids[t.vidTable.Cursor()]
			return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v, AudioOnly: true} }
		}
	case key.Matches(msg, keys.Download):
		if t.vidTable.Cursor() < len(vids) {
			v := vids[t.vidTable.Cursor()]
			return t, func() tea.Msg { return tuipkg.EnqueueMsg{Video: v} }
		}
	case key.Matches(msg, keys.DownloadAudio):
		if t.vidTable.Cursor() < len(vids) {
			v := vids[t.vidTable.Cursor()]
			return t, func() tea.Msg { return tuipkg.EnqueueMsg{Video: v, AudioOnly: true} }
		}
	case key.Matches(msg, keys.CopyURL):
		if t.vidTable.Cursor() < len(vids) {
			return t, func() tea.Msg { return tuipkg.CopyURLMsg{URL: vids[t.vidTable.Cursor()].URL} }
		}
	case key.Matches(msg, keys.VideoInfo):
		if t.vidTable.Cursor() < len(vids) {
			v := vids[t.vidTable.Cursor()]
			return t, func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: "video_detail", Video: v} }
		}
	case key.Matches(msg, keys.AddList):
		if t.vidTable.Cursor() < len(vids) {
			v := vids[t.vidTable.Cursor()]
			return t, func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: "add_to_playlist", Video: v} }
		}
	case key.Matches(msg, keys.Delete):
		return t.removeCurrentVideo(plKey, vids)
	case key.Matches(msg, keys.SortChord):
		if n > 0 {
			t.sortChordActive = true
		}
	}
	return t, nil
}

func (t Playlists) handleTypeSelect(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		t.plTable.SetHeight(t.plTableHeight())
		return t, textinput.Blink
	case key.Matches(msg, keys.Escape):
		t.createStage = plCreateNone
		t.plTable.SetHeight(t.plTableHeight())
	}
	return t, nil
}

func (t Playlists) handleNameInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	switch {
	case key.Matches(msg, keys.DrillDown):
		name := strings.TrimSpace(t.createInput.Value())
		isYT := t.createModeYT
		t.createInput.Blur()
		t.createStage = plCreateNone
		t.createModeYT = false
		t.plTable.SetHeight(t.plTableHeight())
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
		t.plTable.SetHeight(t.plTableHeight())
	default:
		var cmd tea.Cmd
		t.createInput, cmd = t.createInput.Update(msg)
		return t, cmd
	}
	return t, nil
}

func (t Playlists) deleteSelected() (Playlists, tea.Cmd) {
	n := t.plCount()
	cursor := t.plTable.Cursor()
	if cursor >= n {
		return t, nil
	}
	plKey := t.selectedPlaylistKey()
	if plKey == ytWatchLaterID {
		return t, func() tea.Msg { return tuipkg.StatusMsg{Text: "Cannot delete Watch Later", IsErr: true} }
	}
	if t.ytPlLoaded && cursor < len(t.ytPlaylists) {
		pl := t.ytPlaylists[cursor]
		delete(t.vidCache, pl.ID)
		t.ytPlaylists = append(t.ytPlaylists[:cursor], t.ytPlaylists[cursor+1:]...)
		t.plTable.SetRows(t.toPlaylistRows())
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
	t.plTable.SetRows(t.toPlaylistRows())
	id := pl.ID
	return t, func() tea.Msg {
		return plDeletedMsg{err: t.backend.DeletePlaylist(context.Background(), id)}
	}
}

func (t Playlists) removeCurrentVideo(plKey string, vids []domain.Video) (Playlists, tea.Cmd) {
	c := t.vidTable.Cursor()
	if c >= len(vids) {
		return t, nil
	}
	vid := vids[c]
	updated := make([]domain.Video, 0, len(vids)-1)
	for _, v := range vids {
		if v.ID != vid.ID {
			updated = append(updated, v)
		}
	}
	t.vidCache[plKey] = updated
	t.vidTable.SetRows(toVideoRows(updated, t.positions, t.watched, t.localStatus, false, t.width))

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
				t.plTable.SetCursor(offset + i)
				t.pane = 1
				return
			}
		}
		return
	}
	if m.PlaylistID != "" && t.ytPlLoaded {
		for i, pl := range t.ytPlaylists {
			if pl.ID == m.PlaylistID {
				t.plTable.SetCursor(i)
				t.pane = 1
				return
			}
		}
	}
}

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
	k := fmt.Sprintf("local:%d", playlistID)
	return func() tea.Msg {
		vids, err := t.backend.LocalPlaylistVideos(context.Background(), playlistID)
		return plVideosLoadedMsg{playlistID: k, videos: vids, err: err}
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

func (t Playlists) plAuxLoadCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		positions, _ := t.backend.AllVideoPositions(ctx)
		watched, _ := t.backend.WatchedVideoIDs(ctx)
		localVids, _ := t.backend.LocalVideos(ctx)
		localStatus := make(map[string]domain.VideoStatus, len(localVids))
		for i := range localVids {
			localStatus[localVids[i].ID] = localVids[i].Status
		}
		return plAuxLoadedMsg{positions: positions, watched: watched, localStatus: localStatus}
	}
}

func (t Playlists) plColumns() []table.Column {
	nameW := t.width - render.ColNum - colIndicator
	if nameW < 10 {
		nameW = 10
	}
	return []table.Column{
		{Title: ralign("#", render.ColNum), Width: render.ColNum},
		{Title: " ", Width: colIndicator},
		{Title: "Name", Width: nameW},
	}
}

func (t Playlists) toPlaylistRows() []table.Row {
	n := t.plCount()
	rows := make([]table.Row, n)
	for i := 0; i < n; i++ {
		rows[i] = table.Row{rowNum(i), "", t.playlistLabel(i)}
	}
	return rows
}

func (t Playlists) plTableHeight() int {
	switch t.createStage {
	case plCreateTypeSelect:
		h := t.height - 2 - 4 // section title + 4 lines for type select prompt
		if h < 1 {
			return 1
		}
		return h
	case plCreateNameInput:
		h := t.height - 2 - 3
		if h < 1 {
			return 1
		}
		return h
	default:
		h := t.height - 2
		if h < 1 {
			return 1
		}
		return h
	}
}

func (t Playlists) plCount() int {
	if t.ytPlLoaded {
		return len(t.ytPlaylists) + len(t.localPlaylists)
	}
	return len(t.localPlaylists)
}

func (t Playlists) selectedPlaylistKey() string {
	cursor := t.plTable.Cursor()
	if t.ytPlLoaded && cursor < len(t.ytPlaylists) {
		return t.ytPlaylists[cursor].ID
	}
	localIdx := cursor
	if t.ytPlLoaded {
		localIdx -= len(t.ytPlaylists)
	}
	if localIdx >= 0 && localIdx < len(t.localPlaylists) {
		return fmt.Sprintf("local:%d", t.localPlaylists[localIdx].ID)
	}
	return ""
}

func (t Playlists) selectedPlaylistName() string {
	cursor := t.plTable.Cursor()
	if t.ytPlLoaded && cursor < len(t.ytPlaylists) {
		return t.ytPlaylists[cursor].Title
	}
	localIdx := cursor
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

func plLocalID(cacheKey string) int64 {
	if !strings.HasPrefix(cacheKey, "local:") {
		return 0
	}
	id, _ := strconv.ParseInt(strings.TrimPrefix(cacheKey, "local:"), 10, 64)
	return id
}


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

