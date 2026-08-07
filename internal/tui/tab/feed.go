package tab

import (
	"context"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
)

// feedMode is the Feed tab's source filter, chosen from the mode picker. It
// merges the former Recommended and Subscriptions tabs into one panel: the
// recommended feed, the subscribed-channels feed, or the deduplicated union.
type feedMode int

const (
	feedRecommended feedMode = iota
	feedSubscribed
	feedMixed
	feedStale // latest videos from stale tagged channels ("catch up on channels I forgot about")
)

// needsRec / needsSub / needsStale report which source slices a mode projects
// from, so the tab only loads (and refreshes) the data the active mode shows.
func (m feedMode) needsRec() bool   { return m == feedRecommended || m == feedMixed }
func (m feedMode) needsSub() bool   { return m == feedSubscribed || m == feedMixed }
func (m feedMode) needsStale() bool { return m == feedStale }

// feedModes is the picker's ordered option list; the index matches the feedMode
// value, so picker selection maps straight to a mode.
var feedModes = []struct {
	mode  feedMode
	label string // picker option + header label
}{
	{feedRecommended, "Recommended"},
	{feedSubscribed, "Subscribed"},
	{feedMixed, "Mixed"},
	{feedStale, "Stale"},
}

// parseFeedMode maps a config string to a feedMode, defaulting to recommended
// (preserving the pre-merge "first feed is the recommendations" behavior).
func parseFeedMode(s string) feedMode {
	switch s {
	case "subscribed":
		return feedSubscribed
	case "mixed":
		return feedMixed
	case "stale":
		return feedStale
	default:
		return feedRecommended
	}
}

var feedTarget = tuipkg.TabTarget{Tab: tuipkg.TabFeed}

// feedRecCacheMsg carries the recommended feed cache (fast local seed).
type feedRecCacheMsg struct {
	tuipkg.TabTarget
	videos []domain.Video
}

// feedRecFetchedMsg carries the network Recommended() result to merge into the
// recommended source.
type feedRecFetchedMsg struct {
	tuipkg.TabTarget
	videos []domain.Video
	err    error
}

// feedSubLoadedMsg carries the subscribed-channels videos read from the DB.
type feedSubLoadedMsg struct {
	tuipkg.TabTarget
	videos []domain.Video
	err    error
}

// feedStaleLoadedMsg carries the latest videos from stale tagged channels.
type feedStaleLoadedMsg struct {
	tuipkg.TabTarget
	videos []domain.Video
	err    error
}

// feedHiddenMsg confirms a video was hidden from recommendations (write
// succeeded), so the row can be removed from the recommended source.
type feedHiddenMsg struct {
	tuipkg.TabTarget
	videoID string
}

// Feed is the unified video-feed tab. It keeps two source slices (recommended,
// subscribed) and projects the visible list from whichever the active mode
// selects, so switching modes is instant once each source is loaded. Mixed mode
// dedups the union by video ID.
// feedBackend is the narrow slice of the backend the Feed tab needs: recommended
// feed + feed cache, channel listing, and the shared aux data (watched/positions/
// local/subs). Declared consumer-side so the tab depends only on what it uses (ISP).
type feedBackend interface {
	api.FeedBackend
	api.ChannelBackend
	videotable.AuxBackend
}

type Feed struct {
	ctx      context.Context
	backend  feedBackend
	keys     keymap.KeyMap
	circular bool

	width, height int

	mode   feedMode
	picker modePicker

	// Source data per mode; the displayed feed is projected from these.
	recVideos   []domain.Video
	subVideos   []domain.Video
	staleVideos []domain.Video
	// Per-source fetch lifecycle. A single loadState per source replaces the old
	// loaded/loading bool pairs, so "loaded && loading" can't be represented (L-3).
	recLoad   loadState
	subLoad   loadState
	staleLoad loadState
	staleDays int // stale threshold (days) for the stale mode's channel set

	feed         feed.Feed // the currently displayed, sorted projection
	spinnerFrame string
	nav          videotable.TableNav
	cols         []videotable.ColumnDef[videotable.VideoData]
	aux          videotable.AuxData

	sort sortState

	// pendingUnsub holds the videos removed for an in-flight unsubscribe,
	// keyed by channel ID, so they can be restored if the backend call fails.
	pendingUnsub map[string][]domain.Video
}

// feedColumns is the full, natural-order column set for the Feed video list.
// Extracted so the per-panel column selector and tab.PanelColumnKeys catalog
// share one source of truth.
func feedColumns() []videotable.ColumnDef[videotable.VideoData] {
	return []videotable.ColumnDef[videotable.VideoData]{
		videotable.NumCol[videotable.VideoData](), videotable.IndicatorCol[videotable.VideoData](), videotable.TitleFlexCol[videotable.VideoData](),
		videotable.ChannelCol[videotable.VideoData](), videotable.WatchedCol[videotable.VideoData](), videotable.DurationCol[videotable.VideoData](), videotable.ViewsCol[videotable.VideoData](), videotable.DateCol[videotable.VideoData](),
	}
}

// FeedOpts carries the Feed-tab configuration. Grouping the two adjacent string
// fields (Mode, Sort) behind names prevents silent transposition at the call site.
type FeedOpts struct {
	Mode      string // default feed mode (recommended/subscribed/mixed/stale)
	Sort      string // default sort mode
	StaleDays int    // age threshold for "stale"
}

func NewFeed(ctx context.Context, backend feedBackend, keys keymap.KeyMap, circular bool, opts FeedOpts, wantCols ...string) Feed {
	cols := videotable.SelectColumns(feedColumns(), wantCols)
	mode := parseFeedMode(opts.Mode)
	labels := make([]string, len(feedModes))
	for i := range feedModes {
		labels[i] = feedModes[i].label
	}
	return Feed{
		ctx:      ctx,
		backend:  backend,
		keys:     keys,
		circular: circular,
		mode:     mode,
		picker:   newModePicker("Mode", labels, circular),
		// Seed the loading flags for the default mode's sources in the constructor
		// (not Init, whose value receiver would discard the mutation) so the first
		// View shows the spinner instead of "No videos". (L-8)
		recLoad:      initLoad(mode.needsRec()),
		subLoad:      initLoad(mode.needsSub()),
		staleLoad:    initLoad(mode.needsStale()),
		staleDays:    opts.StaleDays,
		feed:         feed.New(nil),
		nav:          videotable.NewTableNav(cols, circular, 2),
		cols:         cols,
		sort:         newSortState(sortModeOr(opts.Sort, feed.SortDate), videotable.ColumnKeys(cols)),
		pendingUnsub: make(map[string][]domain.Video),
	}
}

func (t Feed) ID() tuipkg.TabID      { return tuipkg.TabFeed }
func (t Feed) Title() string         { return "Feed" }
func (t Feed) InterceptsInput() bool { return t.picker.isOpen() }
func (t Feed) SelectedVideo() (domain.Video, bool) {
	return t.feed.At(t.nav.Index())
}

// Loading reports whether a fetch the active mode depends on is in flight.
func (t Feed) Loading() bool {
	return (t.mode.needsRec() && t.recLoad.inFlight()) ||
		(t.mode.needsSub() && t.subLoad.inFlight()) ||
		(t.mode.needsStale() && t.staleLoad.inFlight())
}

func (t Feed) ShortHelp() []key.Binding {
	base := []key.Binding{t.keys.Play, t.keys.Download, t.keys.PanelMode}
	if t.mode.needsRec() {
		base = append(base, t.keys.HideVideo)
	}
	if t.mode.needsSub() {
		base = append(base, t.keys.Unsubscribe)
	}
	return append(base, t.keys.CopyURL, t.keys.VideoInfo, t.keys.SortChord)
}

func (t Feed) Init() tea.Cmd {
	cmds := []tea.Cmd{videotable.LoadAuxDataCmd(t.ctx, t.backend)}
	if t.mode.needsRec() {
		cmds = append(cmds, t.recCacheCmd())
	}
	if t.mode.needsSub() {
		cmds = append(cmds, t.subLoadCmd())
	}
	if t.mode.needsStale() {
		cmds = append(cmds, t.staleLoadCmd())
	}
	return tea.Batch(cmds...)
}

func (t Feed) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.nav.Resize(m.Width, m.Height)
		t.setRows()

	case tuipkg.SpinnerFrameMsg:
		t.spinnerFrame = m.Frame

	case tuipkg.PollTickMsg:
		// Reload subscribed videos so a background crawl's new rows appear live.
		// The recommended feed isn't fed by backfill, so only the sub view polls.
		if t.mode.needsSub() {
			return t, t.subLoadCmd()
		}
		return t, nil

	case feedRecCacheMsg:
		return t.onRecCache(m)

	case feedRecFetchedMsg:
		return t.onRecFetched(m)

	case feedSubLoadedMsg:
		return t.onSubLoaded(m)

	case feedStaleLoadedMsg:
		return t.onStaleLoaded(m)

	case feedHiddenMsg:
		t.recVideos = feed.RemoveVideoByID(t.recVideos, m.videoID)
		t.rebuild()

	case videotable.AuxDataMsg:
		t.aux = m
		t.setRows()

	case tuipkg.UnsubscribeResultMsg:
		return t.onUnsubscribeResult(m), nil

	case tea.KeyPressMsg:
		return t.handleKey(m)
	}
	return t, nil
}

// onRecCache seeds the recommended source from the cache and kicks off a network
// refresh (recLoad stays srcLoading until the fetch lands).
func (t Feed) onRecCache(m feedRecCacheMsg) (tea.Model, tea.Cmd) {
	t.recVideos = m.videos
	t.rebuild()
	return t, t.recFetchCmd()
}

// onRecFetched merges the network recommended result into the source, persists
// the cache, and clears the recommended loading flag.
func (t Feed) onRecFetched(m feedRecFetchedMsg) (tea.Model, tea.Cmd) {
	t.recLoad = srcLoaded
	if m.err != nil {
		t.rebuild()
		return t, errMsg("feed: " + m.err.Error())
	}
	t.recVideos = feed.MergeVideos(t.recVideos, m.videos)
	t.rebuild()
	return t, t.recSaveCacheCmd()
}

// onSubLoaded stores the subscribed source read from the DB.
func (t Feed) onSubLoaded(m feedSubLoadedMsg) (tea.Model, tea.Cmd) {
	t.subLoad = srcLoaded
	if m.err != nil {
		t.rebuild()
		return t, errMsg("feed: " + m.err.Error())
	}
	t.subVideos = m.videos
	t.rebuild()
	return t, nil
}

// onStaleLoaded stores the stale-channels video source read from the DB.
func (t Feed) onStaleLoaded(m feedStaleLoadedMsg) (tea.Model, tea.Cmd) {
	t.staleLoad = srcLoaded
	if m.err != nil {
		t.rebuild()
		return t, errMsg("feed: " + m.err.Error())
	}
	t.staleVideos = m.videos
	t.rebuild()
	return t, nil
}

// onUnsubscribeResult restores an optimistically-removed channel's videos if the
// backend unsubscribe failed (the result is broadcast to every tab).
func (t Feed) onUnsubscribeResult(m tuipkg.UnsubscribeResultMsg) tea.Model {
	removed, ok := t.pendingUnsub[m.Channel.ID]
	delete(t.pendingUnsub, m.Channel.ID)
	if m.Err != nil && ok {
		t.subVideos = feed.MergeVideos(t.subVideos, removed)
		t.rebuild()
	}
	return t
}

func (t Feed) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if t.picker.isOpen() {
		return t.handlePickerKey(msg)
	}

	if t.sort.handleChord(msg, t.keys.Sort, func(int) { t.rebuild() }) {
		return t, nil
	}

	if t.nav.HandleNav(msg, t.keys, t.feed.Len()) {
		return t, nil
	}

	keys := t.keys
	// List-level actions that don't need a highlighted video.
	switch {
	case key.Matches(msg, keys.PanelMode):
		t.picker.openAt(int(t.mode))
		return t, nil
	case key.Matches(msg, keys.Refresh):
		cmd := t.refreshCmd(false) // pointer receiver mutates t; call before return
		return t, cmd
	case key.Matches(msg, keys.ForceRefresh):
		cmd := t.refreshCmd(true)
		return t, cmd
	case key.Matches(msg, keys.SortChord):
		t.sort.chordActive = true
		return t, nil
	}

	// Everything else operates on the highlighted video.
	v, ok := t.feed.At(t.nav.Index())
	if !ok {
		return t, nil
	}
	switch {
	case key.Matches(msg, keys.HideVideo):
		if t.mode.needsRec() {
			return t, t.hideVideoCmd(v)
		}
	case key.Matches(msg, keys.Unsubscribe):
		if t.mode.needsSub() {
			return t.unsubscribe(v)
		}
	default:
		if cmd, ok := HandleVideoAction(msg, v, keys); ok {
			return t, cmd
		}
	}
	return t, nil
}

// handlePickerKey drives the mode picker: applying the chosen mode loads any
// source it newly needs and re-projects the feed from the top.
func (t Feed) handlePickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch t.picker.handleKey(msg, t.keys) {
	case pickerCommitted:
		t.mode = feedModes[t.picker.selection()].mode
		cmd := t.ensureModeCmd()
		t.rebuild()
		t.nav.GotoRow(0)
		return t, cmd
	case pickerCanceled:
		return t, nil
	}
	return t, nil
}

// unsubscribe optimistically drops the channel's videos from the subscribed
// source and asks Root to run the backend call; UnsubscribeResultMsg restores
// them if it fails.
func (t Feed) unsubscribe(v domain.Video) (tea.Model, tea.Cmd) {
	ch := domain.Channel{ID: v.ChannelID, Name: v.Channel}
	t.pendingUnsub[ch.ID] = channelVideos(t.subVideos, ch)
	t.subVideos = feed.RemoveChannelVideos(t.subVideos, ch)
	t.rebuild()
	return t, func() tea.Msg { return tuipkg.UnsubscribeMsg{Channel: ch} }
}

// ── projection ────────────────────────────────────────────────────────────────

// visibleVideos projects the active mode's video list from the source slices.
// Mixed dedups the union by video ID (MergeVideos, subscribed wins on conflict).
func (t Feed) visibleVideos() []domain.Video {
	switch t.mode {
	case feedSubscribed:
		return t.subVideos
	case feedMixed:
		return feed.MergeVideos(t.recVideos, t.subVideos)
	case feedStale:
		return t.staleVideos
	default:
		return t.recVideos
	}
}

// rebuild recomputes the visible projection, sorts it, and refreshes the table
// rows, keeping the cursor on its video across the change.
func (t *Feed) rebuild() {
	prevID := ""
	if v, ok := t.feed.At(t.nav.Index()); ok {
		prevID = v.ID
	}
	t.feed.SetVideos(t.visibleVideos())
	t.feed.Sort(t.sort.mode)
	t.setRows()
	if prevID != "" {
		for i, v := range t.feed.Videos() {
			if v.ID == prevID {
				t.nav.GotoRow(i)
				break
			}
		}
	}
}

// setRows re-enriches and re-lays the displayed feed into the nav table.
func (t *Feed) setRows() {
	videotable.SetVideoRows(&t.nav, t.feed.Videos(), t.aux, t.cols)
}
