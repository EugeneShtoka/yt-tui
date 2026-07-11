package ui

import (
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
	tea "github.com/charmbracelet/bubbletea"
)

// viewCtx carries the shared, router-owned state a tab view needs to render and
// to make decisions. It is rebuilt per Update/View from the live *Model, so it
// always reflects the current state (the Model is copied by value through Bubble
// Tea). A view owns only its private cursor/scroll/sort; everything referenced
// here is shared across tabs and therefore stays on the router (see
// docs/TABVIEW_DESIGN.md, Finding 2).
type viewCtx struct {
	keys     keyMap
	width    int
	pageSize int
	circular bool
	db       Store

	// Shared feed/library data (written across tab boundaries).
	dlItems     []downloader.Item
	playAfter   map[string]bool
	localVideos []db.LocalVideo
	localTitleW int
	recVideos   []youtube.Video
	subVideos   []youtube.Video

	// Search result data + drill-down selection (written by async fetches).
	searchChSel    *youtube.Channel
	searchChannels []youtube.Channel
	searchVideos   []youtube.Video
	searchChVideos []youtube.Video

	// Video-list render inputs (Recommended/Subscriptions share renderVideoList).
	recLoading        bool
	recRefreshing     bool
	subLoading        bool
	localFilter       string
	localFilterCursor int
	// renderList is the router's shared video-list renderer (m.renderVideoList),
	// captured per frame so views can draw without a Model handle.
	renderList func(videos []youtube.Video, cursor, vs int, loading, refreshing bool, height int, title string) string
	// renderSearch is the router's bespoke Search renderer (m.renderSearch),
	// captured per frame; the searchView passes its own cursor/scroll into it.
	renderSearch func(height, cursor, vs, vidCursor, vidVS int) string
}

// viewIntent is an action a view decided on but cannot perform itself because it
// touches shared Model state (playback, deletion, cross-tab navigation). apply
// runs those router-side effects against the live *Model and returns any command.
// This keeps the "view decides, router acts" seam while giving every tab a
// uniform update signature.
type viewIntent interface {
	apply(m *Model) tea.Cmd
}

// tabView is the common surface the router dispatches to for a migrated tab.
// Extracted after four tabs (Activity, History, Downloading, Local) were grouped
// and their shared method set became observable.
type tabView interface {
	// update handles a key. It mutates the view's own cursor/scroll directly and
	// returns a viewIntent (or nil) for anything the router must perform.
	update(msg tea.KeyMsg, ctx viewCtx) viewIntent
	// render draws the tab body.
	render(ctx viewCtx, height int) string
	// context reports the sort/chord context at the cursor. It receives ctx so
	// multi-pane views (Search) can resolve their sub-pane context from the
	// shared, router-owned result data.
	context(ctx viewCtx) ContextID
}

// viewCtx builds the shared context handed to the active view this frame.
func (m *Model) viewCtx() viewCtx {
	return viewCtx{
		keys:        m.keys,
		width:       m.width,
		pageSize:    m.pageSize(),
		circular:    m.cfg.CircularNav,
		db:          m.db,
		dlItems:     m.downloader.Items(),
		playAfter:   m.playAfterDownload,
		localVideos: m.localVideos,
		localTitleW: m.videoListTitleW(),
		recVideos:   m.recVideos,
		subVideos:   m.subVideos,

		searchChSel:    m.searchChSel,
		searchChannels: m.searchChannels,
		searchVideos:   m.searchVideos,
		searchChVideos: m.searchChVideos,

		recLoading:        m.recLoading,
		recRefreshing:     m.recRefreshing,
		subLoading:        m.subChLoading && len(m.subVideos) == 0,
		localFilter:       m.localFilter,
		localFilterCursor: m.localFilterCursor,
		renderList:        m.renderVideoList,
		renderSearch:      m.renderSearch,
	}
}

// activeView returns the tabView for the current tab, or nil if the active tab
// has not been migrated to the tabView interface yet (the multi-pane tabs still
// use the legacy switch path during the P4 migration).
func (m *Model) activeView() tabView {
	switch m.activeTab {
	case tabActivity:
		return &m.activity
	case tabHistory:
		return &m.history
	case tabDownloading:
		return &m.downloading
	case tabLocal:
		return &m.local
	case tabRecommended:
		return &m.recommended
	case tabSubscriptions:
		return &m.subscriptions
	case tabSearch:
		return &m.search
	}
	return nil
}
