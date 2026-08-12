package tab

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/channels"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
	etable "github.com/evertras/bubble-table/table"
)

const (
	chEditNone  = 0
	chEditAlias = 1
	chEditTags  = 2
)

// The Channels tab's source filter is the shared srcMode (see panelmode.go),
// with the channelModes option list: recommended / subscribed / mixed / blocked.

// ChannelRow is the cell input type for the channel list table.
type ChannelRow struct {
	Channel            domain.Channel
	Latest             domain.Video
	LatestPositionSecs int
}

func (r ChannelRow) GetTitle() string       { return r.Latest.Title }
func (r ChannelRow) GetChannelID() string   { return r.Channel.ID }
func (r ChannelRow) GetChannelName() string { return r.Channel.DisplayName() }
func (r ChannelRow) GetCount() int64        { return r.Channel.Subscribers }
func (r ChannelRow) GetTags() []string      { return r.Channel.Tags }
func (r ChannelRow) IsBlocked() bool        { return r.Channel.Blocked }

// GetStateLabel resolves the subscription state to a display label for the
// State column (blocked channels are always state=none, shown by the marker).
func (r ChannelRow) GetStateLabel() string {
	switch r.Channel.SubState() {
	case domain.SubYT:
		return "YT"
	case domain.SubLocal:
		return "Local"
	default:
		return "—"
	}
}
func (r ChannelRow) GetLatestVideo() videotable.VideoData {
	return videotable.VideoData{Video: r.Latest, LastPositionSecs: r.LatestPositionSecs}
}

type chsLoadedMsg struct {
	tuipkg.TabTarget
	chans     []domain.Channel
	recVideos []domain.Video // recommended-feed cache, folded into the universe
	latest    map[string]domain.Video
	err       error
}
type chVideosCachedMsg struct {
	tuipkg.TabTarget
	channelID string
	videos    []domain.Video
}
type chVideosFetchedMsg struct {
	tuipkg.TabTarget
	channelID string
	videos    []domain.Video
	err       error
}

// chVideosPolledMsg carries the result of a poll's DB read (driven by Root's
// shared PollTickMsg), so videos a background crawl streams into the DB appear in
// the open channel-video list without a manual refresh.
type chVideosPolledMsg struct {
	tuipkg.TabTarget
	channelID string
	videos    []domain.Video
}

// channelsBackend is the narrow slice of the backend the Channels tab needs:
// channel listing/mutation, feed cache, and the shared aux data. Declared
// consumer-side so the tab depends only on what it uses (ISP).
type channelsBackend interface {
	api.ChannelBackend
	api.FeedBackend
	videotable.AuxBackend
}

type Channels struct {
	ctx                context.Context
	backend            channelsBackend
	keys               keymap.KeyMap
	circular           bool
	channelLatestCount int
	refreshInterval    time.Duration        // auto-refresh throttle window (0 = always refresh)
	lastRefresh        map[string]time.Time // channelID → last successful video fetch

	// masterDetail owns the two-pane skeleton: pane, listNav (channel list),
	// vidNav (channel video list), width, height (M-1).
	masterDetail

	subs         channels.ChannelSet
	chLatest     map[string]domain.Video
	sortedChs    []domain.Channel // cached sorted+view-filtered slice, rebuilt on mutation
	loading      bool
	sort         sortState
	spinnerFrame string

	view       srcMode         // active source filter (recommended / subscribed / mixed / blocked / stale)
	recFeedIDs map[string]bool // channel IDs currently in the recommended feed
	stale      staleFilter     // stale-tagged-channel partition config (hide + threshold)
	picker     modePicker      // the view picker (shared inline selector)

	aux videotable.AuxData

	chVideos    []domain.Video
	chVidLoad   loadState // fetch lifecycle of the drilled-in channel's videos
	activeChID  string
	activeChURL string

	editMode  int
	editInput textinput.Model

	chCols    []videotable.ColumnDef[ChannelRow]           // channel list columns
	chVidCols []videotable.ColumnDef[videotable.VideoData] // channel video columns
}

// channelColumns is the full, natural-order column set for the Channels list.
// Extracted so the per-panel column selector and tab.PanelColumnKeys catalog
// share one source of truth.
func channelColumns() []videotable.ColumnDef[ChannelRow] {
	return []videotable.ColumnDef[ChannelRow]{
		videotable.NumCol[ChannelRow](),
		videotable.ChBlockedIndicatorCol[ChannelRow](),
		videotable.ChNameCol[ChannelRow](),
		videotable.ChStateCol[ChannelRow](),
		videotable.ChTagsCol[ChannelRow](),
		videotable.SubsCol[ChannelRow](),
		videotable.TitleFlexCol[ChannelRow](),
		videotable.ChLatestWatchedCol[ChannelRow](), videotable.ChLatestDurationCol[ChannelRow](),
		videotable.ChLatestViewsCol[ChannelRow](),
		videotable.ChLatestDateCol[ChannelRow](),
	}
}

// ChannelsOpts carries the Channels-tab configuration. Grouping these named
// fields keeps the constructor call site readable and prevents silent
// transposition of the several adjacent int/bool/string arguments.
type ChannelsOpts struct {
	LatestCount    int    // number of latest videos to show per channel
	RefreshMinutes int    // background refresh interval
	View           string // default source-filter view
	Sort           string // default sort mode
	HideStale      bool   // hide stale-tagged channels
	StaleDays      int    // age threshold for "stale"
}

func NewChannels(ctx context.Context, backend channelsBackend, keys keymap.KeyMap, circular bool, opts ChannelsOpts, wantCols ...string) Channels {
	chCols := videotable.SelectColumns(channelColumns(), wantCols)
	chVidCols := videotable.StandardVideoColumns()
	return Channels{
		ctx:                ctx,
		backend:            backend,
		keys:               keys,
		circular:           circular,
		channelLatestCount: opts.LatestCount,
		refreshInterval:    time.Duration(opts.RefreshMinutes) * time.Minute,
		lastRefresh:        make(map[string]time.Time),
		view:               parseSrcMode(opts.View),
		recFeedIDs:         map[string]bool{},
		stale:              staleFilter{hide: opts.HideStale, days: opts.StaleDays},
		picker:             newModePicker("View", modeLabels(channelModes), circular),
		sort:               newSortState(sortModeOr(opts.Sort, feed.SortDate), videotable.ColumnKeys(chCols)),
		editInput:          textinput.New(),
		masterDetail: masterDetail{
			listNav: videotable.NewTableNav(chCols, circular, 2),
			vidNav:  videotable.NewTableNav(chVidCols, circular, 4),
		},
		chCols:    chCols,
		chVidCols: chVidCols,
	}
}

func (t Channels) ID() tuipkg.TabID { return tuipkg.TabChannels }
func (t Channels) Title() string    { return "Channels" }
func (t Channels) SelectedVideo() (domain.Video, bool) {
	if t.inDetail() {
		return t.chVidAt(t.vidNav.Index())
	}
	idx := t.listNav.Index()
	if idx < len(t.sortedChs) {
		if v := t.chLatest[t.sortedChs[idx].ID]; v.ID != "" {
			return v, true
		}
	}
	return domain.Video{}, false
}
func (t Channels) Loading() bool { return t.loading || t.chVidLoad.inFlight() }

// WithListBorderDimmed fades the list frame while a focused info panel sits over
// it (satisfies the app.listDimmable seam; replaces the styles.ListBorderDimmed
// global). Value receiver: returns the updated copy.
func (t Channels) WithListBorderDimmed(dimmed bool) tuipkg.Tab {
	t.setBorderDimmed(dimmed)
	return t
}
func (t Channels) ShortHelp() []key.Binding {
	if t.inDetail() {
		return []key.Binding{t.keys.Play, t.keys.Refresh, t.keys.ForceRefresh, t.keys.Left}
	}
	return []key.Binding{t.keys.DrillDown, t.keys.RenameChannel, t.keys.TagChannel, t.keys.Unsubscribe, t.keys.Block, t.keys.PanelMode, t.keys.SortChord}
}
func (t Channels) InterceptsInput() bool { return t.editInput.Focused() || t.picker.isOpen() }

func (t Channels) Init() tea.Cmd {
	t.loading = true
	return tea.Batch(t.chsLoadCmd(), videotable.LoadAuxDataCmd(t.ctx, t.backend))
}

// rebuildSorted recomputes and caches the view-filtered, sorted channel slice.
// Call whenever subs, chLatest, the sort mode, or the view changes. It selects
// only the channels matching the active view and the stale partition (so
// subs.Channels() stays the full, unsorted universe) before ordering them.
func (t *Channels) rebuildSorted() {
	out := selectChannels(t.subs.Channels(), t.view, t.recFeedIDs, t.stale, time.Now())
	feed.SortChannels(out, t.sort.mode, t.chLatest)
	t.sortedChs = out
}

func (t Channels) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.resize(m.Width, m.Height)
		t.setChNavRows()
		t.setChVidNavRows()

	case tuipkg.SpinnerFrameMsg:
		t.spinnerFrame = m.Frame

	case chsLoadedMsg:
		return t.onChsLoaded(m)

	case videotable.AuxDataMsg:
		t.aux = m
		t.setChNavRows()
		t.setChVidNavRows()

	case tuipkg.UnsubscribeResultMsg:
		if m.Err != nil {
			t.subs.Subscribe(m.Channel)
			t.rebuildAndSetChNav()
		}

	case tuipkg.BlockChannelResultMsg:
		// Revert the optimistic transition on failure by restoring the original
		// channel value (state + blocked) carried in the message.
		if m.Err != nil {
			t.subs.Set(m.Channel)
			t.rebuildAndSetChNav()
		}

	case chVideosCachedMsg:
		return t.onChVideosCached(m)

	case chVideosFetchedMsg:
		return t.onChVideosFetched(m)

	case tuipkg.PollTickMsg:
		// Root drives the cadence; reload whatever this tab is showing. In the
		// video pane, re-read the open channel's cached videos (grows as a crawl
		// streams them in); in the list pane, refresh the universe so per-channel
		// latest-video columns update as channels get backfilled.
		if t.inDetail() && t.activeChID != "" {
			return t, t.chPollFetchCmd(t.activeChID)
		}
		return t, t.chsLoadCmd()

	case chVideosPolledMsg:
		return t.onChVideosPolled(m)

	case tea.KeyPressMsg:
		if t.editMode != chEditNone {
			return t.handleEditInput(m)
		}
		return t.handleKey(m)
	}
	return t, nil
}

// setChNavRows rebuilds the channel-list table's rows from the sorted slice.
func (t *Channels) setChNavRows() { t.listNav.SetRows(t.toChannelRows(t.sortedChs)) }

// setChVidNavRows rebuilds the channel-video table's rows from the enriched slice.
func (t *Channels) setChVidNavRows() {
	videotable.SetVideoRows(&t.vidNav, t.chVideos, t.aux, t.chVidCols)
}

// rebuildAndSetChNav re-sorts the channel universe and refreshes the list table.
func (t *Channels) rebuildAndSetChNav() {
	t.rebuildSorted()
	t.setChNavRows()
}

func (t Channels) onChsLoaded(m chsLoadedMsg) (tea.Model, tea.Cmd) {
	t.loading = false
	if m.err != nil {
		return t, errMsg("channels: " + m.err.Error())
	}
	// Fold the recommended-feed channels into the universe so the recommended/
	// mixed modes can list (and tag) channels we don't subscribe to. Synthesized
	// rows are state=none and live only in memory until first tagged/aliased.
	t.recFeedIDs = channels.RecFeedIDs(m.recVideos)
	t.subs = channels.New(mergeUniverse(m.chans, m.recVideos))
	t.chLatest = mergeRecLatest(m.latest, m.recVideos)
	// Seed the auto-refresh throttle from the persisted per-channel fetch
	// times so the throttle survives restarts (only ever advancing).
	for i := range m.chans {
		ch := &m.chans[i]
		if ch.VideosRefreshedAt <= 0 {
			continue
		}
		if ts := time.Unix(ch.VideosRefreshedAt, 0); ts.After(t.lastRefresh[ch.ID]) {
			t.lastRefresh[ch.ID] = ts
		}
	}
	t.rebuildAndSetChNav()
	return t, nil
}

func (t Channels) onChVideosCached(m chVideosCachedMsg) (tea.Model, tea.Cmd) {
	if m.channelID == t.activeChID {
		t.chVideos = m.videos
		t.chVidLoad = srcLoaded
		t.setChVidNavRows()
		// Auto-refresh (latest N) only if this channel wasn't fetched recently;
		// otherwise show the cache as-is. Manual refresh (r/R) always fetches.
		if t.channelStale(m.channelID) {
			t.chVidLoad = srcRefreshing
			return t, t.chRefreshCmd(false)
		}
	}
	return t, nil
}

func (t Channels) onChVideosFetched(m chVideosFetchedMsg) (tea.Model, tea.Cmd) {
	if m.err != nil {
		if m.channelID == t.activeChID {
			t.chVidLoad = srcLoaded
			return t, errMsg("load videos: " + m.err.Error())
		}
		return t, nil
	}
	t.lastRefresh[m.channelID] = time.Now()
	if m.channelID == t.activeChID {
		t.chVideos = m.videos
		t.chVidLoad = srcLoaded
		t.setChVidNavRows()
	}
	return t, nil
}

// onChVideosPolled applies a poll's DB read: if it surfaced more videos than are
// currently shown, the open list grows in place (new, older videos append at the
// bottom, so the cursor is undisturbed). Root's PollTickMsg drives the next read.
func (t Channels) onChVideosPolled(m chVideosPolledMsg) (tea.Model, tea.Cmd) {
	if m.channelID == t.activeChID && t.inDetail() && len(m.videos) > len(t.chVideos) {
		t.chVideos = m.videos
		t.setChVidNavRows()
	}
	return t, nil
}

func (t Channels) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys

	if t.picker.isOpen() {
		return t.handlePickerKey(msg)
	}

	if t.sort.handleChord(msg, keys.Sort, func(int) {
		t.rebuildAndSetChNav()
	}) {
		return t, nil
	}

	return t.handleKeyFlat(msg)
}

// handlePickerKey drives the view picker: applying the highlighted view
// rebuilds the list and resets the cursor.
func (t Channels) handlePickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if t.picker.handleKey(msg, t.keys) == pickerCommitted {
		t.view = channelModes[t.picker.selection()]
		t.rebuildAndSetChNav()
		t.listNav.GotoRow(0)
	}
	return t, nil
}

func (t Channels) handleKeyFlat(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if !t.inDetail() {
		return t.handleKeyList(msg)
	}
	return t.handleKeyVideos(msg)
}

// handleKeyList routes keys for pane 0 (the channel list).
func (t Channels) handleKeyList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	if t.listNav.HandleNav(msg, keys, len(t.sortedChs)) {
		return t, nil
	}

	// List-level actions that don't need a highlighted channel.
	switch {
	case key.Matches(msg, keys.PanelMode):
		t.picker.openAt(modeIndex(channelModes, t.view))
		return t, nil
	case key.Matches(msg, keys.SortChord):
		t.sort.chordActive = true
		return t, nil
	}

	// Everything else operates on the highlighted channel.
	ch, ok := t.selectedChannel()
	if !ok {
		return t, nil
	}
	switch {
	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
		return t.drillIntoChannel(ch)
	case key.Matches(msg, keys.RenameChannel):
		return t.beginEdit(ch, chEditAlias)
	case key.Matches(msg, keys.TagChannel):
		return t.beginEdit(ch, chEditTags)
	case key.Matches(msg, keys.Unsubscribe):
		return t.unsubscribeChannel(ch, false)
	case key.Matches(msg, keys.Block):
		return t.toggleBlock(ch)
	}
	if v := t.chLatest[ch.ID]; v.ID != "" {
		if cmd, ok := HandleVideoAction(msg, v, keys); ok {
			return t, cmd
		}
	}
	return t, nil
}

// selectedChannel returns the highlighted channel, or false when the cursor is
// past the end of the (possibly empty) list.
func (t Channels) selectedChannel() (domain.Channel, bool) {
	idx := t.listNav.Index()
	if idx < 0 || idx >= len(t.sortedChs) {
		return domain.Channel{}, false
	}
	return t.sortedChs[idx], true
}

// drillIntoChannel enters pane 1 for ch, reusing already-loaded videos when the
// same channel is re-opened (auto-refreshing only when stale).
func (t Channels) drillIntoChannel(ch domain.Channel) (tea.Model, tea.Cmd) {
	t.pane = 1
	if ch.ID == t.activeChID && len(t.chVideos) > 0 {
		t.chVidLoad = srcLoaded
		if t.channelStale(ch.ID) {
			t.chVidLoad = srcRefreshing
			return t, t.chRefreshCmd(false)
		}
		return t, nil
	}
	t.activeChID = ch.ID
	t.activeChURL = ch.URL
	t.chVideos = nil
	t.chVidLoad = srcLoading
	t.vidNav.SetRows(nil)
	t.vidNav.GotoRow(0)
	return t, tea.Batch(t.chDrilldownCmd(ch))
}

// beginEdit opens the inline alias/tags editor for ch in the given mode.
func (t Channels) beginEdit(ch domain.Channel, mode int) (tea.Model, tea.Cmd) {
	if mode == chEditAlias {
		t.editInput.SetValue(ch.DisplayName())
		t.editInput.Placeholder = "alias (empty to clear)…"
	} else {
		t.editInput.SetValue(strings.Join(ch.Tags, ", "))
		t.editInput.Placeholder = "comma-separated tags…"
	}
	t.editInput.Focus()
	t.editMode = mode
	return t, textinput.Blink
}

// unsubscribeChannel removes ch optimistically and emits the unsubscribe msg;
// when returnToList is set it also drops back to pane 0 (used from the video pane).
func (t Channels) unsubscribeChannel(ch domain.Channel, returnToList bool) (tea.Model, tea.Cmd) {
	t.subs.Remove(ch)
	if returnToList {
		t.drillOut()
	}
	t.rebuildAndSetChNav()
	return t, func() tea.Msg { return tuipkg.UnsubscribeMsg{Channel: ch} }
}

// handleKeyVideos routes keys for pane 1 (the channel's video list).
func (t Channels) handleKeyVideos(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := len(t.chVideos)
	if t.handleDetailBack(msg, keys, n) {
		return t, nil
	}

	idx := t.vidNav.Index()
	switch {
	case key.Matches(msg, keys.Refresh):
		// Manual refresh — latest N videos, bypassing the auto-refresh throttle.
		if t.activeChID != "" {
			t.chVidLoad = t.chVidLoad.fetching()
			return t, t.chRefreshCmd(false)
		}
	case key.Matches(msg, keys.ForceRefresh):
		// Manual full refresh — all videos, paginated, bypassing the throttle.
		if t.activeChID != "" {
			t.chVidLoad = t.chVidLoad.fetching()
			return t, t.chRefreshCmd(true)
		}
	case key.Matches(msg, keys.GotoBottom):
		if n > 0 {
			t.vidNav.GotoRow(n - 1)
		}
	case key.Matches(msg, keys.Unsubscribe):
		chIdx := t.listNav.Index()
		if chIdx < len(t.sortedChs) {
			return t.unsubscribeChannel(t.sortedChs[chIdx], true)
		}
	default:
		if cmd, ok := videoActionAt(t.chVideos, idx, msg, keys); ok {
			return t, cmd
		}
	}
	return t, nil
}

func (t Channels) handleEditInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	switch {
	case key.Matches(msg, keys.Escape):
		t.editMode = chEditNone
		t.editInput.Blur()
		return t, nil
	case key.Matches(msg, keys.DrillDown):
		return t.commitEdit()
	default:
		var cmd tea.Cmd
		t.editInput, cmd = t.editInput.Update(msg)
		return t, cmd
	}
}

// commitEdit applies the inline alias/tags edit to the highlighted channel and
// exits edit mode. Edit state is always cleared — even when the cursor is stale
// (empty list) — so the editor never gets stuck open.
func (t Channels) commitEdit() (tea.Model, tea.Cmd) {
	val := strings.TrimSpace(t.editInput.Value())
	mode := t.editMode
	t.editMode = chEditNone
	t.editInput.Blur()

	ch, ok := t.selectedChannel()
	if !ok {
		return t, nil
	}
	if mode == chEditAlias {
		t.subs.SetAlias(ch.ID, val)
		t.rebuildAndSetChNav()
		status := "Alias set: " + val
		if val == "" {
			status = "Alias cleared"
		}
		return t, t.chSetAliasCmd(ch, val, status)
	}
	tags := parseTags(val)
	t.subs.SetTags(ch.ID, tags)
	t.rebuildAndSetChNav()
	return t, t.chSetTagsCmd(ch, tags)
}

func (t Channels) chVidAt(idx int) (domain.Video, bool) {
	if idx >= 0 && idx < len(t.chVideos) {
		return t.chVideos[idx], true
	}
	return domain.Video{}, false
}

func (t Channels) toChannelRows(sorted []domain.Channel) []etable.Row {
	rows := make([]ChannelRow, len(sorted))
	for i := range sorted {
		latest := t.chLatest[sorted[i].ID]
		rows[i] = ChannelRow{
			Channel:            sorted[i],
			Latest:             latest,
			LatestPositionSecs: int(t.aux.Positions[latest.ID] / 1000),
		}
	}
	return videotable.BuildRows(rows, t.chCols)
}

func (t Channels) appendEditInput(body string) string {
	label := "Alias: "
	if t.editMode == chEditTags {
		label = "Tags (comma-separated): "
	}
	return body + "\n" + styles.Bold.Render(label) + t.editInput.View()
}

// toggleBlock optimistically flips the selected channel's blocked state and asks
// Root to run the guarded transition on the backend. Blocking clears the
// subscription state to satisfy the invariant (blocked ⟹ state=none); the pre-
// transition channel is carried in the message so a failed call can revert.
func (t Channels) toggleBlock(ch domain.Channel) (tea.Model, tea.Cmd) {
	blocking := !ch.Blocked
	updated := ch
	updated.Blocked = blocking
	if blocking {
		updated.State = domain.SubNone
	}
	t.subs.Set(updated)
	t.rebuildAndSetChNav()
	return t, func() tea.Msg { return tuipkg.BlockChannelMsg{Channel: ch, Block: blocking} }
}

// viewLabel is the current view's display label (for the header).
func (t Channels) viewLabel() string { return t.view.label() }

// emptyViewText is the "nothing here" message tailored to the active view.
func (t Channels) emptyViewText() string {
	switch t.view {
	case srcBlocked:
		return "No blocked channels."
	case srcRecommended:
		return "No recommended channels. Refresh the Feed tab to populate them."
	case srcMixed:
		return "No channels found."
	case srcStale:
		return "No stale tagged channels."
	default:
		return "No subscribed channels."
	}
}
