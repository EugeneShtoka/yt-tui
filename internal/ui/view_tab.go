package ui

import (
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
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
	// context reports the sort/chord context at the cursor.
	context() ContextID
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
	}
	return nil
}
