package ui

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/player"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case youtube.FetchResultMsg:
		return m.handleFetchResult(msg)

	case youtube.ChannelListMsg:
		m.subChLoading = false
		m.subChLoaded = true
		if msg.Err != nil {
			m.setStatus("channels: "+msg.Err.Error(), true)
		} else {
			m.subChannels = msg.Channels
		}
		return m, nil

	case youtube.ChannelVideosMsg:
		m.subChVidLoading = false
		if msg.Err != nil {
			m.setStatus("channel videos: "+msg.Err.Error(), true)
		} else {
			m.subChVideos = msg.Videos
			m.subChVidCursor = 0
			m.subChPane = 1
		}
		return m, nil

	case youtube.SearchResultMsg:
		m.searchLoading = false
		if msg.Err != nil {
			m.setStatus("search: "+msg.Err.Error(), true)
		} else {
			m.searchVideos = msg.Videos
			m.searchCursor = 0
		}
		return m, nil

	case downloader.EventMsg:
		m.handleDownloadEvent(downloader.Event(msg))
		return m, m.downloader.WaitForEvent()

	case cursor.BlinkMsg:
		if m.searchFocused {
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			return m, cmd
		}
		if m.createMode {
			var cmd tea.Cmd
			m.createInput, cmd = m.createInput.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleFetchResult(msg youtube.FetchResultMsg) (Model, tea.Cmd) {
	if msg.Err != nil {
		m.setStatus(msg.Source+": "+msg.Err.Error(), true)
	}
	switch msg.Source {
	case "recommended":
		m.recLoading = false
		m.recRefreshing = false
		m.recLoaded = true
		if msg.Err == nil {
			videos := filterByAge(msg.Videos, m.cfg.RecommendedMaxAgeDays)
			sortByViews(videos)
			m.recCursor = preserveCursor(m.recVideos, m.recCursor, videos)
			m.recVideos = videos
			go m.db.SaveFeedCache("recommended", videos)
		}
	case "subscriptions":
		m.subLoading = false
		m.subRefreshing = false
		m.subLoaded = true
		if msg.Err == nil {
			m.subCursor = preserveCursor(m.subVideos, m.subCursor, msg.Videos)
			m.subVideos = msg.Videos
			go m.db.SaveFeedCache("subscriptions", msg.Videos)
		}
	}
	return m, nil
}

func (m *Model) handleDownloadEvent(ev downloader.Event) {
	switch ev.Kind {
	case downloader.EventComplete:
		m.setStatus(fmt.Sprintf("Downloaded: %s", ev.FilePath), false)
		if lv, err := m.db.LocalVideos(); err == nil {
			m.localVideos = lv
		}
	case downloader.EventError:
		m.setStatus("Download failed: "+ev.Err.Error(), true)
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.searchFocused {
		return m.handleSearchInput(msg)
	}
	if m.createMode {
		return m.handleCreateInput(msg)
	}
	if m.addOverlay {
		return m.handleAddOverlay(msg)
	}
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	switch {
	// ── Tab switching ─────────────────────────────────────────────────────
	case key.Matches(msg, keys.Tab):
		idx := m.currentTabIndex()
		m.activeTab = m.tabs[(idx+1)%len(m.tabs)]
		return m, m.onTabActivated()
	case key.Matches(msg, keys.ShiftTab):
		idx := m.currentTabIndex()
		m.activeTab = m.tabs[(idx+len(m.tabs)-1)%len(m.tabs)]
		return m, m.onTabActivated()

	// F-keys are positional: F2 = first tab, F3 = second, etc.
	case key.Matches(msg, keys.F2):
		return m.switchToTabPos(0)
	case key.Matches(msg, keys.F3):
		return m.switchToTabPos(1)
	case key.Matches(msg, keys.F4):
		return m.switchToTabPos(2)
	case key.Matches(msg, keys.F5):
		return m.switchToTabPos(3)
	case key.Matches(msg, keys.F6):
		return m.switchToTabPos(4)
	case key.Matches(msg, keys.F7):
		return m.switchToTabPos(5)
	case key.Matches(msg, keys.F8):
		return m.switchToTabPos(6)

	// ── Global actions ────────────────────────────────────────────────────
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Help):
		m.showHelp = !m.showHelp
		return m, nil
	case key.Matches(msg, keys.Search) && m.activeTab != tabSearch:
		m.activeTab = tabSearch
		m.searchFocused = true
		m.searchInput.Focus()
		return m, textinput.Blink
	case key.Matches(msg, keys.Refresh):
		return m, m.refresh()
	}

	switch m.activeTab {
	case tabRecommended:
		return m.updateRecommended(msg)
	case tabSubscriptions:
		return m.updateSubscriptions(msg)
	case tabPlaylists:
		return m.updatePlaylists(msg)
	case tabSearch:
		return m.updateSearch(msg)
	case tabDownloading:
		return m.updateDownloading(msg)
	case tabLocal:
		return m.updateLocal(msg)
	case tabHistory:
		return m.updateHistory(msg)
	}

	return m, nil
}

func (m Model) switchToTabPos(pos int) (tea.Model, tea.Cmd) {
	if pos < len(m.tabs) {
		m.activeTab = m.tabs[pos]
		return m, m.onTabActivated()
	}
	return m, nil
}

// ── Tab activation ────────────────────────────────────────────────────────────

func (m *Model) onTabActivated() tea.Cmd {
	switch m.activeTab {
	case tabRecommended:
		if !m.recLoading {
			m.recLoading = true
			m.recRefreshing = m.recLoaded
			return youtube.FetchRecommended(m.cfg)
		}
	case tabSubscriptions:
		if m.subMode == subModeAll && !m.subLoading {
			m.subLoading = true
			m.subRefreshing = m.subLoaded
			return youtube.FetchSubscriptions(m.cfg)
		}
		if m.subMode == subModeChannels && !m.subChLoaded && !m.subChLoading {
			m.subChLoading = true
			return youtube.FetchSubscribedChannels(m.cfg)
		}
	case tabHistory:
		return m.loadHistory()
	}
	return nil
}

func (m *Model) loadHistory() tea.Cmd {
	entries, err := m.db.History(200)
	if err != nil {
		m.setStatus("history: "+err.Error(), true)
	} else {
		m.histEntries = entries
		m.histCursor = 0
		m.histLoaded = true
	}
	return nil
}

func (m *Model) refresh() tea.Cmd {
	switch m.activeTab {
	case tabRecommended:
		if !m.recLoading {
			m.recLoading = true
			m.recRefreshing = m.recLoaded
			return youtube.FetchRecommended(m.cfg)
		}
	case tabSubscriptions:
		if m.subMode == subModeAll && !m.subLoading {
			m.subLoading = true
			m.subRefreshing = m.subLoaded
			return youtube.FetchSubscriptions(m.cfg)
		}
		if m.subMode == subModeChannels {
			m.subChLoading = true
			m.subChLoaded = false
			return youtube.FetchSubscribedChannels(m.cfg)
		}
	case tabSearch:
		if m.lastQuery != "" {
			m.searchLoading = true
			return youtube.Search(m.cfg, m.lastQuery)
		}
	case tabLocal:
		if lv, err := m.db.LocalVideos(); err == nil {
			m.localVideos = lv
		}
	case tabHistory:
		return m.loadHistory()
	}
	return nil
}

// ── Video tabs: Recommended ───────────────────────────────────────────────────

func (m Model) updateRecommended(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.recVideos)
	switch {
	case key.Matches(msg, keys.Up):
		m.recCursor = clamp(m.recCursor-1, n)
	case key.Matches(msg, keys.Down):
		m.recCursor = clamp(m.recCursor+1, n)
	case key.Matches(msg, keys.PageUp):
		m.recCursor = clamp(m.recCursor-m.pageSize(), n)
	case key.Matches(msg, keys.PageDown):
		m.recCursor = clamp(m.recCursor+m.pageSize(), n)
	case key.Matches(msg, keys.Download):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeVideo)
		}
	case key.Matches(msg, keys.DownloadAudio):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeAudio)
		}
	case key.Matches(msg, keys.WatchLater):
		if v, ok := m.currentVideo(); ok {
			_ = m.db.AddWatchLater(v.ID, v.Title, v.Channel, v.URL)
			m.setStatus("Added to Watch Later: "+truncate(v.Title, 50), false)
		}
	case key.Matches(msg, keys.AddList):
		if v, ok := m.currentVideo(); ok {
			m.openAddOverlay(v)
		}
	}
	return m, nil
}

// ── Subscriptions ─────────────────────────────────────────────────────────────

func (m Model) updateSubscriptions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.ToggleMode):
		if m.subMode == subModeAll {
			m.subMode = subModeChannels
			if !m.subChLoaded && !m.subChLoading {
				m.subChLoading = true
				return m, youtube.FetchSubscribedChannels(m.cfg)
			}
		} else {
			m.subMode = subModeAll
			m.subChPane = 0
			if !m.subLoaded && !m.subLoading {
				m.subLoading = true
				return m, youtube.FetchSubscriptions(m.cfg)
			}
		}
		return m, nil
	}

	if m.subMode == subModeAll {
		return m.updateSubAll(msg)
	}
	return m.updateSubChannels(msg)
}

func (m Model) updateSubAll(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.subVideos)
	switch {
	case key.Matches(msg, keys.Up):
		m.subCursor = clamp(m.subCursor-1, n)
	case key.Matches(msg, keys.Down):
		m.subCursor = clamp(m.subCursor+1, n)
	case key.Matches(msg, keys.PageUp):
		m.subCursor = clamp(m.subCursor-m.pageSize(), n)
	case key.Matches(msg, keys.PageDown):
		m.subCursor = clamp(m.subCursor+m.pageSize(), n)
	case key.Matches(msg, keys.Download):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeVideo)
		}
	case key.Matches(msg, keys.DownloadAudio):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeAudio)
		}
	case key.Matches(msg, keys.WatchLater):
		if v, ok := m.currentVideo(); ok {
			_ = m.db.AddWatchLater(v.ID, v.Title, v.Channel, v.URL)
			m.setStatus("Added to Watch Later: "+truncate(v.Title, 50), false)
		}
	case key.Matches(msg, keys.AddList):
		if v, ok := m.currentVideo(); ok {
			m.openAddOverlay(v)
		}
	}
	return m, nil
}

func (m Model) updateSubChannels(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.subChPane == 0 {
		n := len(m.subChannels)
		switch {
		case key.Matches(msg, keys.Up):
			m.subChCursor = clamp(m.subChCursor-1, n)
		case key.Matches(msg, keys.Down):
			m.subChCursor = clamp(m.subChCursor+1, n)
		case key.Matches(msg, keys.Enter), key.Matches(msg, keys.Right):
			if m.subChCursor < n {
				ch := m.subChannels[m.subChCursor]
				m.subChActiveID = ch.ID
				m.subChVidLoading = true
				m.subChVidCursor = 0
				return m, youtube.FetchChannelVideos(m.cfg, ch.ID)
			}
		}
		return m, nil
	}

	n := len(m.subChVideos)
	switch {
	case key.Matches(msg, keys.Left), key.Matches(msg, keys.Escape):
		m.subChPane = 0
	case key.Matches(msg, keys.Up):
		m.subChVidCursor = clamp(m.subChVidCursor-1, n)
	case key.Matches(msg, keys.Down):
		m.subChVidCursor = clamp(m.subChVidCursor+1, n)
	case key.Matches(msg, keys.PageUp):
		m.subChVidCursor = clamp(m.subChVidCursor-m.pageSize(), n)
	case key.Matches(msg, keys.PageDown):
		m.subChVidCursor = clamp(m.subChVidCursor+m.pageSize(), n)
	case key.Matches(msg, keys.Download):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeVideo)
		}
	case key.Matches(msg, keys.DownloadAudio):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeAudio)
		}
	case key.Matches(msg, keys.WatchLater):
		if v, ok := m.currentVideo(); ok {
			_ = m.db.AddWatchLater(v.ID, v.Title, v.Channel, v.URL)
			m.setStatus("Added to Watch Later: "+truncate(v.Title, 50), false)
		}
	case key.Matches(msg, keys.AddList):
		if v, ok := m.currentVideo(); ok {
			m.openAddOverlay(v)
		}
	}
	return m, nil
}

// ── Playlists ─────────────────────────────────────────────────────────────────

func (m Model) updatePlaylists(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.playlistPane == 0 {
		n := len(m.playlists)
		switch {
		case key.Matches(msg, keys.Up):
			m.playlistCursor = clamp(m.playlistCursor-1, n)
		case key.Matches(msg, keys.Down):
			m.playlistCursor = clamp(m.playlistCursor+1, n)
		case key.Matches(msg, keys.Enter), key.Matches(msg, keys.Right):
			if m.playlistCursor < n {
				m.playlistPane = 1
				m.playlistVidCursor = 0
			}
		case key.Matches(msg, keys.NewList):
			m.createMode = true
			m.createInput.SetValue("")
			m.createInput.Focus()
			return m, textinput.Blink
		case key.Matches(msg, keys.Delete):
			if m.playlistCursor < n {
				pl := m.playlists[m.playlistCursor]
				_ = m.db.DeletePlaylist(pl.ID)
				playlists, _ := m.db.Playlists()
				m.playlists = playlists
				m.playlistCursor = clamp(m.playlistCursor, len(m.playlists))
			}
		}
		return m, nil
	}

	if m.playlistCursor >= len(m.playlists) {
		m.playlistPane = 0
		return m, nil
	}
	pl := m.playlists[m.playlistCursor]
	vids := m.playlistVidCache[pl.ID]
	n := len(vids)

	switch {
	case key.Matches(msg, keys.Left), key.Matches(msg, keys.Escape):
		m.playlistPane = 0
	case key.Matches(msg, keys.Up):
		m.playlistVidCursor = clamp(m.playlistVidCursor-1, n)
	case key.Matches(msg, keys.Down):
		m.playlistVidCursor = clamp(m.playlistVidCursor+1, n)
	case key.Matches(msg, keys.Delete):
		if m.playlistVidCursor < n {
			vid := vids[m.playlistVidCursor]
			_ = m.db.RemoveFromPlaylist(pl.ID, vid.ID)
			delete(m.playlistVidCache, pl.ID)
			m.playlistVidCursor = clamp(m.playlistVidCursor, n-1)
		}
	case key.Matches(msg, keys.Download):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeVideo)
		}
	case key.Matches(msg, keys.DownloadAudio):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeAudio)
		}
	}
	return m, nil
}

// ── Search ────────────────────────────────────────────────────────────────────

func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Allow tab switching directly from search results without needing Escape first.
	switch {
	case key.Matches(msg, keys.Tab):
		idx := m.currentTabIndex()
		m.activeTab = m.tabs[(idx+1)%len(m.tabs)]
		return m, m.onTabActivated()
	case key.Matches(msg, keys.ShiftTab):
		idx := m.currentTabIndex()
		m.activeTab = m.tabs[(idx+len(m.tabs)-1)%len(m.tabs)]
		return m, m.onTabActivated()
	case key.Matches(msg, keys.F2):
		return m.switchToTabPos(0)
	case key.Matches(msg, keys.F3):
		return m.switchToTabPos(1)
	case key.Matches(msg, keys.F4):
		return m.switchToTabPos(2)
	case key.Matches(msg, keys.F5):
		return m.switchToTabPos(3)
	case key.Matches(msg, keys.F6):
		return m.switchToTabPos(4)
	case key.Matches(msg, keys.F7):
		return m.switchToTabPos(5)
	case key.Matches(msg, keys.F8):
		return m.switchToTabPos(6)
	}

	if key.Matches(msg, keys.Search) {
		m.searchFocused = true
		m.searchInput.Focus()
		return m, textinput.Blink
	}
	n := len(m.searchVideos)
	switch {
	case key.Matches(msg, keys.Up):
		m.searchCursor = clamp(m.searchCursor-1, n)
	case key.Matches(msg, keys.Down):
		m.searchCursor = clamp(m.searchCursor+1, n)
	case key.Matches(msg, keys.PageUp):
		m.searchCursor = clamp(m.searchCursor-m.pageSize(), n)
	case key.Matches(msg, keys.PageDown):
		m.searchCursor = clamp(m.searchCursor+m.pageSize(), n)
	case key.Matches(msg, keys.Download):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeVideo)
		}
	case key.Matches(msg, keys.DownloadAudio):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeAudio)
		}
	case key.Matches(msg, keys.WatchLater):
		if v, ok := m.currentVideo(); ok {
			_ = m.db.AddWatchLater(v.ID, v.Title, v.Channel, v.URL)
			m.setStatus("Added to Watch Later: "+truncate(v.Title, 50), false)
		}
	case key.Matches(msg, keys.AddList):
		if v, ok := m.currentVideo(); ok {
			m.openAddOverlay(v)
		}
	}
	return m, nil
}

func (m Model) handleSearchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		q := m.searchInput.Value()
		if q == "" {
			m.searchFocused = false
			m.searchInput.Blur()
			return m, nil
		}
		m.lastQuery = q
		m.searchLoading = true
		m.searchFocused = false
		m.searchInput.Blur()
		_ = m.db.AddHistory("", "search", q)
		return m, youtube.Search(m.cfg, q)
	case "esc":
		m.searchFocused = false
		m.searchInput.Blur()
		return m, nil
	case "tab":
		m.searchFocused = false
		m.searchInput.Blur()
		idx := m.currentTabIndex()
		m.activeTab = m.tabs[(idx+1)%len(m.tabs)]
		return m, m.onTabActivated()
	case "shift+tab":
		m.searchFocused = false
		m.searchInput.Blur()
		idx := m.currentTabIndex()
		m.activeTab = m.tabs[(idx+len(m.tabs)-1)%len(m.tabs)]
		return m, m.onTabActivated()
	default:
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}
}

// ── Downloading ───────────────────────────────────────────────────────────────

func (m Model) updateDownloading(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.downloader.Items()
	n := len(items)
	switch {
	case key.Matches(msg, keys.Up):
		m.dlCursor = clamp(m.dlCursor-1, n)
	case key.Matches(msg, keys.Down):
		m.dlCursor = clamp(m.dlCursor+1, n)
	}
	return m, nil
}

// ── Local ─────────────────────────────────────────────────────────────────────

func (m Model) updateLocal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.localVideos)
	switch {
	case key.Matches(msg, keys.Up):
		m.localCursor = clamp(m.localCursor-1, n)
	case key.Matches(msg, keys.Down):
		m.localCursor = clamp(m.localCursor+1, n)
	case key.Matches(msg, keys.PageUp):
		m.localCursor = clamp(m.localCursor-m.pageSize(), n)
	case key.Matches(msg, keys.PageDown):
		m.localCursor = clamp(m.localCursor+m.pageSize(), n)
	case key.Matches(msg, keys.Play):
		if m.localCursor < n {
			lv := m.localVideos[m.localCursor]
			if err := player.Play(lv.FilePath, m.cfg); err != nil {
				m.setStatus("play: "+err.Error(), true)
			} else {
				_ = m.db.SetVideoStatus(lv.ID, db.StatusStarted)
				_ = m.db.AddHistory(lv.ID, "play", "")
				if lv2, err := m.db.LocalVideos(); err == nil {
					m.localVideos = lv2
				}
				m.setStatus("Playing: "+truncate(lv.Title, 50), false)
			}
		}
	case key.Matches(msg, keys.Delete):
		if m.localCursor < n {
			lv := m.localVideos[m.localCursor]
			_ = os.Remove(lv.FilePath)
			_ = m.db.DeleteLocalVideo(lv.ID)
			_ = m.db.AddHistory(lv.ID, "delete", "")
			if lv2, err := m.db.LocalVideos(); err == nil {
				m.localVideos = lv2
			}
			m.localCursor = clamp(m.localCursor, len(m.localVideos))
			m.setStatus("Deleted: "+truncate(lv.Title, 50), false)
		}
	}
	return m, nil
}

// ── History ───────────────────────────────────────────────────────────────────

func (m Model) updateHistory(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.histEntries)
	switch {
	case key.Matches(msg, keys.Up):
		m.histCursor = clamp(m.histCursor-1, n)
	case key.Matches(msg, keys.Down):
		m.histCursor = clamp(m.histCursor+1, n)
	case key.Matches(msg, keys.PageUp):
		m.histCursor = clamp(m.histCursor-m.pageSize(), n)
	case key.Matches(msg, keys.PageDown):
		m.histCursor = clamp(m.histCursor+m.pageSize(), n)
	}
	return m, nil
}

// ── Playlist create input ─────────────────────────────────────────────────────

func (m Model) handleCreateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := m.createInput.Value()
		m.createMode = false
		m.createInput.Blur()
		if name != "" {
			if _, err := m.db.CreatePlaylist(name); err != nil {
				m.setStatus("create playlist: "+err.Error(), true)
			} else {
				playlists, _ := m.db.Playlists()
				m.playlists = playlists
				m.setStatus("Created playlist: "+name, false)
			}
		}
	case "esc":
		m.createMode = false
		m.createInput.Blur()
	default:
		var cmd tea.Cmd
		m.createInput, cmd = m.createInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// ── Add-to-playlist overlay ───────────────────────────────────────────────────

func (m *Model) openAddOverlay(v youtube.Video) {
	_ = m.db.UpsertVideo(v.ID, v.Title, v.Channel, v.ChannelID, v.Duration, v.ViewCount, v.UploadDate, v.URL)
	m.addVideo = v
	m.addOverlay = true
	m.addOverlaySel = 0
}

func (m Model) handleAddOverlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := 1 + len(m.playlists)
	switch msg.String() {
	case "esc", "q":
		m.addOverlay = false
	case "k", "up":
		m.addOverlaySel = clamp(m.addOverlaySel-1, n)
	case "j", "down":
		m.addOverlaySel = clamp(m.addOverlaySel+1, n)
	case "enter":
		v := m.addVideo
		if m.addOverlaySel == 0 {
			_ = m.db.AddWatchLater(v.ID, v.Title, v.Channel, v.URL)
			m.setStatus("Added to Watch Later: "+truncate(v.Title, 40), false)
		} else {
			pl := m.playlists[m.addOverlaySel-1]
			_ = m.db.AddToPlaylist(pl.ID, v.ID)
			m.setStatus(fmt.Sprintf("Added to '%s'", pl.Name), false)
		}
		m.addOverlay = false
	}
	return m, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (m *Model) startDownload(v youtube.Video, dlType downloader.DownloadType) {
	m.downloader.Start(v, dlType)
	label := "video"
	if dlType == downloader.TypeAudio {
		label = "audio"
	}
	m.setStatus(fmt.Sprintf("Queued %s: %s", label, truncate(v.Title, 50)), false)
	m.activeTab = tabDownloading
}

func (m *Model) pageSize() int {
	ps := m.height - 6
	if ps < 5 {
		return 5
	}
	return ps
}

// preserveCursor finds the previously selected video ID in the new list and returns
// the new cursor position so the selection follows the same video after a refresh.
func preserveCursor(old []youtube.Video, cursor int, new []youtube.Video) int {
	if cursor >= len(old) {
		return 0
	}
	prevID := old[cursor].ID
	for i, v := range new {
		if v.ID == prevID {
			return i
		}
	}
	return 0
}

// filterByAge removes videos whose upload date is older than maxDays.
// Videos with no date are kept.
func filterByAge(videos []youtube.Video, maxDays int) []youtube.Video {
	if maxDays <= 0 {
		return videos
	}
	cutoff := time.Now().AddDate(0, 0, -maxDays)
	out := videos[:0]
	for _, v := range videos {
		if len(v.UploadDate) != 8 {
			out = append(out, v)
			continue
		}
		t, err := time.Parse("20060102", v.UploadDate)
		if err != nil || !t.Before(cutoff) {
			out = append(out, v)
		}
	}
	return out
}

// sortByViews sorts videos descending by view count.
func sortByViews(videos []youtube.Video) {
	sort.Slice(videos, func(i, j int) bool {
		return videos[i].ViewCount > videos[j].ViewCount
	})
}
