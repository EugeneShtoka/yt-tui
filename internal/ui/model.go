package ui

import (
	"fmt"
	"image"
	"sort"
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/channels"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	"github.com/EugeneShtoka/yt-tui/internal/domain/library"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/player"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

const (
	tabRecommended   = 0
	tabSubscriptions = 1
	tabChannels      = 2
	tabPlaylists     = 3
	tabSearch        = 4
	tabDownloading   = 5
	tabLocal         = 6
	tabHistory       = 7
	tabActivity      = 8
	numTabIDs        = 9
)

// ytWatchLaterID is YouTube's internal Watch Later playlist ID.
const ytWatchLaterID = "WL"

// tabMeta is the single source of truth for tab identity.
// name: lowercase key used in config, commands, and debug logs.
// display: title-case label shown in the tab bar.
var tabMeta = [numTabIDs]struct {
	name    string
	display string
}{
	tabRecommended:   {"recommended", "Recommended"},
	tabSubscriptions: {"subscriptions", "Subscriptions"},
	tabChannels:      {"channels", "Channels"},
	tabPlaylists:     {"playlists", "Playlists"},
	tabSearch:        {"search", "Search"},
	tabDownloading:   {"downloading", "Downloading"},
	tabLocal:         {"local", "Local"},
	tabHistory:       {"history", "History"},
	tabActivity:      {"activity", "Activity"},
}

var tabNames [numTabIDs]string
var tabIDByName map[string]int

func init() {
	tabIDByName = make(map[string]int, numTabIDs)
	for id, m := range tabMeta {
		tabNames[id] = m.display
		tabIDByName[m.name] = id
	}
}

// ContextID identifies the UI context for key dispatch and sort-matrix filtering.
type ContextID int

const (
	CtxVideoList     ContextID = iota // rec, subs-all, channel drill-down vids, playlist vids
	CtxChannelList                    // subscriptions channel pane (left)
	CtxTagList                        // channels tab: tag list (tags-grouped view)
	CtxSearchVideo                    // search: video rows
	CtxSearchChannel                  // search: channel rows
	CtxPlaylistList                   // playlists top level
	CtxLocal                          // local tab
	CtxDownloading                    // downloading tab
	CtxHistoryVideo                   // history: video entry
	CtxHistorySearch                  // history: search entry
)

const (
	pseudoTagAll      = "\x00all"      // pseudo-tag: show all channels
	pseudoTagUntagged = "\x00untagged" // pseudo-tag: show channels with no tags
)

// sortContextSupport maps sort-action names to the contexts that support them.
// This drives both hint filtering and sort-chord dispatch.
var sortContextSupport = map[string][]ContextID{
	"date":        {CtxVideoList, CtxChannelList, CtxSearchVideo, CtxLocal},
	"views":       {CtxVideoList, CtxChannelList, CtxSearchVideo, CtxLocal},
	"name":        {CtxVideoList, CtxChannelList, CtxSearchVideo, CtxLocal},
	"channel":     {CtxVideoList, CtxChannelList, CtxSearchVideo, CtxLocal},
	"duration":    {CtxVideoList, CtxChannelList, CtxSearchVideo, CtxLocal},
	"subscribers": {CtxChannelList},
	"tags":        {CtxChannelList},
}

func sortSupported(action string, ctx ContextID) bool {
	for _, c := range sortContextSupport[action] {
		if c == ctx {
			return true
		}
	}
	return false
}

const (
	subChSortDate     = 0 // sort channels by latest video date (newest first)
	subChSortName     = 1 // sort channels alphabetically by channel name
	subChSortSubs     = 2 // sort channels by subscriber count (desc)
	subChSortViews    = 3 // sort channels by latest video view count (desc)
	subChSortVidName  = 4 // sort channels by latest video title (asc)
	subChSortDuration = 5 // sort channels by latest video duration (desc)
	subChSortTags     = 6 // sort channels alphabetically by first tag (untagged last)
)

// Video list sort modes (used by each tab view's sort field). Canonical
// definitions live in internal/feed; these aliases keep the view code terse.
const (
	vidSortViews    = feed.SortViews
	vidSortDate     = feed.SortDate
	vidSortName     = feed.SortName
	vidSortNone     = feed.SortNone
	vidSortChannel  = feed.SortChannel
	vidSortDuration = feed.SortDuration
)

// Model is the root bubbletea model.
type Model struct {
	cfg        *config.Config
	backend    api.Backend
	db         Store              // backendStore wrapper; derived from backend in NewModel
	downloader *downloader.Downloader // kept for Items() / WaitForEvent() display primitives

	width  int
	height int

	// tabs holds the ordered list of visible tab IDs, derived from config.
	tabs      []int
	activeTab int // one of the tabXxx constants above

	// ── Recommended ─────────────────────────────────────────────────────────
	// P5 item #5: the feed slice + fetch-lifecycle flags + filter pipeline are
	// owned by recFeed (internal/feed.Feed). recommendedView keeps only the
	// private cursor/scroll/sort.
	recFeed     feed.Feed
	recommended recommendedView

	// ── Subscriptions ────────────────────────────────────────────────────────
	// subFeed — all-channel feed (Subscriptions tab); shared with Channels'
	// tagVideos. P5 item #5 data-owner: it owns the slice; its display loading
	// state is derived from subChLoading (it has no independent fetch lifecycle).
	subFeed       feed.Feed
	subscriptions subscriptionsView
	// ── Channels ─────────────────────────────────────────────────────────────
	// P4 slice: private cursor/scroll/pane/mode/sort live in channelsView; the
	// channel/video slices, loading flags, activeID, and latest map stay here
	// (shared / async-written — docs/TABVIEW_DESIGN.md, Finding 2).
	channels           channelsView
	subs               channels.ChannelSet
	subChLoading       bool
	subChLoaded        bool
	subChVideos        []domain.Video
	subChVidLoading    bool
	subChVidRefreshing bool // has cached data; background fetch running
	subChActiveID      string
	subChLatest        map[string]domain.Video // channelID → latest known video

	// ── Channels: alias/tag editing ───────────────────────────────────────────
	// Handled by the pre-dispatch edit-input gate (handleChannelEditInput), so
	// these stay router-owned rather than moving into channelsView.
	subChEditKind  int // 1=editing alias, 2=editing tags (valid when mode==modeChannelEdit)
	subChEditInput textinput.Model

	// ── Playlists ────────────────────────────────────────────────────────────
	playlists            []domain.Playlist   // local playlists (fallback when no YT)
	ytPlaylists          []domain.YTPlaylist // YouTube playlists (loaded from YT)
	ytPlLoading          bool
	ytPlLoaded           bool
	ytClient             *youtube.YTClient         // nil until browser cookies extracted
	playlist             playlistsView             // P4 slice: cursor/scroll/pane/sort for both panes
	playlistVidCache     map[string][]domain.Video // per-playlist video cache (written by async fetches)
	playlistVidLoading   bool
	createModeYT         bool // true = creating a YouTube playlist (not local)
	createTypeSel        int  // 0 = local, 1 = YouTube
	createInput          textinput.Model
	addOverlaySel        int
	addVideo             domain.Video
	addAfterCreate       bool
	addOverlayCreateMode bool
	addOverlayCreateYT   bool
	addOverlayInput      textinput.Model

	// ── Search ────────────────────────────────────────────────────────────────
	// P4 slice: Search's private cursor/scroll/sort (both result modes) live in
	// its own view struct. The result slices, drill-down selection, and loading
	// flags are written by async fetches, so they stay here (router).
	search        searchView
	searchInput   textinput.Model
	searchVideos  []domain.Video
	searchLoading bool
	lastQuery     string
	searchHistory []string // past queries, newest first
	searchHistIdx int      // -1 = not navigating; 0+ = index into searchHistory
	searchDraft   string   // saved current input text when history nav starts

	// ── Downloading ───────────────────────────────────────────────────────────
	// P4 slice: Downloading's private cursor/scroll lives in its own view struct.
	downloading downloadingView

	// ── Local ────────────────────────────────────────────────────────────────
	// P4 slice: Local's private cursor/scroll/sort lives in its own view struct.
	// P5 item #5: the downloaded-video slice + its by-ID index are owned by
	// library.Library (written across tabs; reloaded via library.Set).
	library library.Library
	local   localView

	// ── History ──────────────────────────────────────────────────────────────
	// P4 slice: History's state lives in its own view struct.
	history historyView

	// ── Activity ─────────────────────────────────────────────────────────────
	// P4 reference slice: Activity's state lives in its own view struct.
	activity activityView

	// ── Local filter ─────────────────────────────────────────────────────────
	localFilter       string
	localFilterInput  textinput.Model
	localFilterCursor int

	// ── Input mode ────────────────────────────────────────────────────────────
	// Single source of truth for which text-input mode owns the keyboard; see
	// input_mode.go. Replaces the former cmdMode/searchFocused/localFilterFocused/
	// createMode/createTypeMode bools.
	mode inputMode

	// ── Command mode (:cmd) ───────────────────────────────────────────────────
	cmdInput        textinput.Model
	cmdCompletions  []string
	cmdCompIdx      int
	cmdLastTabValue string

	// ── Search: channel results + drill-down ─────────────────────────────────
	searchChannels  []domain.Channel
	searchChSel     *domain.Channel
	searchChVideos  []domain.Video
	searchChLoading bool

	// ── Shared ───────────────────────────────────────────────────────────────
	spinner   spinner.Model
	status    string
	statusErr bool
	statusAt  time.Time
	showHelp  bool
	keys      keyMap

	// ── Vim-style goto navigation ─────────────────────────────────────────
	numPrefix string // accumulated digit keys (e.g. "42" before G or gg)
	gPending  bool   // true after first GotoPrefix press, waiting for second

	// ── Chord system ──────────────────────────────────────────────────────
	pendingChord string      // chord trigger key currently waiting for completion
	chordBuffer  string      // keys accumulated after the trigger (supports multi-char)
	chordCache   *[]chordDef // built once in NewModel; shared across BubbleTea value copies

	// ── Recommended: hide/blacklist state ────────────────────────────────
	streamedVideoIDs map[string]bool  // video IDs with any play/stream history event
	videoPositions   map[string]int64 // last known position ms for any video
	recHidden        map[string]bool  // video IDs hidden from recommended

	// ── Downloading: play-after-download ─────────────────────────────────
	playAfterDownload map[string]bool

	// ── Playback resume ───────────────────────────────────────────────────
	playerBackend     player.Backend
	playingVideoID    string             // ID of the video currently playing (for position saves)
	playingSBSegments []domain.SBSegment // SponsorBlock segments for the current local file (empty = no conversion)

	// ── Pending direct overlay (chapters/links opened without info panel) ──
	pendingDirectOverlay string // "links" or "chapters"; cleared after VideoDetailsMsg handled

	// ── Overlay stack ─────────────────────────────────────────────────────
	// Modal overlays layered above tab content; see overlay.go. Replaces the
	// former addOverlay/vidDetailOverlay/linkOverlay/chapterOverlay bools. The
	// links/chapters overlays stack on top of video-detail, so a single scalar
	// (unlike inputMode) can't represent them — hence a stack.
	overlays []overlayKind

	// ── Video detail overlay ──────────────────────────────────────────────
	vidDetailVideo         *domain.VideoDetails
	vidDetailLoading       bool
	vidDetailDescVS        int               // description scroll start line
	vidDetailThumb         image.Image       // nil until loaded; stays nil if fetch fails
	vidDetailLinks         *[]domain.Link    // nil = not yet parsed; &[]domain.Link{} = parsed, none found
	vidDetailChapters      *[]domain.Chapter // nil = not available; populated from yt-dlp metadata
	vidDetailDescLines     []string          // pre-wrapped description lines; nil until video is set
	vidDetailThumbB64      string            // pre-encoded PNG base64 for Kitty; empty until loaded
	vidDetailThumbRendered string            // pre-rendered half-block string for non-Kitty terminals
	vidDetailKittyOverlay  string            // full Kitty sequence; recomputed only on thumbnail load or resize

	// ── Link list overlay (opened from video detail) ───────────────────────
	linkOverlaySel  int
	linkOverlayURLs []domain.Link

	// ── Chapter list overlay (opened from video detail) ────────────────────
	chapterOverlaySel   int
	chapterOverlayItems []domain.Chapter
}

func buildTabs(cfg *config.Config) []int {
	names := cfg.Tabs
	if len(names) == 0 {
		names = config.DefaultTabs
	}
	var tabs []int
	seen := map[int]bool{}
	for _, name := range names {
		if id, ok := tabIDByName[name]; ok && !seen[id] {
			tabs = append(tabs, id)
			seen[id] = true
		}
	}
	if len(tabs) == 0 {
		tabs = []int{tabRecommended, tabSubscriptions, tabChannels, tabPlaylists,
			tabSearch, tabDownloading, tabLocal, tabHistory}
	}
	return tabs
}

func NewModel(cfg *config.Config, backend api.Backend, dl *downloader.Downloader) Model {
	database := backendStore{backend}
	si := textinput.New()
	si.Placeholder = "Search YouTube..."
	si.CharLimit = 200
	si.Width = 60

	ci := textinput.New()
	ci.Placeholder = "Playlist name..."
	ci.CharLimit = 80
	ci.Width = 40

	ei := textinput.New()
	ei.CharLimit = 120
	ei.Width = 50

	oi := textinput.New()
	oi.CharLimit = 80
	oi.Width = 36

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	localVideos, _ := database.LocalVideos()
	playlists, _ := database.Playlists()

	// Drop cached recommended entries that have no channel_id so the
	// next fetch repopulates them correctly.
	_ = database.PurgeFeedCacheMissingChannelID("recommended")

	// Load feed caches synchronously — fast DB reads, shown immediately.
	recCache, _ := database.GetFeedCache("recommended")

	tabs := buildTabs(cfg)
	firstTab := tabRecommended
	if len(tabs) > 0 {
		firstTab = tabs[0]
	}

	recHidden, _ := database.HiddenRecVideoIDs()
	if recHidden == nil {
		recHidden = make(map[string]bool)
	}

	// Load full channel list from DB for immediate display.
	cachedChannels, _ := database.GetSubscribedChannels()

	subs := channels.New(cachedChannels, database, nil)
	recCache = feed.FilterSubscribed(recCache, subs.Index())

	// Load subscriptions all-video list from channel_videos aggregate.
	channelIDs := make([]string, 0, len(cachedChannels))
	for _, ch := range cachedChannels {
		if ch.ID != "" {
			channelIDs = append(channelIDs, ch.ID)
		}
	}
	subVideos, _ := database.GetAllChannelVideos(channelIDs)
	feed.SortVideos(subVideos, vidSortDate)

	// Load YouTube playlists from DB for immediate display.
	cachedYTPlaylists, _ := database.GetYTPlaylists()

	// Load latest-video-per-channel from channel_videos for immediate sort/display.
	chLatest, _ := database.GetChannelLatestAll()
	if chLatest == nil {
		chLatest = make(map[string]domain.Video)
	}

	backend, _ := player.New(cfg)

	m := Model{
		cfg:               cfg,
		backend:           backend,
		db:                database,
		downloader:        dl,
		tabs:              tabs,
		activeTab:         firstTab,
		recFeed:           feed.NewStarting(recCache),
		subFeed:           feed.New(subVideos),
		searchInput:       si,
		createInput:       ci,
		subChEditInput:    ei,
		addOverlayInput:   oi,
		channels:          channelsView{sort: subChSortDate, vidSort: vidSortDate, tagSort: vidSortDate},
		spinner:           sp,
		library:           library.New(localVideos),
		streamedVideoIDs:  mustWatchedIDs(database),
		videoPositions:    mustVideoPositions(database),
		recHidden:         recHidden,
		subs:              subs,
		subChLoaded:       len(cachedChannels) > 0,
		subChLatest:       chLatest,
		localFilterInput:  textinput.New(),
		cmdInput:          func() textinput.Model { t := textinput.New(); t.Prompt = ""; return t }(),
		playAfterDownload: make(map[string]bool),
		playlists:         playlists,
		ytPlaylists:       cachedYTPlaylists,
		ytPlLoaded:        len(cachedYTPlaylists) > 0,
		playlistVidCache:  make(map[string][]domain.Video),
		keys:              buildKeyMap(cfg.Keybindings),
		playerBackend:     backend,
		recommended:       recommendedView{sort: vidSortViews},
		subscriptions:     subscriptionsView{sort: vidSortDate},
		search:            searchView{sort: vidSortNone},
		local:             localView{sort: vidSortNone},
		playlist:          playlistsView{sort: vidSortNone},
		searchHistIdx:     -1,
	}
	chords := m.buildChordDefs()
	m.chordCache = &chords
	return m
}

// sortChannelSlice returns a sorted copy of the given channel slice.
func (m Model) sortChannelSlice(channels []domain.Channel) []domain.Channel {
	out := make([]domain.Channel, len(channels))
	copy(out, channels)
	switch m.channels.sort {
	case subChSortDate:
		sort.SliceStable(out, func(i, j int) bool {
			di := m.subChLatest[out[i].ID].UploadDate
			dj := m.subChLatest[out[j].ID].UploadDate
			return di > dj
		})
	case subChSortName:
		sort.SliceStable(out, func(i, j int) bool {
			return strings.ToLower(out[i].DisplayName()) < strings.ToLower(out[j].DisplayName())
		})
	case subChSortSubs:
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].Subscribers > out[j].Subscribers
		})
	case subChSortViews:
		sort.SliceStable(out, func(i, j int) bool {
			return m.subChLatest[out[i].ID].ViewCount > m.subChLatest[out[j].ID].ViewCount
		})
	case subChSortVidName:
		sort.SliceStable(out, func(i, j int) bool {
			ti := strings.ToLower(m.subChLatest[out[i].ID].Title)
			tj := strings.ToLower(m.subChLatest[out[j].ID].Title)
			return ti < tj
		})
	case subChSortDuration:
		sort.SliceStable(out, func(i, j int) bool {
			return m.subChLatest[out[i].ID].Duration > m.subChLatest[out[j].ID].Duration
		})
	case subChSortTags:
		sort.SliceStable(out, func(i, j int) bool {
			ti := firstTag(out[i].Tags)
			tj := firstTag(out[j].Tags)
			if ti != tj {
				return ti < tj
			}
			return strings.ToLower(out[i].DisplayName()) < strings.ToLower(out[j].DisplayName())
		})
	}
	return out
}

func firstTag(tags []string) string {
	if len(tags) == 0 {
		return "\xff" // untagged channels sort last
	}
	return strings.ToLower(tags[0])
}

// sortedChannels returns all subscribed channels in the current sort order.
func (m Model) sortedChannels() []domain.Channel {
	return m.sortChannelSlice(m.subs.Channels())
}

// channelsInTag returns channels belonging to the given tag (supports pseudo-tags).
func (m Model) channelsInTag(tag string) []domain.Channel {
	switch tag {
	case pseudoTagAll:
		return m.subs.Channels()
	case pseudoTagUntagged:
		var out []domain.Channel
		for _, ch := range m.subs.Channels() {
			if len(ch.Tags) == 0 {
				out = append(out, ch)
			}
		}
		return out
	default:
		var out []domain.Channel
		for _, ch := range m.subs.Channels() {
			for _, t := range ch.Tags {
				if t == tag {
					out = append(out, ch)
					break
				}
			}
		}
		return out
	}
}

// sortedChannelsInTag returns channels for the given tag in the current sort order.
func (m Model) sortedChannelsInTag(tag string) []domain.Channel {
	return m.sortChannelSlice(m.channelsInTag(tag))
}

// allTags returns all unique user-defined tags, sorted alphabetically.
func (m Model) allTags() []string {
	seen := map[string]bool{}
	for _, ch := range m.subs.Channels() {
		for _, t := range ch.Tags {
			if t != "" {
				seen[t] = true
			}
		}
	}
	tags := make([]string, 0, len(seen))
	for t := range seen {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags
}

// tagListItems returns only real user-defined tags (no pseudo-tags) for the tag list pane.
func (m Model) tagListItems() []string {
	return m.allTags()
}

// tagDisplayName returns a human-readable label for a tag.
func tagDisplayName(tag string) string {
	switch tag {
	case pseudoTagAll:
		return "All channels"
	case pseudoTagUntagged:
		return "Untagged"
	}
	return tag
}

// tagVideos returns videos from the subscriptions feed (m.subFeed) that belong to channels in the selected tag,
// sorted by m.channels.tagSort. The returned slice is always a fresh copy.
func (m Model) tagVideos() []domain.Video {
	chans := m.channelsInTag(m.channels.tagSel)
	if len(chans) == 0 {
		return nil
	}
	idSet := make(map[string]bool, len(chans))
	for _, ch := range chans {
		if ch.ID != "" {
			idSet[ch.ID] = true
		}
	}
	var out []domain.Video
	for _, v := range m.subFeed.Videos() {
		if idSet[v.ChannelID] {
			out = append(out, v)
		}
	}
	feed.SortVideos(out, m.channels.tagSort)
	return out
}

func mustWatchedIDs(d Store) map[string]bool {
	ids, _ := d.WatchedVideoIDs()
	if ids == nil {
		return make(map[string]bool)
	}
	return ids
}

func mustVideoPositions(d Store) map[string]int64 {
	pos, _ := d.AllVideoPositions()
	if pos == nil {
		return make(map[string]int64)
	}
	return pos
}

type ytClientInitMsg struct {
	client *youtube.YTClient
	err    error
}

func initYTClient(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		client, err := youtube.NewYTClient(cfg)
		return ytClientInitMsg{client: client, err: err}
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		cmdFetchRecommended(m.backend),
		cmdFetchSubscribedChannelsBackground(m.backend), // silently populate filter on startup
		m.downloader.WaitForEvent(),
		m.spinner.Tick,
		positionTick(),
		initYTClient(m.cfg),
	)
}

func positionTick() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return positionTickMsg{} })
}

type positionTickMsg struct{}

func (m *Model) setStatus(msg string, isErr bool) {
	m.status = msg
	m.statusErr = isErr
	m.statusAt = time.Now()
}

// playlistCount returns the total number of items in the playlist pane.
func (m Model) playlistCount() int {
	if m.ytPlLoaded {
		return len(m.ytPlaylists) + len(m.playlists)
	}
	return len(m.playlists)
}

// selectedPlaylistKey returns the cache key for the currently highlighted playlist.
func (m Model) selectedPlaylistKey() string {
	if m.ytPlLoaded && m.playlist.cursor < len(m.ytPlaylists) {
		return m.ytPlaylists[m.playlist.cursor].ID
	}
	localIdx := m.playlist.cursor
	if m.ytPlLoaded {
		localIdx -= len(m.ytPlaylists)
	}
	if localIdx >= 0 && localIdx < len(m.playlists) {
		return fmt.Sprintf("local:%d", m.playlists[localIdx].ID)
	}
	return ""
}

// selectedPlaylistName returns the display name for the currently highlighted playlist entry.
func (m Model) selectedPlaylistName() string {
	if m.ytPlLoaded && m.playlist.cursor < len(m.ytPlaylists) {
		return m.ytPlaylists[m.playlist.cursor].Title
	}
	localIdx := m.playlist.cursor
	if m.ytPlLoaded {
		localIdx -= len(m.ytPlaylists)
	}
	if localIdx >= 0 && localIdx < len(m.playlists) {
		return m.playlists[localIdx].Name
	}
	return ""
}

// selectedPlaylistIsYT returns true when the selected playlist is YouTube-backed.
func (m Model) selectedPlaylistIsYT() bool {
	return m.ytPlLoaded && m.ytClient != nil && m.playlist.cursor < len(m.ytPlaylists)
}

func (m *Model) currentTabIndex() int {
	for i, id := range m.tabs {
		if id == m.activeTab {
			return i
		}
	}
	return 0
}

func filterText(videos []domain.Video, q string) []domain.Video {
	if q == "" {
		return videos
	}
	lower := strings.ToLower(q)
	out := make([]domain.Video, 0, len(videos))
	for _, v := range videos {
		if strings.Contains(strings.ToLower(v.Title), lower) ||
			strings.Contains(strings.ToLower(v.Channel), lower) {
			out = append(out, v)
		}
	}
	return out
}

func (m *Model) localFilteredVideos() []domain.Video {
	var raw []domain.Video
	switch m.activeTab {
	case tabRecommended:
		raw = m.recFeed.Videos()
	case tabSubscriptions:
		raw = m.subFeed.Videos()
	case tabChannels:
		if m.channels.tagsMode && m.channels.pane == 1 {
			raw = m.tagVideos()
		} else if !m.channels.tagsMode && m.channels.pane == 1 {
			raw = m.subChVideos
		}
	case tabSearch:
		if m.searchChSel != nil {
			raw = m.searchChVideos
		} else {
			raw = m.searchVideos
		}
	case tabLocal:
		// local tab uses db.LocalVideo, handled separately
	}
	return filterText(raw, m.localFilter)
}

func (m *Model) currentVideo() (domain.Video, bool) {
	if m.localFilter != "" {
		filtered := m.localFilteredVideos()
		if i := m.localFilterCursor; i >= 0 && i < len(filtered) {
			return filtered[i], true
		}
		return domain.Video{}, false
	}
	return m.activeView().currentVideo(m.viewCtx())
}

func (m *Model) parseNumPrefix() int {
	if m.numPrefix == "" {
		return 0
	}
	n := 0
	for _, ch := range m.numPrefix {
		n = n*10 + int(ch-'0')
	}
	return n
}

func (m *Model) jumpToLine(idx int) { m.activeView().jumpTo(idx, m.viewCtx()) }
func (m *Model) jumpToLast()        { m.activeView().jumpToLast(m.viewCtx()) }

func clamp(v, max int) int {
	if v < 0 {
		return 0
	}
	if max == 0 {
		return 0
	}
	if v >= max {
		return max - 1
	}
	return v
}

// thumbDimensions returns the (width, height) in terminal cells for the
// video-detail thumbnail area, accounting for the actual image aspect ratio.
func (m Model) thumbDimensions() (w, h int) {
	thumbW := vidDetailPanelW - 2
	thumbH := (thumbW*9 + 15) / 16 / 2
	if thumbH < 1 {
		thumbH = 1
	}
	if m.vidDetailThumb != nil {
		b := m.vidDetailThumb.Bounds()
		iw := b.Max.X - b.Min.X
		ih := b.Max.Y - b.Min.Y
		if iw > 0 && ih > 0 {
			if h := (thumbW*ih + iw - 1) / iw / 2; h >= 1 {
				thumbH = h
			}
		}
	}
	return thumbW, thumbH
}

func truncate(s string, n int) string {
	if runewidth.StringWidth(s) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	runes := []rune(s)
	var w, i int
	for i < len(runes) {
		if w+runewidth.RuneWidth(runes[i]) > n-1 {
			break
		}
		w += runewidth.RuneWidth(runes[i])
		i++
	}
	return string(runes[:i]) + "…"
}

// currentContext returns the ContextID for the currently focused UI area.
func (m Model) currentContext() ContextID {
	if v := m.activeView(); v != nil {
		return v.context(m.viewCtx())
	}
	return CtxVideoList
}

// contextSupportsNewPlaylist reports whether a new-playlist chord should activate.
func (m Model) contextSupportsNewPlaylist() bool {
	return m.activeTab == tabPlaylists && m.playlist.pane == 0
}

// contextSupportsSubscribe reports whether the current focus has an identifiable channel to subscribe to.
func (m Model) contextSupportsSubscribe() bool {
	chID, _ := m.currentChannelInfo()
	return chID != ""
}

// contextSupportsSorting reports whether the current context has any valid sort actions.
func (m Model) contextSupportsSorting() bool {
	ctx := m.currentContext()
	for _, ctxs := range sortContextSupport {
		for _, c := range ctxs {
			if c == ctx {
				return true
			}
		}
	}
	return false
}

// ── Generic chord registry ────────────────────────────────────────────────────

// chordAction is one completion option within a chord.
type chordAction struct {
	key   string      // completion key sequence (may be multi-char, e.g. "ss")
	label string      // shown in the pending-chord hint
	ctx   []ContextID // empty = valid in all contexts; non-empty = only these
	exec  func(Model) (Model, tea.Cmd)
}

// chordDef groups a trigger key with its set of completions.
type chordDef struct {
	trigger string // key that starts the chord (e.g. "t", "s")
	name    string // shown in the status bar hint (e.g. "tab", "sort")
	actions []chordAction
}

// validActions filters chord actions to those valid in ctx.
func validActions(actions []chordAction, ctx ContextID) []chordAction {
	var out []chordAction
	for _, a := range actions {
		if len(a.ctx) == 0 {
			out = append(out, a)
			continue
		}
		for _, c := range a.ctx {
			if c == ctx {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

// chordDefs returns the chord registry, using the cached value built in NewModel.
func (m Model) chordDefs() []chordDef {
	if m.chordCache != nil {
		return *m.chordCache
	}
	return m.buildChordDefs()
}

// buildChordDefs constructs the full chord registry from the current config and tabs.
func (m Model) buildChordDefs() []chordDef {
	kb := m.cfg.Keybindings
	tk := kb.TabKeys
	sk := kb.SortKeys

	// ── Tab chord ─────────────────────────────────────────────────────────
	visible := map[int]bool{}
	for _, id := range m.tabs {
		visible[id] = true
	}
	type tabEntry struct {
		key, label string
		id         int
	}
	allTabs := []tabEntry{
		{tk.Recommended, "recommended", tabRecommended},
		{tk.Subscriptions, "subscriptions", tabSubscriptions},
		{tk.Channels, "channels", tabChannels},
		{tk.Playlists, "playlists", tabPlaylists},
		{tk.Search, "search", tabSearch},
		{tk.Downloading, "downloading", tabDownloading},
		{tk.Local, "local", tabLocal},
		{tk.History, "history", tabHistory},
	}
	var tabActions []chordAction
	for _, e := range allTabs {
		if !visible[e.id] {
			continue
		}
		id := e.id
		tabActions = append(tabActions, chordAction{
			key:   e.key,
			label: e.label,
			exec: func(m Model) (Model, tea.Cmd) {
				m.activeTab = id
				cmd := m.onTabActivated()
				return m, cmd
			},
		})
	}

	// ── Sort chord ────────────────────────────────────────────────────────
	type sortEntry struct {
		key, label, action string
		vidSort            int
	}
	allSorts := []sortEntry{
		{sk.Date, "date", "date", vidSortDate},
		{sk.Views, "views", "views", vidSortViews},
		{sk.Name, "name", "name", vidSortName},
		{sk.Channel, "channel", "channel", vidSortChannel},
		{sk.Duration, "duration", "duration", vidSortDuration},
		{sk.Subscribers, "subscribers", "subscribers", -1},
		{sk.Tags, "tags", "tags", -1},
	}
	var sortActions []chordAction
	for _, e := range allSorts {
		action := e.action
		vidSort := e.vidSort
		sortActions = append(sortActions, chordAction{
			key:   e.key,
			label: e.label,
			ctx:   sortContextSupport[action],
			exec: func(m Model) (Model, tea.Cmd) {
				return m.applySortAction(action, vidSort, m.currentContext())
			},
		})
	}

	// ── Subscribe chord ───────────────────────────────────────────────────────
	subCtx := []ContextID{CtxVideoList, CtxSearchVideo, CtxSearchChannel, CtxHistoryVideo, CtxChannelList}
	subsk := kb.SubscribeKeys
	subscribeActions := []chordAction{
		{
			key:   subsk.Remote,
			label: "remote",
			ctx:   subCtx,
			exec: func(m Model) (Model, tea.Cmd) {
				if m.ytClient == nil {
					m.setStatus("subscribe: configure 'browser' in config to enable", true)
					return m, nil
				}
				chID, chName := m.currentChannelInfo()
				if chID == "" {
					m.setStatus("subscribe: no channel", true)
					return m, nil
				}
				return m, youtube.SubscribeToChannel(m.ytClient, chID, chName)
			},
		},
		{
			key:   subsk.Local,
			label: "local",
			ctx:   subCtx,
			exec: func(m Model) (Model, tea.Cmd) {
				chID, chName := m.currentChannelInfo()
				if chID == "" {
					m.setStatus("local subscribe: no channel", true)
					return m, nil
				}
				ch := domain.Channel{
					ID:      chID,
					Name:    chName,
					URL:     "https://www.youtube.com/channel/" + chID,
					IsLocal: true,
				}
				if err := m.db.AddSubscribedChannel(ch); err != nil {
					m.setStatus("local subscribe: "+err.Error(), true)
					return m, nil
				}
				m.subs.Subscribe(ch)
				m.setStatus("Locally subscribed: "+chName, false)
				_ = m.db.LogActivity(domain.ActivityEntry{
					Type: "subscribe", IsLocal: true,
					ChannelID: chID, ChannelName: chName,
				})
				return m, nil
			},
		},
	}

	// ── New-playlist chord ────────────────────────────────────────────────────
	plsk := kb.PlaylistKeys
	plCtx := []ContextID{CtxPlaylistList}
	newPlaylistActions := []chordAction{
		{
			key:   plsk.Remote,
			label: "YouTube",
			ctx:   plCtx,
			exec: func(m Model) (Model, tea.Cmd) {
				if m.ytClient == nil {
					m.setStatus("new YouTube playlist: configure 'browser' in config to enable", true)
					return m, nil
				}
				m.createModeYT = true
				m.createInput.SetValue("")
				m.createInput.Placeholder = "New YouTube playlist…"
				m.createInput.Focus()
				m.mode = modeCreatePlaylist
				return m, textinput.Blink
			},
		},
		{
			key:   plsk.Local,
			label: "local",
			ctx:   plCtx,
			exec: func(m Model) (Model, tea.Cmd) {
				m.createModeYT = false
				m.createInput.SetValue("")
				m.createInput.Placeholder = "New local playlist…"
				m.createInput.Focus()
				m.mode = modeCreatePlaylist
				return m, textinput.Blink
			},
		},
	}

	return []chordDef{
		{trigger: kb.TabChord, name: "tab", actions: tabActions},
		{trigger: kb.SortChord, name: "sort", actions: sortActions},
		{trigger: kb.Subscribe, name: "subscribe", actions: subscribeActions},
		{trigger: kb.NewPlaylist, name: "new playlist", actions: newPlaylistActions},
	}
}

// vidSortLabel returns a short display label for the current sort mode.
func vidSortLabel(mode int) string {
	switch mode {
	case vidSortViews:
		return "by views"
	case vidSortDate:
		return "by date"
	case vidSortName:
		return "by name"
	case vidSortChannel:
		return "by channel"
	case vidSortDuration:
		return "by duration"
	}
	return "default"
}

// videoShowChannel returns false when the Channel column is redundant
// (drilling into a specific channel's videos).
func (m Model) videoShowChannel() bool {
	if m.activeTab == tabChannels && !m.channels.tagsMode && m.channels.pane == 1 {
		return false
	}
	if m.activeTab == tabSearch && m.searchChSel != nil {
		return false
	}
	return true
}
