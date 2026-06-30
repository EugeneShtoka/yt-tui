package ui

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

const (
	tabRecommended   = 0
	tabSubscriptions = 1
	tabPlaylists     = 2
	tabSearch        = 3
	tabDownloading   = 4
	tabLocal         = 5
	tabHistory       = 6
	numTabIDs        = 7
)

var tabNames = [numTabIDs]string{
	"Recommended", "Subscriptions", "Playlists",
	"Search", "Downloading", "Local", "History",
}

var tabIDByName = map[string]int{
	"recommended":   tabRecommended,
	"subscriptions": tabSubscriptions,
	"playlists":     tabPlaylists,
	"search":        tabSearch,
	"downloading":   tabDownloading,
	"local":         tabLocal,
	"history":       tabHistory,
}

// subMode controls how the Subscriptions tab is displayed.
type subMode int

const (
	subModeAll      subMode = 0
	subModeChannels subMode = 1
)

// Model is the root bubbletea model.
type Model struct {
	cfg        *config.Config
	db         *db.DB
	downloader *downloader.Downloader

	width  int
	height int

	// tabs holds the ordered list of visible tab IDs, derived from config.
	tabs      []int
	activeTab int // one of the tabXxx constants above

	// ── Recommended ─────────────────────────────────────────────────────────
	recVideos     []youtube.Video
	recCursor     int
	recLoading    bool
	recLoaded     bool
	recRefreshing bool // true when background refresh is running over stale cache

	// ── Subscriptions ────────────────────────────────────────────────────────
	subMode subMode
	// subModeAll
	subVideos     []youtube.Video
	subCursor     int
	subLoading    bool
	subLoaded     bool
	subRefreshing bool
	// subModeChannels
	subChannels     []youtube.Channel
	subChCursor     int
	subChLoading    bool
	subChLoaded     bool
	subChVideos     []youtube.Video
	subChVidCursor  int
	subChVidLoading bool
	subChPane       int
	subChActiveID   string

	// ── Playlists ────────────────────────────────────────────────────────────
	playlists         []db.Playlist
	playlistCursor    int
	playlistVidCache  map[int64][]youtube.Video
	playlistVidCursor int
	playlistPane      int
	createMode        bool
	createInput       textinput.Model
	addOverlay        bool
	addOverlaySel     int
	addVideo          youtube.Video

	// ── Search ────────────────────────────────────────────────────────────────
	searchInput   textinput.Model
	searchFocused bool
	searchVideos  []youtube.Video
	searchCursor  int
	searchLoading bool
	lastQuery     string

	// ── Downloading ───────────────────────────────────────────────────────────
	dlCursor int

	// ── Local ────────────────────────────────────────────────────────────────
	localVideos []db.LocalVideo
	localCursor int

	// ── History ──────────────────────────────────────────────────────────────
	histEntries []db.HistoryEntry
	histCursor  int
	histLoaded  bool

	// ── Shared ───────────────────────────────────────────────────────────────
	spinner   spinner.Model
	status    string
	statusErr bool
	statusAt  time.Time
	showHelp  bool
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
		tabs = []int{tabRecommended, tabSubscriptions, tabPlaylists,
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

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	localVideos, _ := database.LocalVideos()
	playlists, _ := database.Playlists()

	// Load feed caches synchronously — fast DB reads, shown immediately.
	recCache, _ := database.GetFeedCache("recommended")
	subCache, _ := database.GetFeedCache("subscriptions")

	tabs := buildTabs(cfg)
	firstTab := tabRecommended
	if len(tabs) > 0 {
		firstTab = tabs[0]
	}

	return Model{
		cfg:              cfg,
		db:               database,
		downloader:       dl,
		tabs:             tabs,
		activeTab:        firstTab,
		recVideos:        recCache,
		recLoaded:        len(recCache) > 0,
		recLoading:       true,
		recRefreshing:    len(recCache) > 0,
		subVideos:        subCache,
		subLoaded:        len(subCache) > 0,
		subRefreshing:    len(subCache) > 0,
		searchInput:      si,
		createInput:      ci,
		spinner:          sp,
		localVideos:      localVideos,
		playlists:        playlists,
		playlistVidCache: make(map[int64][]youtube.Video),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		youtube.FetchRecommended(m.cfg),
		m.downloader.WaitForEvent(),
		m.spinner.Tick,
	)
}

func (m *Model) setStatus(msg string, isErr bool) {
	m.status = msg
	m.statusErr = isErr
	m.statusAt = time.Now()
}

func (m *Model) currentTabIndex() int {
	for i, id := range m.tabs {
		if id == m.activeTab {
			return i
		}
	}
	return 0
}

func (m *Model) currentVideo() (youtube.Video, bool) {
	switch m.activeTab {
	case tabRecommended:
		if i := m.recCursor; i >= 0 && i < len(m.recVideos) {
			return m.recVideos[i], true
		}
	case tabSubscriptions:
		if m.subMode == subModeAll {
			if i := m.subCursor; i >= 0 && i < len(m.subVideos) {
				return m.subVideos[i], true
			}
		} else if m.subChPane == 1 {
			if i := m.subChVidCursor; i >= 0 && i < len(m.subChVideos) {
				return m.subChVideos[i], true
			}
		}
	case tabSearch:
		if i := m.searchCursor; i >= 0 && i < len(m.searchVideos) {
			return m.searchVideos[i], true
		}
	case tabPlaylists:
		if m.playlistPane == 1 && len(m.playlists) > 0 {
			pl := m.playlists[m.playlistCursor]
			if vids, ok := m.playlistVidCache[pl.ID]; ok {
				if i := m.playlistVidCursor; i >= 0 && i < len(vids) {
					return vids[i], true
				}
			}
		}
	}
	return youtube.Video{}, false
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

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return string(runes[:n-1]) + "…"
}
