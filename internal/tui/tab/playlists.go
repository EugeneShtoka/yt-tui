package tab

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
	etable "github.com/evertras/bubble-table/table"
)

const ytWatchLaterID = "WL"

type plCreateStage int

const (
	plCreateNone plCreateStage = iota
	plCreateTypeSelect
	plCreateNameInput
)

type plLocalLoadedMsg struct {
	tuipkg.TabTarget
	playlists []domain.Playlist
}
type plYTLoadedMsg struct {
	tuipkg.TabTarget
	playlists  []domain.YTPlaylist
	err        error
	background bool
	fromCache  bool
}
type plVideosCachedMsg struct {
	tuipkg.TabTarget
	playlistID string
	videos     []domain.Video
}
type plVideosLoadedMsg struct {
	tuipkg.TabTarget
	playlistID string
	videos     []domain.Video
	err        error
	background bool
}
type plYTCreatedMsg struct {
	tuipkg.TabTarget
	name string
	id   string
	err  error
}
type plLocalCreatedMsg struct {
	tuipkg.TabTarget
	name string
	id   int64
	err  error
}
type plDeletedMsg struct {
	tuipkg.TabTarget
	err     error
	isYT    bool
	ytPl    domain.YTPlaylist
	localPl domain.Playlist
}
type plRemovedMsg struct {
	tuipkg.TabTarget
	err   error
	plKey string
	video domain.Video
}

// PlaylistRow is the cell input type for playlist list columns.
type PlaylistRow struct {
	Label string
	Index int
}

func (r PlaylistRow) GetTitle() string { return r.Label }

// playlistsBackend is the narrow slice of the backend the Playlists tab needs:
// local + YouTube playlist ops plus the shared aux data. Declared consumer-side (ISP).
type playlistsBackend interface {
	api.PlaylistBackend
	videotable.AuxBackend
}

type Playlists struct {
	ctx      context.Context
	backend  playlistsBackend
	keys     keymap.KeyMap
	circular bool

	// masterDetail owns the two-pane skeleton: pane, listNav (playlist list),
	// vidNav (playlist video list), width, height (M-1).
	masterDetail

	localPlaylists []domain.Playlist
	ytPlaylists    []domain.YTPlaylist
	ytPlLoad       loadState // fetch lifecycle of the YouTube-playlist list

	vidCache         map[string][]domain.Video
	vidLoad          loadState // fetch lifecycle of the active playlist's videos
	activePlaylistID string
	vidSort          sortState

	aux videotable.AuxData

	createStage   plCreateStage
	createTypeSel int
	createModeYT  bool
	createInput   textinput.Model

	spinnerFrame string
	plCols       []videotable.ColumnDef[PlaylistRow]
	vidCols      []videotable.ColumnDef[videotable.VideoData]
}

// playlistColumns is the full, natural-order column set for the in-playlist
// video list — the list the Playlists sort chord acts on and the one exposed to
// per-panel column configuration. Extracted so the config selector and the
// tab.PanelColumnKeys catalog share one source of truth.
func playlistColumns() []videotable.ColumnDef[videotable.VideoData] {
	return []videotable.ColumnDef[videotable.VideoData]{
		videotable.NumCol[videotable.VideoData](), videotable.IndicatorCol[videotable.VideoData](), videotable.TitleFlexCol[videotable.VideoData](),
		videotable.WatchedCol[videotable.VideoData](), videotable.DurationCol[videotable.VideoData](), videotable.ViewsCol[videotable.VideoData](), videotable.DateCol[videotable.VideoData](),
	}
}

func NewPlaylists(ctx context.Context, backend playlistsBackend, keys keymap.KeyMap, circular bool, defaultSort string, wantCols ...string) Playlists {
	ti := textinput.New()
	ti.Placeholder = "Playlist name…"
	plCols := []videotable.ColumnDef[PlaylistRow]{
		videotable.NumCol[PlaylistRow](),
		videotable.BlankIndicatorCol[PlaylistRow](),
		videotable.TitleFlexCol[PlaylistRow](),
	}
	vidCols := videotable.SelectColumns(playlistColumns(), wantCols)
	return Playlists{
		ctx:         ctx,
		backend:     backend,
		keys:        keys,
		circular:    circular,
		vidCache:    make(map[string][]domain.Video),
		createInput: ti,
		masterDetail: masterDetail{
			listNav: videotable.NewTableNav(plCols, circular, 2),
			vidNav:  videotable.NewTableNav(vidCols, circular, 4),
		},
		plCols:  plCols,
		vidCols: vidCols,
		vidSort: newSortState(sortModeOr(defaultSort, feed.SortViews), videotable.ColumnKeys(vidCols)),
	}
}

func (t Playlists) ID() tuipkg.TabID { return tuipkg.TabPlaylists }
func (t Playlists) Title() string    { return "Playlists" }
func (t Playlists) SelectedVideo() (domain.Video, bool) {
	if t.inDetail() {
		plKey := t.selectedPlaylistKey()
		vids := t.vidCache[plKey]
		idx := t.vidNav.Index()
		if idx >= 0 && idx < len(vids) {
			return vids[idx], true
		}
	}
	return domain.Video{}, false
}
func (t Playlists) Loading() bool { return t.ytPlLoad.inFlight() || t.vidLoad.inFlight() }
func (t Playlists) ShortHelp() []key.Binding {
	if t.inDetail() {
		return []key.Binding{t.keys.Play, t.keys.Download, t.keys.CopyURL, t.keys.VideoInfo, t.keys.SortChord}
	}
	return []key.Binding{t.keys.DrillDown, t.keys.NewList, t.keys.Delete}
}
func (t Playlists) InterceptsInput() bool { return t.createStage == plCreateNameInput }

func (t Playlists) Init() tea.Cmd {
	return tea.Batch(t.localLoadCmd(), t.ytCachedLoadCmd(), t.ytRefreshCmd(false), videotable.LoadAuxDataCmd(t.ctx, t.backend))
}

func (t Playlists) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.listNav.Resize(m.Width, t.plTableHeight())
		t.listNav.SetRows(t.toPlaylistRows())
		t.vidNav.Resize(m.Width, m.Height)

	case tuipkg.SpinnerFrameMsg:
		t.spinnerFrame = m.Frame

	case plLocalLoadedMsg:
		t.localPlaylists = m.playlists
		t.listNav.SetRows(t.toPlaylistRows())

	case plYTLoadedMsg:
		return t.onYTLoaded(m)

	case plVideosCachedMsg:
		return t.onVideosCached(m)

	case plVideosLoadedMsg:
		return t.onVideosLoaded(m)

	case videotable.AuxDataMsg:
		t.aux = m
		if vids, ok := t.vidCache[t.activePlaylistID]; ok {
			t.setVidNavRows(vids)
		}

	case plYTCreatedMsg:
		if m.err != nil {
			return t, errMsg("create playlist: " + m.err.Error())
		}
		t.ytPlaylists = append(t.ytPlaylists, domain.YTPlaylist{ID: m.id, Title: m.name})
		t.listNav.SetRows(t.toPlaylistRows())
		return t, statusMsg("Created playlist: " + m.name)

	case plLocalCreatedMsg:
		if m.err != nil {
			return t, errMsg("create playlist: " + m.err.Error())
		}
		return t, tea.Batch(t.localLoadCmd(), statusMsg("Created playlist: "+m.name))

	case plDeletedMsg:
		return t.onDeleted(m)

	case plRemovedMsg:
		return t.onRemoved(m)

	case tuipkg.NavigateToPlaylistMsg:
		t.scrollToPlaylist(m)

	case tea.KeyPressMsg:
		return t.handleKey(m)
	}
	return t, nil
}

// setVidNavRows rebuilds the video table's rows from the enriched slice.
func (t *Playlists) setVidNavRows(vids []domain.Video) {
	videotable.SetVideoRows(&t.vidNav, vids, t.aux, t.vidCols)
}

func (t Playlists) onYTLoaded(m plYTLoadedMsg) (tea.Model, tea.Cmd) {
	if m.fromCache && t.ytPlLoad.hasData() {
		// A network refresh already landed; don't stomp fresher data with stale cache.
		return t, nil
	}
	if m.err != nil {
		t.ytPlLoad = t.ytPlLoad.settled() // clear in-flight, keep whether we had data
		if !m.background {
			return t, errMsg("playlists: " + m.err.Error())
		}
		return t, nil
	}
	t.ytPlLoad = srcLoaded
	if ytPlaylistSetChanged(t.ytPlaylists, m.playlists) {
		t.ytPlaylists = m.playlists
		t.vidCache = make(map[string][]domain.Video)
	}
	t.listNav.SetRows(t.toPlaylistRows())
	return t, nil
}

func (t Playlists) onVideosCached(m plVideosCachedMsg) (tea.Model, tea.Cmd) {
	if m.playlistID == t.activePlaylistID {
		t.vidLoad = srcRefreshing
		vids := m.videos
		feed.SortVideos(vids, t.vidSort.mode)
		t.vidCache[m.playlistID] = vids
		t.setVidNavRows(vids)
		return t, t.ytVideosRefreshCmd(m.playlistID)
	}
	return t, nil
}

func (t Playlists) onVideosLoaded(m plVideosLoadedMsg) (tea.Model, tea.Cmd) {
	if m.playlistID == t.activePlaylistID {
		t.vidLoad = srcLoaded
	}
	if m.err != nil {
		if m.playlistID == t.activePlaylistID && len(t.vidCache[m.playlistID]) == 0 {
			return t, errMsg("playlist: " + m.err.Error())
		}
		return t, nil
	}
	vids := m.videos
	feed.SortVideos(vids, t.vidSort.mode)
	t.vidCache[m.playlistID] = vids
	if m.playlistID == t.activePlaylistID {
		t.setVidNavRows(vids)
	}
	return t, nil
}

func (t Playlists) onDeleted(m plDeletedMsg) (tea.Model, tea.Cmd) {
	if m.err != nil {
		if m.isYT {
			t.ytPlaylists = append(t.ytPlaylists, m.ytPl)
		} else {
			t.localPlaylists = append(t.localPlaylists, m.localPl)
		}
		t.listNav.SetRows(t.toPlaylistRows())
		return t, errMsg("delete playlist: " + m.err.Error())
	}
	return t, nil
}

func (t Playlists) onRemoved(m plRemovedMsg) (tea.Model, tea.Cmd) {
	if m.err != nil {
		t.vidCache[m.plKey] = append(t.vidCache[m.plKey], m.video)
		if m.plKey == t.activePlaylistID {
			t.setVidNavRows(t.vidCache[m.plKey])
		}
		return t, errMsg("remove from playlist: " + m.err.Error())
	}
	return t, nil
}

func (t Playlists) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch t.createStage {
	case plCreateTypeSelect:
		return t.handleTypeSelect(msg)
	case plCreateNameInput:
		return t.handleNameInput(msg)
	}

	keys := t.keys
	if key.Matches(msg, keys.GotoLine) {
		if t.inDetail() {
			n := len(t.vidCache[t.selectedPlaylistKey()])
			t.vidNav.HandleNav(msg, keys, n)
		} else {
			t.listNav.HandleNav(msg, keys, t.plCount())
		}
		return t, nil
	}

	if t.inDetail() {
		return t.handleVideoPaneKey(msg)
	}
	return t.handleListPaneKey(msg)
}

func (t Playlists) handleListPaneKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	if t.listNav.HandleNav(msg, keys, t.plCount()) {
		return t, nil
	}
	switch {
	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
		return t.drillIntoSelectedPlaylist()
	case key.Matches(msg, keys.NewList):
		return t.beginCreatePlaylist()
	case key.Matches(msg, keys.Refresh):
		t.ytPlLoad = t.ytPlLoad.fetching()
		return t, t.ytRefreshCmd(true)
	case key.Matches(msg, keys.Delete):
		return t.deleteSelected()
	}
	return t, nil
}

// drillIntoSelectedPlaylist enters the video pane for the highlighted playlist:
// it serves cached videos when present and otherwise issues the right fetch — a
// YouTube drilldown for a YT playlist, a local read for a local one. No-op when
// the cursor is past the end of the list.
func (t Playlists) drillIntoSelectedPlaylist() (tea.Model, tea.Cmd) {
	idx, n := t.listNav.Index(), t.plCount()
	if idx >= n {
		return t, nil
	}
	plKey := t.selectedPlaylistKey()
	t.drillIn()
	t.activePlaylistID = plKey

	if !t.ytPlLoad.hasData() || idx >= len(t.ytPlaylists) {
		return t, t.localVideosCmd(plLocalID(plKey))
	}
	if cached, ok := t.vidCache[plKey]; ok && len(cached) > 0 {
		t.vidLoad = srcRefreshing
		return t, t.ytVideosRefreshCmd(plKey)
	}
	t.vidLoad = srcLoading
	return t, t.ytVideosDrilldownCmd(plKey)
}

// beginCreatePlaylist opens the new-playlist flow: the local/YouTube type picker
// when YouTube playlists are available, or the name prompt directly (local-only)
// otherwise.
func (t Playlists) beginCreatePlaylist() (tea.Model, tea.Cmd) {
	t.listNav.Resize(t.width, t.plTableHeight())
	if t.ytPlLoad.hasData() {
		t.createTypeSel = 0
		t.createStage = plCreateTypeSelect
		return t, nil
	}
	t.createModeYT = false
	t.createInput.SetValue("")
	t.createInput.Focus()
	t.createStage = plCreateNameInput
	return t, textinput.Blink
}

func (t Playlists) handleVideoPaneKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if t.listNav.Index() >= t.plCount() {
		t.drillOut()
		return t, nil
	}
	keys := t.keys
	plKey := t.selectedPlaylistKey()
	vids := t.vidCache[plKey]
	n := len(vids)

	if t.vidSort.handleChord(msg, keys.Sort, func(mode int) {
		feed.SortVideos(vids, mode)
		t.vidCache[plKey] = vids
		t.setVidNavRows(vids)
	}) {
		return t, nil
	}

	if t.handleDetailBack(msg, keys, n) {
		return t, nil
	}

	idx := t.vidNav.Index()

	switch {
	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Play):
		if idx < len(vids) {
			v := vids[idx]
			return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v} }
		}
	case key.Matches(msg, keys.Delete):
		return t.removeCurrentVideo(plKey, vids)
	case key.Matches(msg, keys.SortChord):
		if n > 0 {
			t.vidSort.chordActive = true
		}
	default:
		if cmd, ok := videoActionAt(vids, idx, msg, keys); ok {
			return t, cmd
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
		t.listNav.Resize(t.width, t.plTableHeight())
		return t, textinput.Blink
	case key.Matches(msg, keys.Escape):
		t.createStage = plCreateNone
		t.listNav.Resize(t.width, t.plTableHeight())
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
		t.listNav.Resize(t.width, t.plTableHeight())
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
		t.listNav.Resize(t.width, t.plTableHeight())
	default:
		var cmd tea.Cmd
		t.createInput, cmd = t.createInput.Update(msg)
		return t, cmd
	}
	return t, nil
}

func (t Playlists) deleteSelected() (Playlists, tea.Cmd) {
	refs := t.refs()
	cursor := t.listNav.Index()
	if cursor < 0 || cursor >= len(refs) {
		return t, nil
	}
	ref := refs[cursor]
	if ref.key == ytWatchLaterID {
		return t, func() tea.Msg { return tuipkg.StatusMsg{Text: "Cannot delete Watch Later", IsErr: true} }
	}
	delete(t.vidCache, ref.key)
	if ref.yt != nil {
		pl := *ref.yt
		for i, p := range t.ytPlaylists {
			if p.ID == pl.ID {
				t.ytPlaylists = append(t.ytPlaylists[:i], t.ytPlaylists[i+1:]...)
				break
			}
		}
		t.listNav.SetRows(t.toPlaylistRows())
		return t, func() tea.Msg {
			return plDeletedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabPlaylists}, err: t.backend.DeleteYTPlaylist(t.ctx, pl.ID), isYT: true, ytPl: pl}
		}
	}
	pl := *ref.local
	for i, p := range t.localPlaylists {
		if p.ID == pl.ID {
			t.localPlaylists = append(t.localPlaylists[:i], t.localPlaylists[i+1:]...)
			break
		}
	}
	t.listNav.SetRows(t.toPlaylistRows())
	return t, func() tea.Msg {
		return plDeletedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabPlaylists}, err: t.backend.DeletePlaylist(t.ctx, pl.ID), isYT: false, localPl: pl}
	}
}

func (t Playlists) removeCurrentVideo(plKey string, vids []domain.Video) (Playlists, tea.Cmd) {
	c := t.vidNav.Index()
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
	t.setVidNavRows(updated)

	vidID := vid.ID
	if localID := plLocalID(plKey); localID != 0 {
		return t, func() tea.Msg {
			return plRemovedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabPlaylists}, err: t.backend.RemoveFromPlaylist(t.ctx, localID, vidID), plKey: plKey, video: vid}
		}
	}
	// Watch Later routes through the dedicated backend verb (YouTube "WL" when
	// authed, else the local fallback) so removal works offline instead of failing
	// with ErrYTNotInitialized like a plain YT-playlist removal would.
	if plKey == ytWatchLaterID {
		return t, func() tea.Msg {
			return plRemovedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabPlaylists}, err: t.backend.RemoveFromWatchLater(t.ctx, vidID), plKey: plKey, video: vid}
		}
	}
	return t, func() tea.Msg {
		return plRemovedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabPlaylists}, err: t.backend.RemoveFromYTPlaylist(t.ctx, plKey, vidID), plKey: plKey, video: vid}
	}
}

func (t *Playlists) scrollToPlaylist(m tuipkg.NavigateToPlaylistMsg) {
	var key string
	switch {
	case m.PlaylistLocalID != 0:
		key = fmt.Sprintf("local:%d", m.PlaylistLocalID)
	case m.PlaylistID != "":
		key = m.PlaylistID
	default:
		return
	}
	for i, ref := range t.refs() {
		if ref.key == key {
			t.listNav.GotoRow(i)
			t.pane = 1
			return
		}
	}
}

func (t Playlists) toPlaylistRows() []etable.Row {
	n := t.plCount()
	rows := make([]PlaylistRow, n)
	for i := range rows {
		rows[i] = PlaylistRow{Label: t.playlistLabel(i), Index: i}
	}
	return videotable.BuildRows(rows, t.plCols)
}

// Rows the in-progress "new playlist" UI overlays onto the list, shrinking the
// list table by that much while it's showing.
const (
	plCreateTypeSelectRows = 6 // type picker (local vs YouTube)
	plCreateNameInputRows  = 3 // name-entry prompt
)

func (t Playlists) plTableHeight() int {
	switch t.createStage {
	case plCreateTypeSelect:
		if h := t.height - plCreateTypeSelectRows; h >= 1 {
			return h
		}
		return 1
	case plCreateNameInput:
		if h := t.height - plCreateNameInputRows; h >= 1 {
			return h
		}
		return 1
	default:
		if h := t.height; h >= 1 {
			return h
		}
		return 1
	}
}

// plRef projects one playlist-list entry, unifying the YT and local segments
// so callers look up by cursor/key instead of re-deriving "cursor < len(yt)"
// offset math (M-17).
type plRef struct {
	key   string
	title string
	yt    *domain.YTPlaylist // non-nil for a YT playlist
	local *domain.Playlist   // non-nil for a local playlist
}

func (t Playlists) refs() []plRef {
	refs := make([]plRef, 0, len(t.ytPlaylists)+len(t.localPlaylists))
	if t.ytPlLoad.hasData() {
		for i := range t.ytPlaylists {
			pl := &t.ytPlaylists[i]
			refs = append(refs, plRef{key: pl.ID, title: pl.Title, yt: pl})
		}
	}
	for i := range t.localPlaylists {
		pl := &t.localPlaylists[i]
		refs = append(refs, plRef{key: fmt.Sprintf("local:%d", pl.ID), title: pl.Name, local: pl})
	}
	return refs
}

func (t Playlists) plCount() int { return len(t.refs()) }

func (t Playlists) selectedPlaylistKey() string {
	refs := t.refs()
	if cursor := t.listNav.Index(); cursor >= 0 && cursor < len(refs) {
		return refs[cursor].key
	}
	return ""
}

func (t Playlists) selectedPlaylistName() string {
	refs := t.refs()
	if cursor := t.listNav.Index(); cursor >= 0 && cursor < len(refs) {
		return refs[cursor].title
	}
	return ""
}

func (t Playlists) playlistLabel(i int) string {
	refs := t.refs()
	if i >= 0 && i < len(refs) {
		return refs[i].title
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
