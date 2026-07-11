package ui

import (
	"fmt"
	"image"
	"sort"
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
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

// Video list sort modes (used by recSort, subSort, searchSort, localSort).
const (
	vidSortViews    = 0 // view count desc (default for recommended)
	vidSortDate     = 1 // upload date desc
	vidSortName     = 2 // title alphabetical asc
	vidSortNone     = 3 // no re-sort — keep fetch/API order
	vidSortChannel  = 4 // channel name alphabetical asc
	vidSortDuration = 5 // duration desc (longest first)
)

// Model is the root bubbletea model.
type Model struct {
	cfg        *config.Config
	db         Store
	downloader *downloader.Downloader

	width  int
	height int

	// tabs holds the ordered list of visible tab IDs, derived from config.
	tabs      []int
	activeTab int // one of the tabXxx constants above

	// ── Recommended ─────────────────────────────────────────────────────────
	recVideos     []youtube.Video
	recCursor     int
	recVS         int // nvim-style viewStart: first visible row
	recLoading    bool
	recLoaded     bool
	recRefreshing bool // true when background refresh is running over stale cache

	// ── Subscriptions ────────────────────────────────────────────────────────
	// subVideos — all-channel feed (Subscriptions tab)
	subVideos []youtube.Video
	subCursor int
	subVS     int
	// ── Channels ─────────────────────────────────────────────────────────────
	subChannels        []youtube.Channel
	subChCursor        int
	subChVS            int
	subChLoading       bool
	subChLoaded        bool
	subChVideos        []youtube.Video
	subChVidCursor     int
	subChVidVS         int
	subChVidLoading    bool
	subChVidRefreshing bool // has cached data; background fetch running
	subChPane          int
	subChActiveID      string
	subChSort          int                      // subChSortDate or subChSortName
	subChLatest        map[string]youtube.Video // channelID → latest known video

	// ── Channels: alias/tag editing ───────────────────────────────────────────
	subChEditMode  int // 0=none, 1=editing alias, 2=editing tags
	subChEditInput textinput.Model

	// ── Channels: tags-grouped view ───────────────────────────────────────────
	subChTagsMode  bool   // true = grouped-by-tags view
	subChTagCursor int    // cursor in tag list (tags mode, pane 0)
	subChTagVS     int    // viewStart for tag list
	subChTagSel    string // selected tag name
	subChTagSort   int    // video sort mode for tag video list

	// ── Playlists ────────────────────────────────────────────────────────────
	playlists            []db.Playlist        // local playlists (fallback when no YT)
	ytPlaylists          []youtube.YTPlaylist // YouTube playlists (loaded from YT)
	ytPlLoading          bool
	ytPlLoaded           bool
	ytClient             *youtube.YTClient // nil until browser cookies extracted
	playlistCursor       int
	playlistVS           int
	playlistVidCache     map[string][]youtube.Video
	playlistVidCursor    int
	playlistVidVS        int
	playlistVidLoading   bool
	playlistPane         int
	createMode           bool
	createModeYT         bool // true = creating a YouTube playlist (not local)
	createTypeMode       bool // true = in type-selection dialog before creating
	createTypeSel        int  // 0 = local, 1 = YouTube
	createInput          textinput.Model
	addOverlay           bool
	addOverlaySel        int
	addVideo             youtube.Video
	addAfterCreate       bool
	addOverlayCreateMode bool
	addOverlayCreateYT   bool
	addOverlayInput      textinput.Model

	// ── Search ────────────────────────────────────────────────────────────────
	searchInput   textinput.Model
	searchFocused bool
	searchVideos  []youtube.Video
	searchCursor  int
	searchVS      int
	searchLoading bool
	lastQuery     string
	searchHistory []string // past queries, newest first
	searchHistIdx int      // -1 = not navigating; 0+ = index into searchHistory
	searchDraft   string   // saved current input text when history nav starts

	// ── Downloading ───────────────────────────────────────────────────────────
	dlCursor int
	dlVS     int

	// ── Local ────────────────────────────────────────────────────────────────
	localVideos []db.LocalVideo
	localCursor int
	localVS     int

	// ── History ──────────────────────────────────────────────────────────────
	histEntries       []db.HistoryEntry
	histCursor        int
	histVS            int
	histLoaded        bool
	histDetailVideoID string
	histDetail        []db.HistoryEntry

	// ── Activity ─────────────────────────────────────────────────────────────
	actEntries []db.ActivityEntry
	actCursor  int
	actVS      int

	// ── Local filter ─────────────────────────────────────────────────────────
	localFilter        string
	localFilterFocused bool
	localFilterInput   textinput.Model
	localFilterCursor  int

	// ── Command mode (:cmd) ───────────────────────────────────────────────────
	cmdMode         bool
	cmdInput        textinput.Model
	cmdCompletions  []string
	cmdCompIdx      int
	cmdLastTabValue string

	// ── Search: channel results + drill-down ─────────────────────────────────
	searchChannels    []youtube.Channel
	searchChSel       *youtube.Channel
	searchChVideos    []youtube.Video
	searchChLoading   bool
	searchChVidCursor int
	searchChVidVS     int

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

	// ── Sort state per tab ────────────────────────────────────────────────
	recSort      int // recommended: vidSortViews default
	subSort      int // subscriptions all videos: vidSortDate default
	subChVidSort int // subscriptions channel drill-down: vidSortDate default
	searchSort   int // search results: vidSortNone default
	localSort    int // local library: vidSortNone default
	playlistSort int // playlist video pane: vidSortNone default

	// ── Recommended: hide/blacklist state ────────────────────────────────
	localVideoIDs        map[string]db.LocalVideo // cached for fast per-row lookup
	streamedVideoIDs     map[string]bool          // video IDs with any play/stream history event
	videoPositions       map[string]int64         // last known position ms for any video
	recHidden            map[string]bool          // video IDs hidden from recommended
	recPage              int                      // number of fetches fired this session
	subscribedChannelIDs map[string]bool          // channel IDs from subscriptions

	// ── Downloading: play-after-download ─────────────────────────────────
	playAfterDownload map[string]bool

	// ── Playback resume ───────────────────────────────────────────────────
	playerBackend     player.Backend
	playingVideoID    string         // ID of the video currently playing (for position saves)
	playingSBSegments []db.SBSegment // SponsorBlock segments for the current local file (empty = no conversion)

	// ── Pending direct overlay (chapters/links opened without info panel) ──
	pendingDirectOverlay string // "links" or "chapters"; cleared after VideoDetailsMsg handled

	// ── Video detail overlay ──────────────────────────────────────────────
	vidDetailOverlay       bool
	vidDetailVideo         *youtube.VideoDetails
	vidDetailLoading       bool
	vidDetailDescVS        int           // description scroll start line
	vidDetailThumb         image.Image   // nil until loaded; stays nil if fetch fails
	vidDetailLinks         *[]db.Link    // nil = not yet parsed; &[]db.Link{} = parsed, none found
	vidDetailChapters      *[]db.Chapter // nil = not available; populated from yt-dlp metadata
	vidDetailDescLines     []string      // pre-wrapped description lines; nil until video is set
	vidDetailThumbB64      string        // pre-encoded PNG base64 for Kitty; empty until loaded
	vidDetailThumbRendered string        // pre-rendered half-block string for non-Kitty terminals
	vidDetailKittyOverlay  string        // full Kitty sequence; recomputed only on thumbnail load or resize

	// ── Link list overlay (opened from video detail) ───────────────────────
	linkOverlay     bool
	linkOverlaySel  int
	linkOverlayURLs []db.Link

	// ── Chapter list overlay (opened from video detail) ────────────────────
	chapterOverlay      bool
	chapterOverlaySel   int
	chapterOverlayItems []db.Chapter
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

func NewModel(cfg *config.Config, database *db.DB, dl *downloader.Downloader) Model {
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

	localIDMap := buildLocalIDMap(localVideos)
	recHidden, _ := database.HiddenRecVideoIDs()
	if recHidden == nil {
		recHidden = make(map[string]bool)
	}

	// Load full channel list from DB for immediate display.
	cachedChannels, _ := database.GetSubscribedChannels()

	// Derive subscribed channel IDs from cached channels to pre-filter recommended feed.
	subscribedIDs := make(map[string]bool)
	for _, ch := range cachedChannels {
		if ch.ID != "" {
			subscribedIDs[ch.ID] = true
		}
		if ch.Name != "" {
			subscribedIDs["name:"+strings.ToLower(ch.Name)] = true
		}
	}
	if len(subscribedIDs) > 0 {
		recCache = filterSubscribed(recCache, subscribedIDs)
	}

	// Load subscriptions all-video list from channel_videos aggregate.
	channelIDs := make([]string, 0, len(cachedChannels))
	for _, ch := range cachedChannels {
		if ch.ID != "" {
			channelIDs = append(channelIDs, ch.ID)
		}
	}
	subVideos, _ := database.GetAllChannelVideos(channelIDs)
	sortVideos(subVideos, vidSortDate)

	// Load YouTube playlists from DB for immediate display.
	cachedYTPlaylists, _ := database.GetYTPlaylists()

	// Load latest-video-per-channel from channel_videos for immediate sort/display.
	chLatest, _ := database.GetChannelLatestAll()
	if chLatest == nil {
		chLatest = make(map[string]youtube.Video)
	}

	backend, _ := player.New(cfg)

	m := Model{
		cfg:                  cfg,
		db:                   database,
		downloader:           dl,
		tabs:                 tabs,
		activeTab:            firstTab,
		recVideos:            recCache,
		recLoaded:            len(recCache) > 0,
		recLoading:           true,
		recRefreshing:        len(recCache) > 0,
		subVideos:            subVideos,
		searchInput:          si,
		createInput:          ci,
		subChEditInput:       ei,
		addOverlayInput:      oi,
		subChTagSort:         vidSortDate,
		spinner:              sp,
		localVideos:          localVideos,
		localVideoIDs:        localIDMap,
		streamedVideoIDs:     mustWatchedIDs(database),
		videoPositions:       mustVideoPositions(database),
		recHidden:            recHidden,
		subscribedChannelIDs: subscribedIDs,
		subChannels:          cachedChannels,
		subChLoaded:          len(cachedChannels) > 0,
		subChLatest:          chLatest,
		localFilterInput:     textinput.New(),
		cmdInput:             func() textinput.Model { t := textinput.New(); t.Prompt = ""; return t }(),
		playAfterDownload:    make(map[string]bool),
		playlists:            playlists,
		ytPlaylists:          cachedYTPlaylists,
		ytPlLoaded:           len(cachedYTPlaylists) > 0,
		playlistVidCache:     make(map[string][]youtube.Video),
		keys:                 buildKeyMap(cfg.Keybindings),
		playerBackend:        backend,
		recSort:              vidSortViews,
		subSort:              vidSortDate,
		subChVidSort:         vidSortDate,
		searchSort:           vidSortNone,
		localSort:            vidSortNone,
		playlistSort:         vidSortNone,
		searchHistIdx:        -1,
	}
	chords := m.buildChordDefs()
	m.chordCache = &chords
	return m
}

// sortChannelSlice returns a sorted copy of the given channel slice.
func (m Model) sortChannelSlice(channels []youtube.Channel) []youtube.Channel {
	out := make([]youtube.Channel, len(channels))
	copy(out, channels)
	switch m.subChSort {
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
func (m Model) sortedChannels() []youtube.Channel {
	return m.sortChannelSlice(m.subChannels)
}

// channelsInTag returns channels belonging to the given tag (supports pseudo-tags).
func (m Model) channelsInTag(tag string) []youtube.Channel {
	switch tag {
	case pseudoTagAll:
		return m.subChannels
	case pseudoTagUntagged:
		var out []youtube.Channel
		for _, ch := range m.subChannels {
			if len(ch.Tags) == 0 {
				out = append(out, ch)
			}
		}
		return out
	default:
		var out []youtube.Channel
		for _, ch := range m.subChannels {
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
func (m Model) sortedChannelsInTag(tag string) []youtube.Channel {
	return m.sortChannelSlice(m.channelsInTag(tag))
}

// allTags returns all unique user-defined tags, sorted alphabetically.
func (m Model) allTags() []string {
	seen := map[string]bool{}
	for _, ch := range m.subChannels {
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

// tagVideos returns videos from subVideos that belong to channels in the selected tag,
// sorted by subChTagSort. The returned slice is always a fresh copy.
func (m Model) tagVideos() []youtube.Video {
	chans := m.channelsInTag(m.subChTagSel)
	if len(chans) == 0 {
		return nil
	}
	idSet := make(map[string]bool, len(chans))
	for _, ch := range chans {
		if ch.ID != "" {
			idSet[ch.ID] = true
		}
	}
	var out []youtube.Video
	for _, v := range m.subVideos {
		if idSet[v.ChannelID] {
			out = append(out, v)
		}
	}
	sortVideos(out, m.subChTagSort)
	return out
}

func buildLocalIDMap(lvs []db.LocalVideo) map[string]db.LocalVideo {
	m := make(map[string]db.LocalVideo, len(lvs))
	for _, lv := range lvs {
		m[lv.ID] = lv
	}
	return m
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
		youtube.FetchRecommended(m.cfg),
		youtube.FetchSubscribedChannelsBackground(m.cfg), // silently populate filter on startup
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
	if m.ytPlLoaded && m.playlistCursor < len(m.ytPlaylists) {
		return m.ytPlaylists[m.playlistCursor].ID
	}
	localIdx := m.playlistCursor
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
	if m.ytPlLoaded && m.playlistCursor < len(m.ytPlaylists) {
		return m.ytPlaylists[m.playlistCursor].Title
	}
	localIdx := m.playlistCursor
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
	return m.ytPlLoaded && m.ytClient != nil && m.playlistCursor < len(m.ytPlaylists)
}

func (m *Model) currentTabIndex() int {
	for i, id := range m.tabs {
		if id == m.activeTab {
			return i
		}
	}
	return 0
}

func filterText(videos []youtube.Video, q string) []youtube.Video {
	if q == "" {
		return videos
	}
	lower := strings.ToLower(q)
	out := make([]youtube.Video, 0, len(videos))
	for _, v := range videos {
		if strings.Contains(strings.ToLower(v.Title), lower) ||
			strings.Contains(strings.ToLower(v.Channel), lower) {
			out = append(out, v)
		}
	}
	return out
}

func (m *Model) localFilteredVideos() []youtube.Video {
	var raw []youtube.Video
	switch m.activeTab {
	case tabRecommended:
		raw = m.recVideos
	case tabSubscriptions:
		raw = m.subVideos
	case tabChannels:
		if m.subChTagsMode && m.subChPane == 1 {
			raw = m.tagVideos()
		} else if !m.subChTagsMode && m.subChPane == 1 {
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

func (m *Model) currentVideo() (youtube.Video, bool) {
	if m.localFilter != "" {
		filtered := m.localFilteredVideos()
		if i := m.localFilterCursor; i >= 0 && i < len(filtered) {
			return filtered[i], true
		}
		return youtube.Video{}, false
	}
	switch m.activeTab {
	case tabRecommended:
		if i := m.recCursor; i >= 0 && i < len(m.recVideos) {
			return m.recVideos[i], true
		}
	case tabSubscriptions:
		if i := m.subCursor; i >= 0 && i < len(m.subVideos) {
			return m.subVideos[i], true
		}
	case tabChannels:
		if m.subChTagsMode && m.subChPane == 1 {
			vids := m.tagVideos()
			if i := m.subChCursor; i >= 0 && i < len(vids) {
				return vids[i], true
			}
		} else if !m.subChTagsMode && m.subChPane == 1 {
			if i := m.subChVidCursor; i >= 0 && i < len(m.subChVideos) {
				return m.subChVideos[i], true
			}
		}
	case tabSearch:
		if m.searchChSel != nil {
			if i := m.searchChVidCursor; i >= 0 && i < len(m.searchChVideos) {
				return m.searchChVideos[i], true
			}
		} else {
			nCh := len(m.searchChannels)
			idx := m.searchCursor - nCh
			if idx >= 0 && idx < len(m.searchVideos) {
				return m.searchVideos[idx], true
			}
		}
	case tabPlaylists:
		if m.playlistPane == 1 {
			if vids, ok := m.playlistVidCache[m.selectedPlaylistKey()]; ok {
				if i := m.playlistVidCursor; i >= 0 && i < len(vids) {
					return vids[i], true
				}
			}
		}
	case tabDownloading:
		items := m.downloader.Items()
		if i := m.dlCursor; i >= 0 && i < len(items) {
			return items[i].Video, true
		}
	case tabLocal:
		if i := m.localCursor; i >= 0 && i < len(m.localVideos) {
			lv := m.localVideos[i]
			return youtube.Video{
				ID:    lv.ID,
				Title: lv.Title,
				URL:   "https://www.youtube.com/watch?v=" + lv.ID,
			}, true
		}
	}
	return youtube.Video{}, false
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

func (m *Model) jumpToLine(idx int) {
	ps := m.pageSize()
	switch m.activeTab {
	case tabRecommended:
		m.recCursor, m.recVS = vsJump(idx, len(m.recVideos), ps)
	case tabSubscriptions:
		m.subCursor, m.subVS = vsJump(idx, len(m.subVideos), ps)
	case tabChannels:
		if m.subChTagsMode {
			if m.subChPane == 1 {
				m.subChCursor, m.subChVS = vsJump(idx, len(m.tagVideos()), ps)
			} else {
				m.subChTagCursor, m.subChTagVS = vsJump(idx, len(m.tagListItems()), ps)
			}
		} else if m.subChPane == 0 {
			m.subChCursor, m.subChVS = vsJump(idx, len(m.sortedChannels()), ps)
		} else {
			m.subChVidCursor, m.subChVidVS = vsJump(idx, len(m.subChVideos), ps)
		}
	case tabPlaylists:
		if m.playlistPane == 0 {
			m.playlistCursor, m.playlistVS = vsJump(idx, m.playlistCount(), ps)
		} else {
			vids := m.playlistVidCache[m.selectedPlaylistKey()]
			m.playlistVidCursor, m.playlistVidVS = vsJump(idx, len(vids), ps)
		}
	case tabSearch:
		nCh := len(m.searchChannels)
		m.searchCursor = clamp(nCh+idx, nCh+len(m.searchVideos))
		m.updateSearchVS(nCh, len(m.searchVideos))
	case tabDownloading:
		m.dlCursor, m.dlVS = vsJump(idx, len(m.downloader.Items()), ps)
	case tabLocal:
		m.localCursor, m.localVS = vsJump(idx, len(m.localVideos), ps)
	case tabHistory:
		m.histCursor, m.histVS = vsJump(idx, len(m.histEntries), ps)
	}
}

func (m *Model) jumpToLast() {
	ps := m.pageSize()
	switch m.activeTab {
	case tabRecommended:
		m.recCursor, m.recVS = vsJump(len(m.recVideos)-1, len(m.recVideos), ps)
	case tabSubscriptions:
		m.subCursor, m.subVS = vsJump(len(m.subVideos)-1, len(m.subVideos), ps)
	case tabChannels:
		if m.subChTagsMode {
			if m.subChPane == 1 {
				vids := m.tagVideos()
				m.subChCursor, m.subChVS = vsJump(len(vids)-1, len(vids), ps)
			} else {
				n := len(m.tagListItems())
				m.subChTagCursor, m.subChTagVS = vsJump(n-1, n, ps)
			}
		} else if m.subChPane == 0 {
			sc := m.sortedChannels()
			m.subChCursor, m.subChVS = vsJump(len(sc)-1, len(sc), ps)
		} else {
			m.subChVidCursor, m.subChVidVS = vsJump(len(m.subChVideos)-1, len(m.subChVideos), ps)
		}
	case tabPlaylists:
		if m.playlistPane == 0 {
			m.playlistCursor, m.playlistVS = vsJump(m.playlistCount()-1, m.playlistCount(), ps)
		} else {
			vids := m.playlistVidCache[m.selectedPlaylistKey()]
			m.playlistVidCursor, m.playlistVidVS = vsJump(len(vids)-1, len(vids), ps)
		}
	case tabSearch:
		nCh := len(m.searchChannels)
		nVid := len(m.searchVideos)
		m.searchCursor = nCh + clamp(nVid-1, nVid)
		m.updateSearchVS(nCh, nVid)
	case tabDownloading:
		items := m.downloader.Items()
		m.dlCursor, m.dlVS = vsJump(len(items)-1, len(items), ps)
	case tabLocal:
		m.localCursor, m.localVS = vsJump(len(m.localVideos)-1, len(m.localVideos), ps)
	case tabHistory:
		m.histCursor, m.histVS = vsJump(len(m.histEntries)-1, len(m.histEntries), ps)
	}
}

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

type vidSortKey struct {
	viewCount  int64
	uploadDate string
	title      string
	channel    string
	duration   int
}

func sortByMode[T any](s []T, mode int, extract func(T) vidSortKey) {
	switch mode {
	case vidSortViews:
		sort.SliceStable(s, func(i, j int) bool { return extract(s[i]).viewCount > extract(s[j]).viewCount })
	case vidSortDate:
		sort.SliceStable(s, func(i, j int) bool { return extract(s[i]).uploadDate > extract(s[j]).uploadDate })
	case vidSortName:
		sort.SliceStable(s, func(i, j int) bool {
			return strings.ToLower(extract(s[i]).title) < strings.ToLower(extract(s[j]).title)
		})
	case vidSortChannel:
		sort.SliceStable(s, func(i, j int) bool {
			return strings.ToLower(extract(s[i]).channel) < strings.ToLower(extract(s[j]).channel)
		})
	case vidSortDuration:
		sort.SliceStable(s, func(i, j int) bool { return extract(s[i]).duration > extract(s[j]).duration })
	// vidSortNone: no-op — keep current order
	}
}

func sortVideos(videos []youtube.Video, mode int) {
	sortByMode(videos, mode, func(v youtube.Video) vidSortKey {
		return vidSortKey{v.ViewCount, v.UploadDate, v.Title, v.Channel, v.Duration}
	})
}

func sortLocalVideos(videos []db.LocalVideo, mode int) {
	sortByMode(videos, mode, func(v db.LocalVideo) vidSortKey {
		return vidSortKey{v.ViewCount, v.UploadDate, v.Title, v.Channel, v.Duration}
	})
}

// currentContext returns the ContextID for the currently focused UI area.
func (m Model) currentContext() ContextID {
	switch m.activeTab {
	case tabRecommended:
		return CtxVideoList
	case tabSubscriptions:
		return CtxVideoList
	case tabChannels:
		if m.subChTagsMode {
			if m.subChPane == 1 {
				return CtxVideoList
			}
			return CtxTagList
		}
		if m.subChPane == 0 {
			return CtxChannelList
		}
		return CtxVideoList
	case tabSearch:
		if m.searchChSel != nil {
			return CtxVideoList // channel drill-down shows a video list
		}
		if m.searchCursor < len(m.searchChannels) {
			return CtxSearchChannel
		}
		return CtxSearchVideo
	case tabPlaylists:
		if m.playlistPane == 0 {
			return CtxPlaylistList
		}
		return CtxVideoList
	case tabDownloading:
		return CtxDownloading
	case tabLocal:
		return CtxLocal
	case tabHistory:
		if m.histDetailVideoID != "" {
			return CtxHistoryVideo
		}
		if m.histCursor < len(m.histEntries) && m.histEntries[m.histCursor].EventType == "search" {
			return CtxHistorySearch
		}
		return CtxHistoryVideo
	}
	return CtxVideoList
}

// contextSupportsNewPlaylist reports whether a new-playlist chord should activate.
func (m Model) contextSupportsNewPlaylist() bool {
	return m.activeTab == tabPlaylists && m.playlistPane == 0
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
				ch := youtube.Channel{
					ID:      chID,
					Name:    chName,
					URL:     "https://www.youtube.com/channel/" + chID,
					IsLocal: true,
				}
				if err := m.db.AddSubscribedChannel(ch); err != nil {
					m.setStatus("local subscribe: "+err.Error(), true)
					return m, nil
				}
				found := false
				for _, c := range m.subChannels {
					if c.ID == chID {
						found = true
						break
					}
				}
				if !found {
					m.subChannels = append(m.subChannels, ch)
					m.subscribedChannelIDs[chID] = true
					if chName != "" {
						m.subscribedChannelIDs["name:"+strings.ToLower(chName)] = true
					}
				}
				m.setStatus("Locally subscribed: "+chName, false)
				_ = m.db.LogActivity(db.ActivityEntry{
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
				m.createMode = true
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
				m.createMode = true
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
	if m.activeTab == tabChannels && !m.subChTagsMode && m.subChPane == 1 {
		return false
	}
	if m.activeTab == tabSearch && m.searchChSel != nil {
		return false
	}
	return true
}
