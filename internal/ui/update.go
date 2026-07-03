package ui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case positionTickMsg:
		if pos, ok := m.playerBackend.Position(); ok && m.playingVideoID != "" {
			_ = m.db.UpdateLastPosition(m.playingVideoID, pos.Milliseconds())
		}
		return m, positionTick()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case ytClientInitMsg:
		if msg.err != nil {
			m.setStatus("YouTube sync unavailable: "+msg.err.Error(), true)
		} else {
			m.ytClient = msg.client
			// If the playlists tab is open, kick off a background refresh now.
			if m.activeTab == tabPlaylists && !m.ytPlLoading {
				m.ytPlLoading = true
				return m, youtube.FetchYTPlaylistsBackground(m.cfg)
			}
		}
		return m, nil

	case youtube.YTPlaylistsMsg:
		m.ytPlLoading = false
		if msg.Err != nil {
			if !msg.Background {
				m.setStatus("playlists: "+msg.Err.Error(), true)
			}
		} else {
			m.ytPlLoaded = true
			if ytPlaylistSetChanged(m.ytPlaylists, msg.Playlists) {
				m.ytPlaylists = msg.Playlists
				m.playlistVidCache = make(map[string][]youtube.Video)
			}
			go func(pls []youtube.YTPlaylist) {
				_ = m.db.SaveYTPlaylists(pls)
			}(msg.Playlists)
		}
		return m, nil

	case youtube.PlaylistVideosMsg:
		m.playlistVidLoading = false
		if msg.Err != nil {
			if len(m.playlistVidCache[msg.PlaylistID]) == 0 {
				m.setStatus("playlist: "+msg.Err.Error(), true)
			}
		} else {
			vids := msg.Videos
			sortVideos(vids, m.playlistSort)
			m.playlistVidCache[msg.PlaylistID] = vids
			go func(id string, v []youtube.Video) {
				_ = m.db.SaveYTPlaylistVideos(id, v)
			}(msg.PlaylistID, msg.Videos)
		}
		return m, nil

	case youtube.SubscribeMsg:
		if msg.Err != nil {
			m.setStatus("subscribe failed: "+msg.Err.Error(), true)
		} else {
			m.setStatus("Subscribed to: "+msg.ChannelName, false)
		}
		return m, nil

	case youtube.UnsubscribeMsg:
		if msg.Err != nil {
			m.setStatus("unsubscribe failed: "+msg.Err.Error(), true)
		} else {
			m.setStatus("Unsubscribed from: "+msg.ChannelName, false)
			delete(m.subscribedChannelIDs, msg.ChannelID)
			m.subChannels = removeChannelByID(m.subChannels, msg.ChannelID)
			m.recVideos = filterSubscribed(m.recVideos, m.subscribedChannelIDs)
		}
		return m, nil

	case youtube.CreatePlaylistMsg:
		if msg.Err != nil {
			m.setStatus("create playlist: "+msg.Err.Error(), true)
		} else {
			m.ytPlaylists = append(m.ytPlaylists, youtube.YTPlaylist{ID: msg.ID, Title: msg.Name})
			m.setStatus("Created playlist: "+msg.Name, false)
		}
		return m, nil

	case youtube.FetchResultMsg:
		return m.handleFetchResult(msg)

	case youtube.ChannelListMsg:
		m.subChLoading = false
		m.subChLoaded = true
		if msg.Err != nil {
			if !msg.Background {
				m.setStatus("channels: "+msg.Err.Error(), true)
			}
		} else {
			// Update channel list only when membership changed (added or removed).
			if channelSetChanged(m.subChannels, msg.Channels) {
				m.subChannels = msg.Channels
			}
			for _, ch := range msg.Channels {
				if ch.ID != "" {
					m.subscribedChannelIDs[ch.ID] = true
				}
				if ch.Name != "" {
					m.subscribedChannelIDs["name:"+strings.ToLower(ch.Name)] = true
				}
			}
			m.recVideos = filterSubscribed(m.recVideos, m.subscribedChannelIDs)
			go func(channels []youtube.Channel, videos []youtube.Video) {
				_ = m.db.SaveSubscribedChannels(channels)
				_ = m.db.SaveFeedCache("recommended", videos)
			}(msg.Channels, m.recVideos)
			// Always fetch latest N in background — full fetch only happens on explicit channel entry.
			var bgCmds []tea.Cmd
			for _, ch := range msg.Channels {
				if ch.ID == "" {
					continue
				}
				ch := ch
				bgCmds = append(bgCmds, youtube.FetchChannelLatestN(m.cfg, ch.URL, ch.ID, m.cfg.ChannelLatestCount))
			}
			if len(bgCmds) > 0 {
				return m, tea.Batch(bgCmds...)
			}
		}
		return m, nil

	case youtube.ChannelVideosMsg:
		if msg.Source == "search" {
			m.searchChLoading = false
			if msg.Err != nil {
				m.setStatus("channel videos: "+msg.Err.Error(), true)
			} else {
				m.searchChVideos = msg.Videos
				m.searchChVidCursor = 0
			}
		} else if msg.Source == "ch-background" {
			// Background latest-video fetch: merge and persist; rebuild subVideos if newer found.
			if msg.ChannelID == m.subChActiveID && m.subChPane == 1 {
				m.subChVidRefreshing = false
			}
			if msg.Err == nil && len(msg.Videos) > 0 {
				newest := msg.Videos[0]
				existing, ok := m.subChLatest[msg.ChannelID]
				if !ok || newest.UploadDate > existing.UploadDate {
					m.subChLatest[msg.ChannelID] = newest
					go func(chID string, vids []youtube.Video) {
						_ = m.db.SaveChannelVideos(chID, vids)
					}(msg.ChannelID, msg.Videos)
					m.rebuildSubVideos()
				}
			}
		} else {
			m.subChVidLoading = false
			m.subChVidRefreshing = false
			if msg.Err != nil {
				m.setStatus("channel videos: "+msg.Err.Error(), true)
			} else if msg.ChannelID != m.subChActiveID || m.subChPane != 1 {
				// Stale response — user navigated away; save to DB but don't touch UI.
				if len(msg.Videos) > 0 {
					go func(chID string, vids []youtube.Video) {
						_ = m.db.SaveChannelVideos(chID, vids)
					}(msg.ChannelID, msg.Videos)
				}
			} else {
				// Merge fetched videos with any already-loaded DB cache.
				merged := mergeVideos(m.subChVideos, msg.Videos)
				sortVideos(merged, m.subChVidSort)
				m.subChVideos = merged
				m.subChVidCursor = 0
				// Update latest-video entry and persist.
				if len(merged) > 0 {
					latest := merged[0]
					if existing, ok := m.subChLatest[msg.ChannelID]; !ok || latest.UploadDate > existing.UploadDate {
						m.subChLatest[msg.ChannelID] = latest
					}
					go func(chID string, vids []youtube.Video) {
						_ = m.db.SaveChannelVideos(chID, vids)
					}(msg.ChannelID, merged)
					m.rebuildSubVideos()
				}
			}
		}
		return m, nil

	case youtube.SearchResultMsg:
		m.searchLoading = false
		if msg.Err != nil {
			m.setStatus("search: "+msg.Err.Error(), true)
		} else {
			m.searchChannels = msg.Channels
			m.searchVideos = msg.Videos
			m.searchCursor = 0
			m.searchChSel = nil
			m.searchChVideos = nil
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
		m.recPage++
		m.recLoading = false
		m.recRefreshing = false
		m.recLoaded = true
		if msg.Err == nil {
			merged := mergeVideos(m.recVideos, msg.Videos)
			filtered := filterByAge(merged, m.cfg.RecommendedMaxAgeDays)
			filtered = filterDownloaded(filtered, m.localVideoIDs)
			filtered = filterHidden(filtered, m.recHidden)
			filtered = filterBlacklisted(filtered, m.cfg.BlacklistedChannels, m.cfg)
			filtered = filterSubscribed(filtered, m.subscribedChannelIDs)
			sortVideos(filtered, m.recSort)
			m.recCursor = preserveCursor(m.recVideos, m.recCursor, filtered)
			m.recVideos = filtered
			go m.db.SaveFeedCache("recommended", filtered)

			// If too few results and we haven't hit the page cap, fetch again.
			maxPages := m.cfg.RecommendedMaxPages
			if maxPages <= 0 {
				maxPages = 3
			}
			if len(filtered) < 20 && m.recPage < maxPages {
				m.recLoading = true
				m.recRefreshing = true
				return m, youtube.FetchRecommended(m.cfg)
			}
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
			m.localVideoIDs = buildLocalIDMap(lv)
		}
		playKey := ev.VideoID
		if ev.Type == downloader.TypeAudio {
			playKey = ev.VideoID + ":audio"
		}
		if m.playAfterDownload[playKey] {
			delete(m.playAfterDownload, playKey)
			if lv, ok := m.localVideoIDs[ev.VideoID]; ok {
				m.launchVideo(lv)
			}
		}
	case downloader.EventError:
		m.setStatus("Download failed: "+ev.Err.Error(), true)
	}
}

func (m Model) handleLocalFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	prev := m.localFilterInput.Value()
	switch msg.String() {
	case "esc":
		m.localFilterFocused = false
		m.localFilterInput.SetValue("")
		m.localFilter = ""
		m.localFilterCursor = 0
		return m, nil
	case "enter":
		m.localFilterFocused = false
		return m, nil
	default:
		var cmd tea.Cmd
		m.localFilterInput, cmd = m.localFilterInput.Update(msg)
		if m.localFilterInput.Value() != prev {
			m.localFilter = m.localFilterInput.Value()
			m.localFilterCursor = 0
		}
		return m, cmd
	}
}

var tabDebugNames = [numTabIDs]string{
	"recommended", "subscriptions", "playlists", "search", "downloading", "local", "history",
}

func tabName(id int) string {
	if id >= 0 && id < len(tabDebugNames) {
		return tabDebugNames[id]
	}
	return "unknown"
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	debug.Log("key=%q tab=%s localFilterFocused=%v localFilter=%q searchFocused=%v createMode=%v addOverlay=%v showHelp=%v pendingChord=%q gPending=%v numPrefix=%q",
		msg.String(), tabName(m.activeTab), m.localFilterFocused, m.localFilter, m.searchFocused,
		m.createMode, m.addOverlay, m.showHelp, m.pendingChord, m.gPending, m.numPrefix)

	if m.localFilterFocused {
		debug.Log("→ handleLocalFilter (localFilterFocused=true)")
		return m.handleLocalFilter(msg)
	}
	if m.searchFocused {
		debug.Log("→ handleSearchInput (searchFocused=true)")
		return m.handleSearchInput(msg)
	}
	if m.createMode {
		debug.Log("→ handleCreateInput (createMode=true)")
		return m.handleCreateInput(msg)
	}
	if m.addOverlay {
		debug.Log("→ handleAddOverlay (addOverlay=true)")
		return m.handleAddOverlay(msg)
	}
	if m.showHelp {
		debug.Log("→ dismiss help")
		m.showHelp = false
		return m, nil
	}

	s := msg.String()
	kb := m.cfg.Keybindings

	// ── Pending chord: accumulate keys and resolve ────────────────────────
	if m.pendingChord != "" {
		debug.Log("→ resolveChord(%q) pending=%q buf=%q", s, m.pendingChord, m.chordBuffer)
		return m.resolveChord(s)
	}

	// ── Vim-style number prefix (digits 1–9 always accumulate) ───────────
	if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
		m.numPrefix += s
		m.gPending = false
		return m, nil
	}
	if s == "0" && m.numPrefix != "" {
		m.numPrefix += "0"
		return m, nil
	}

	// ── GotoBottom (configurable; consumes numPrefix) ─────────────────────
	if key.Matches(msg, m.keys.GotoBottom) {
		n := m.parseNumPrefix()
		m.numPrefix = ""
		m.gPending = false
		if n > 0 {
			m.jumpToLine(n - 1)
		} else {
			m.jumpToLast()
		}
		return m, nil
	}

	// ── GotoPrefix chord: press twice for top ─────────────────────────────
	if s == kb.GotoPrefix {
		if m.gPending {
			m.gPending = false
			m.numPrefix = ""
			m.jumpToLine(0)
			return m, nil
		}
		m.gPending = true
		return m, nil
	}

	// Any other key resets number/goto prefix state.
	m.numPrefix = ""
	m.gPending = false

	// ── Chord trigger detection ───────────────────────────────────────────
	if s == kb.TabChord {
		m.pendingChord = kb.TabChord
		m.chordBuffer = ""
		debug.Log("→ tab chord pending")
		return m, nil
	}
	if s == kb.SortChord && m.contextSupportsSorting() {
		m.pendingChord = kb.SortChord
		m.chordBuffer = ""
		debug.Log("→ sort chord pending")
		return m, nil
	}

	switch {
	// ── Tab switching ─────────────────────────────────────────────────────
	case key.Matches(msg, m.keys.Tab):
		idx := m.currentTabIndex()
		m.activeTab = m.tabs[(idx+1)%len(m.tabs)]
		debug.Log("→ tab switch → %s", tabName(m.activeTab))
		return m, m.onTabActivated()
	case key.Matches(msg, m.keys.ShiftTab):
		idx := m.currentTabIndex()
		m.activeTab = m.tabs[(idx+len(m.tabs)-1)%len(m.tabs)]
		debug.Log("→ tab switch ← %s", tabName(m.activeTab))
		return m, m.onTabActivated()

	// ── Global actions ────────────────────────────────────────────────────
	case key.Matches(msg, m.keys.Quit):
		debug.Log("→ quit")
		m.playerBackend.Close()
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		debug.Log("→ toggle help")
		m.showHelp = !m.showHelp
		return m, nil
	case key.Matches(msg, m.keys.ForceRefresh):
		debug.Log("→ force refresh")
		return m, m.forceRefresh()
	case key.Matches(msg, m.keys.Refresh):
		debug.Log("→ refresh")
		return m, m.refresh()
	case key.Matches(msg, m.keys.Filter):
		debug.Log("→ activate local filter")
		m.localFilterInput.SetValue("")
		m.localFilter = ""
		m.localFilterCursor = 0
		m.localFilterFocused = true
		m.localFilterInput.Focus()
		return m, textinput.Blink
	}

	// When a local filter is active, override up/down/page navigation globally.
	if m.localFilter != "" {
		filtered := m.localFilteredVideos()
		n := len(filtered)
		switch {
		case key.Matches(msg, m.keys.Up):
			m.localFilterCursor = clamp(m.localFilterCursor-1, n)
			return m, nil
		case key.Matches(msg, m.keys.Down):
			m.localFilterCursor = clamp(m.localFilterCursor+1, n)
			return m, nil
		case key.Matches(msg, m.keys.PageUp):
			m.localFilterCursor = clamp(m.localFilterCursor-m.pageSize(), n)
			return m, nil
		case key.Matches(msg, m.keys.PageDown):
			m.localFilterCursor = clamp(m.localFilterCursor+m.pageSize(), n)
			return m, nil
		}
		// other keys (download, play, etc.) fall through to tab handlers but
		// currentVideo() will use localFilterCursor + filtered list
	}

	debug.Log("→ dispatch to %s handler", tabName(m.activeTab))
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

// resolveChord accumulates s into chordBuffer and dispatches against the
// generic chord registry. Supports multi-char completions: stays pending on a
// valid prefix, fires on exact match, cancels on no match.
func (m Model) resolveChord(s string) (tea.Model, tea.Cmd) {
	m.chordBuffer += s
	buf := m.chordBuffer
	ctx := m.currentContext()

	for _, chord := range m.chordDefs() {
		if chord.trigger != m.pendingChord {
			continue
		}
		valid := validActions(chord.actions, ctx)

		for _, a := range valid {
			if buf == a.key {
				m.pendingChord = ""
				m.chordBuffer = ""
				return a.exec(m)
			}
		}
		for _, a := range valid {
			if strings.HasPrefix(a.key, buf) {
				return m, nil // valid prefix — stay pending
			}
		}
		// No match and no prefix.
		m.pendingChord = ""
		m.chordBuffer = ""
		return m, nil
	}

	m.pendingChord = ""
	m.chordBuffer = ""
	return m, nil
}

// applySortAction applies a sort to the appropriate state for the current tab/context.
func (m Model) applySortAction(action string, vidSort int, ctx ContextID) (Model, tea.Cmd) {
	if ctx == CtxChannelList {
		sorted := m.sortedChannels()
		var selID string
		if m.subChCursor < len(sorted) {
			selID = sorted[m.subChCursor].ID
		}
		switch action {
		case "date":
			m.subChSort = subChSortDate
		case "name":
			m.subChSort = subChSortVidName
		case "channel":
			m.subChSort = subChSortName
		case "subscribers":
			m.subChSort = subChSortSubs
		case "views":
			m.subChSort = subChSortViews
		case "duration":
			m.subChSort = subChSortDuration
		}
		if selID != "" {
			for i, ch := range m.sortedChannels() {
				if ch.ID == selID {
					m.subChCursor = i
					break
				}
			}
		}
		return m, nil
	}

	if ctx == CtxLocal {
		m.localSort = vidSort
		sortLocalVideos(m.localVideos, vidSort)
		return m, nil
	}

	// Video-list contexts: apply to the appropriate tab slice.
	switch m.activeTab {
	case tabRecommended:
		m.recSort = vidSort
		sortVideos(m.recVideos, vidSort)
	case tabSubscriptions:
		if m.subMode == subModeChannels && m.subChPane == 1 {
			m.subChVidSort = vidSort
			sortVideos(m.subChVideos, vidSort)
		} else {
			m.subSort = vidSort
			sortVideos(m.subVideos, vidSort)
		}
	case tabSearch:
		m.searchSort = vidSort
		if m.searchChSel != nil {
			sortVideos(m.searchChVideos, vidSort)
		} else {
			sortVideos(m.searchVideos, vidSort)
		}
	case tabPlaylists:
		m.playlistSort = vidSort
		plKey := m.selectedPlaylistKey()
		if vids, ok := m.playlistVidCache[plKey]; ok {
			sortVideos(vids, vidSort)
		}
	}
	return m, nil
}

// ── Tab activation ────────────────────────────────────────────────────────────

func (m *Model) onTabActivated() tea.Cmd {
	// Always clear search focus when switching tabs — prevents searchFocused
	// leaking to other tabs (e.g. t+chord while search box is active types
	// into the input instead of triggering the chord).
	if m.activeTab != tabSearch {
		m.searchFocused = false
		m.searchInput.Blur()
	}
	// Clear local filter state on tab switch so it can't block tab-specific keys.
	m.localFilter = ""
	m.localFilterInput.SetValue("")
	m.localFilterFocused = false
	m.localFilterCursor = 0
	switch m.activeTab {
	case tabRecommended:
		if !m.recLoading {
			m.recLoading = true
			m.recRefreshing = m.recLoaded
			m.recPage = 0
			return youtube.FetchRecommended(m.cfg)
		}
	case tabSearch:
		m.searchFocused = true
		m.searchInput.Focus()
		if queries, err := m.db.SearchQueries(50); err == nil {
			m.searchHistory = queries
		}
		m.searchHistIdx = -1
		return textinput.Blink
	case tabSubscriptions:
		if m.subMode == subModeChannels && !m.subChLoading {
			m.subChLoading = true
			if m.subChLoaded {
				// Already have data — refresh silently in background.
				return youtube.FetchSubscribedChannelsBackground(m.cfg)
			}
			return youtube.FetchSubscribedChannels(m.cfg)
		}
	case tabPlaylists:
		if m.ytClient != nil && !m.ytPlLoading {
			m.ytPlLoading = true
			if m.ytPlLoaded {
				return youtube.FetchYTPlaylistsBackground(m.cfg)
			}
			return youtube.FetchYTPlaylists(m.cfg)
		}
	case tabHistory:
		return m.loadHistory()
	}
	return nil
}

func (m *Model) loadHistory() tea.Cmd {
	entries, err := m.db.HistoryVideos(200)
	if err != nil {
		m.setStatus("history: "+err.Error(), true)
	} else {
		m.histEntries = entries
		m.histCursor = 0
		m.histDetailVideoID = ""
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
			m.recPage = 0
			return youtube.FetchRecommended(m.cfg)
		}
	case tabSubscriptions:
		if m.subMode == subModeChannels && m.subChPane == 1 {
			// Inside a channel's video pane: latest fetch for that channel.
			return m.fetchChannelLatest(m.subChActiveID)
		}
		// Channel list or all-video: refresh channel list + per-channel latest/full.
		m.subChLoading = true
		return youtube.FetchSubscribedChannels(m.cfg)
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
	case tabPlaylists:
		if m.playlistPane == 1 {
			return m.fetchCurrentPlaylistVideos()
		}
		if m.ytClient != nil && !m.ytPlLoading {
			m.ytPlLoading = true
			return youtube.FetchYTPlaylistsBackground(m.cfg)
		}
	}
	return nil
}

func (m *Model) forceRefresh() tea.Cmd {
	switch m.activeTab {
	case tabRecommended:
		if !m.recLoading {
			m.recLoading = true
			m.recRefreshing = m.recLoaded
			m.recPage = 0
			return youtube.FetchRecommended(m.cfg)
		}
	case tabSubscriptions:
		if m.subMode == subModeChannels && m.subChPane == 1 {
			// Full fetch for the open channel.
			return youtube.FetchChannelVideos(m.cfg, m.channelURL(m.subChActiveID), m.subChActiveID, "subscriptions")
		}
		// Full fetch for all subscribed channels regardless of existing data.
		return m.forceRefreshAllChannels()
	case tabSearch:
		if m.lastQuery != "" {
			m.searchLoading = true
			return youtube.Search(m.cfg, m.lastQuery)
		}
	case tabPlaylists:
		return m.fetchCurrentPlaylistVideos()
	}
	return nil
}

// fetchChannelLatest fires a background latest-N fetch for a channel.
func (m *Model) fetchChannelLatest(channelID string) tea.Cmd {
	if channelID == "" {
		return nil
	}
	ch := m.channelByID(channelID)
	return youtube.FetchChannelLatestN(m.cfg, ch.URL, channelID, m.cfg.ChannelLatestCount)
}

// forceRefreshAllChannels fires a full fetch for every subscribed channel.
func (m *Model) forceRefreshAllChannels() tea.Cmd {
	var cmds []tea.Cmd
	for _, ch := range m.subChannels {
		if ch.ID == "" {
			continue
		}
		ch := ch
		cmds = append(cmds, youtube.FetchChannelVideos(m.cfg, ch.URL, ch.ID, "subscriptions"))
	}
	return tea.Batch(cmds...)
}

// channelByID returns the Channel struct for a given ID, or an empty Channel.
func (m *Model) channelByID(id string) youtube.Channel {
	for _, ch := range m.subChannels {
		if ch.ID == id {
			return ch
		}
	}
	return youtube.Channel{ID: id}
}

// rebuildSubVideos re-queries GetAllChannelVideos and re-sorts by the current subSort.
func (m *Model) rebuildSubVideos() {
	ids := make([]string, 0, len(m.subChannels))
	for _, ch := range m.subChannels {
		if ch.ID != "" {
			ids = append(ids, ch.ID)
		}
	}
	if videos, err := m.db.GetAllChannelVideos(ids); err == nil {
		sortVideos(videos, m.subSort)
		m.subCursor = preserveCursor(m.subVideos, m.subCursor, videos)
		m.subVideos = videos
	}
}

// channelURL returns a channel's URL from subChannels, falling back to the ID-based URL.
func (m *Model) channelURL(id string) string {
	ch := m.channelByID(id)
	if ch.URL != "" {
		return ch.URL
	}
	return "https://www.youtube.com/channel/" + id
}

// fetchCurrentPlaylistVideos fires a full fetch for the currently open YT playlist.
func (m *Model) fetchCurrentPlaylistVideos() tea.Cmd {
	key := m.selectedPlaylistKey()
	if key == "" || !m.selectedPlaylistIsYT() {
		return nil
	}
	m.playlistVidLoading = true
	return youtube.FetchPlaylistVideos(m.cfg, key)
}

// ── Video tabs: Recommended ───────────────────────────────────────────────────

func (m Model) updateRecommended(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.recVideos)
	debug.Log("updateRecommended: key=%q hideVideo=%q hideChannel=%q",
		msg.String(), m.cfg.Keybindings.HideVideo, m.cfg.Keybindings.HideChannel)
	switch {
	case key.Matches(msg, m.keys.Up):
		m.recCursor, m.recVS = vsMove(m.recCursor, m.recVS, n, -1, m.pageSize())
	case key.Matches(msg, m.keys.Down):
		m.recCursor, m.recVS = vsMove(m.recCursor, m.recVS, n, +1, m.pageSize())
	case key.Matches(msg, m.keys.PageUp):
		m.recCursor, m.recVS = vsPage(m.recCursor, m.recVS, n, -1, m.pageSize())
	case key.Matches(msg, m.keys.PageDown):
		m.recCursor, m.recVS = vsPage(m.recCursor, m.recVS, n, +1, m.pageSize())
	case key.Matches(msg, m.keys.HideVideo):
		if v, ok := m.currentVideo(); ok {
			_ = m.db.HideRecVideo(v.ID)
			m.recHidden[v.ID] = true
			m.recVideos = removeVideoByID(m.recVideos, v.ID)
			m.recCursor, m.recVS = vsMove(clamp(m.recCursor, len(m.recVideos)), m.recVS, len(m.recVideos), 0, m.pageSize())
			m.setStatus("Hidden: "+truncate(v.Title, 50), false)
			m.checkVideoHideAutoBlacklist(v.ChannelID, v.Channel)
		}
	case key.Matches(msg, m.keys.HideChannel):
		debug.Log("recommended: hide channel matched")
		if v, ok := m.currentVideo(); ok {
			m.hideChannel(v.ChannelID, v.Channel)
		}
	case key.Matches(msg, m.keys.DrillDown):
		if v, ok := m.currentVideo(); ok {
			m.downloadAndPlay(v)
		}
	case key.Matches(msg, m.keys.Play):
		if v, ok := m.currentVideo(); ok {
			m.downloadAndPlay(v)
		}
	case key.Matches(msg, m.keys.PlayAudio):
		if v, ok := m.currentVideo(); ok {
			m.downloadAndPlayAudio(v)
		}
	case key.Matches(msg, m.keys.Download):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeVideo)
		}
	case key.Matches(msg, m.keys.DownloadAudio):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeAudio)
		}
	case key.Matches(msg, m.keys.WatchLater):
		if v, ok := m.currentVideo(); ok {
			m.addToWatchLater(v)
			delete(m.playlistVidCache, ytWatchLaterID)
			m.setStatus("Added to Watch Later: "+truncate(v.Title, 50), false)
		}
	case key.Matches(msg, m.keys.AddList):
		if v, ok := m.currentVideo(); ok {
			m.openAddOverlay(v)
		}
	case key.Matches(msg, m.keys.CopyURL):
		m.copyCurrentURL()
	case key.Matches(msg, m.keys.Subscribe):
		return m.subscribeCurrentChannel()
	}
	return m, nil
}

// ── Subscriptions ─────────────────────────────────────────────────────────────

func (m Model) updateSubscriptions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	toggleKey := m.cfg.Keybindings.ToggleMode
	if toggleKey == "" {
		toggleKey = "m"
	}
	debug.Log("updateSubscriptions: key=%q toggleKey=%q match=%v subMode=%d subChLoaded=%v subChLoading=%v",
		msg.String(), toggleKey, msg.String() == toggleKey, m.subMode, m.subChLoaded, m.subChLoading)
	switch {
	case msg.String() == toggleKey:
		if m.subMode == subModeAll {
			debug.Log("→ toggle subModeAll→subModeChannels (chLoaded=%v chLoading=%v)", m.subChLoaded, m.subChLoading)
			m.subMode = subModeChannels
			if !m.subChLoaded && !m.subChLoading {
				m.subChLoading = true
				return m, youtube.FetchSubscribedChannels(m.cfg)
			}
		} else {
			debug.Log("→ toggle subModeChannels→subModeAll")
			m.subMode = subModeAll
			m.subChPane = 0
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
	case key.Matches(msg, m.keys.Up):
		m.subCursor, m.subVS = vsMove(m.subCursor, m.subVS, n, -1, m.pageSize())
	case key.Matches(msg, m.keys.Down):
		m.subCursor, m.subVS = vsMove(m.subCursor, m.subVS, n, +1, m.pageSize())
	case key.Matches(msg, m.keys.PageUp):
		m.subCursor, m.subVS = vsPage(m.subCursor, m.subVS, n, -1, m.pageSize())
	case key.Matches(msg, m.keys.PageDown):
		m.subCursor, m.subVS = vsPage(m.subCursor, m.subVS, n, +1, m.pageSize())
	case key.Matches(msg, m.keys.DrillDown):
		if v, ok := m.currentVideo(); ok {
			m.downloadAndPlay(v)
		}
	case key.Matches(msg, m.keys.Play):
		if v, ok := m.currentVideo(); ok {
			m.downloadAndPlay(v)
		}
	case key.Matches(msg, m.keys.PlayAudio):
		if v, ok := m.currentVideo(); ok {
			m.downloadAndPlayAudio(v)
		}
	case key.Matches(msg, m.keys.Download):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeVideo)
		}
	case key.Matches(msg, m.keys.DownloadAudio):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeAudio)
		}
	case key.Matches(msg, m.keys.WatchLater):
		if v, ok := m.currentVideo(); ok {
			m.addToWatchLater(v)
			delete(m.playlistVidCache, ytWatchLaterID)
			m.setStatus("Added to Watch Later: "+truncate(v.Title, 50), false)
		}
	case key.Matches(msg, m.keys.AddList):
		if v, ok := m.currentVideo(); ok {
			m.openAddOverlay(v)
		}
	case key.Matches(msg, m.keys.CopyURL):
		m.copyCurrentURL()
	case key.Matches(msg, m.keys.Unsubscribe):
		return m.unsubscribeCurrentChannel()
	}
	return m, nil
}

func (m Model) updateSubChannels(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.subChPane == 0 {
		sorted := m.sortedChannels()
		n := len(sorted)
		switch {
		case key.Matches(msg, m.keys.Up):
			m.subChCursor, m.subChVS = vsMove(m.subChCursor, m.subChVS, n, -1, m.pageSize())
		case key.Matches(msg, m.keys.Down):
			m.subChCursor, m.subChVS = vsMove(m.subChCursor, m.subChVS, n, +1, m.pageSize())
		case key.Matches(msg, m.keys.DrillDown), key.Matches(msg, m.keys.Right):
			if m.subChCursor < n {
				ch := sorted[m.subChCursor]
				m.subChVidCursor = 0
				m.subChPane = 1
				if ch.ID == m.subChActiveID && len(m.subChVideos) > 0 {
					// Re-entering same channel — reuse in-memory data, refresh in background.
					m.subChVidLoading = false
					m.subChVidRefreshing = true
					return m, youtube.FetchChannelLatestN(m.cfg, ch.URL, ch.ID, m.cfg.ChannelLatestCount)
				}
				m.subChActiveID = ch.ID
				if cached, err := m.db.GetChannelVideos(ch.ID); err == nil && len(cached) > 0 {
					// Has cached data — show immediately, fetch latest in background.
					m.subChVideos = cached
					m.subChVidLoading = false
					m.subChVidRefreshing = true
					return m, youtube.FetchChannelLatestN(m.cfg, ch.URL, ch.ID, m.cfg.ChannelLatestCount)
				}
				// No data — full fetch.
				m.subChVideos = nil
				m.subChVidLoading = true
				m.subChVidRefreshing = false
				return m, youtube.FetchChannelVideos(m.cfg, ch.URL, ch.ID, "subscriptions")
			}
		case key.Matches(msg, m.keys.Unsubscribe):
			if m.subChCursor < n {
				ch := sorted[m.subChCursor]
				return m.unsubscribeChannel(ch.ID, ch.Name)
			}
		}
		return m, nil
	}

	n := len(m.subChVideos)
	switch {
	case key.Matches(msg, m.keys.Left), key.Matches(msg, m.keys.Escape):
		m.subChPane = 0
	case key.Matches(msg, m.keys.Up):
		m.subChVidCursor, m.subChVidVS = vsMove(m.subChVidCursor, m.subChVidVS, n, -1, m.pageSize())
	case key.Matches(msg, m.keys.Down):
		m.subChVidCursor, m.subChVidVS = vsMove(m.subChVidCursor, m.subChVidVS, n, +1, m.pageSize())
	case key.Matches(msg, m.keys.PageUp):
		m.subChVidCursor, m.subChVidVS = vsPage(m.subChVidCursor, m.subChVidVS, n, -1, m.pageSize())
	case key.Matches(msg, m.keys.PageDown):
		m.subChVidCursor, m.subChVidVS = vsPage(m.subChVidCursor, m.subChVidVS, n, +1, m.pageSize())
	case key.Matches(msg, m.keys.DrillDown):
		if v, ok := m.currentVideo(); ok {
			m.downloadAndPlay(v)
		}
	case key.Matches(msg, m.keys.Play):
		if v, ok := m.currentVideo(); ok {
			m.downloadAndPlay(v)
		}
	case key.Matches(msg, m.keys.PlayAudio):
		if v, ok := m.currentVideo(); ok {
			m.downloadAndPlayAudio(v)
		}
	case key.Matches(msg, m.keys.Download):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeVideo)
		}
	case key.Matches(msg, m.keys.DownloadAudio):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeAudio)
		}
	case key.Matches(msg, m.keys.WatchLater):
		if v, ok := m.currentVideo(); ok {
			m.addToWatchLater(v)
			delete(m.playlistVidCache, ytWatchLaterID)
			m.setStatus("Added to Watch Later: "+truncate(v.Title, 50), false)
		}
	case key.Matches(msg, m.keys.AddList):
		if v, ok := m.currentVideo(); ok {
			m.openAddOverlay(v)
		}
	case key.Matches(msg, m.keys.CopyURL):
		m.copyCurrentURL()
	case key.Matches(msg, m.keys.Unsubscribe):
		return m.unsubscribeCurrentChannel()
	}
	return m, nil
}

// ── Playlists ─────────────────────────────────────────────────────────────────

// loadCurrentPlaylistVideos loads videos for the selected playlist.
// For YT playlists: loads from DB immediately, always fires a background fetch to detect changes.
// For local playlists: reads from DB synchronously.
func (m *Model) loadCurrentPlaylistVideos() tea.Cmd {
	plKey := m.selectedPlaylistKey()
	if plKey == "" {
		return nil
	}

	if m.ytPlLoaded && parseLocalPlaylistID(plKey) == 0 {
		// Load from DB immediately for instant display.
		if cached, err := m.db.GetYTPlaylistVideos(plKey); err == nil && len(cached) > 0 {
			m.playlistVidCache[plKey] = cached
		}
		// Fire a background YouTube fetch only when the client is available.
		if m.ytClient != nil {
			m.playlistVidLoading = true
			return youtube.FetchPlaylistVideos(m.cfg, plKey)
		}
		return nil
	}

	// Local playlist — synchronous DB read.
	localID := parseLocalPlaylistID(plKey)
	if localID > 0 {
		vids, _ := m.db.PlaylistVideos(localID)
		m.playlistVidCache[plKey] = vids
	}
	return nil
}

// parseLocalPlaylistID extracts the int64 ID from a "local:<id>" cache key.
func parseLocalPlaylistID(key string) int64 {
	if !strings.HasPrefix(key, "local:") {
		return 0
	}
	id, _ := strconv.ParseInt(strings.TrimPrefix(key, "local:"), 10, 64)
	return id
}

func (m Model) updatePlaylists(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.playlistPane == 0 {
		n := m.playlistCount()
		switch {
		case key.Matches(msg, m.keys.Up):
			m.playlistCursor, m.playlistVS = vsMove(m.playlistCursor, m.playlistVS, n, -1, m.pageSize())
		case key.Matches(msg, m.keys.Down):
			m.playlistCursor, m.playlistVS = vsMove(m.playlistCursor, m.playlistVS, n, +1, m.pageSize())
		case key.Matches(msg, m.keys.DrillDown), key.Matches(msg, m.keys.Right):
			if m.playlistCursor < n {
				cmd := m.loadCurrentPlaylistVideos()
				m.playlistPane = 1
				m.playlistVidCursor = 0
				return m, cmd
			}
		case key.Matches(msg, m.keys.NewList):
			m.createMode = true
			m.createInput.SetValue("")
			m.createInput.Focus()
			return m, textinput.Blink
		case key.Matches(msg, m.keys.Delete):
			plKey := m.selectedPlaylistKey()
			if plKey == ytWatchLaterID {
				break // Watch Later cannot be deleted
			}
			idx := m.playlistCursor
			if m.ytPlLoaded && m.ytClient != nil && idx < len(m.ytPlaylists) {
				pl := m.ytPlaylists[idx]
				go func() { _ = m.ytClient.DeletePlaylist(pl.ID) }()
				delete(m.playlistVidCache, pl.ID)
				m.ytPlaylists = append(m.ytPlaylists[:idx], m.ytPlaylists[idx+1:]...)
			} else if idx < len(m.playlists) {
				pl := m.playlists[idx]
				_ = m.db.DeletePlaylist(pl.ID)
				delete(m.playlistVidCache, fmt.Sprintf("local:%d", pl.ID))
				playlists, _ := m.db.Playlists()
				m.playlists = playlists
			}
			m.playlistCursor, m.playlistVS = vsMove(clamp(m.playlistCursor, m.playlistCount()), m.playlistVS, m.playlistCount(), 0, m.pageSize())
		case key.Matches(msg, m.keys.Subscribe):
			return m.subscribeCurrentChannel()
		}
		return m, nil
	}

	if m.playlistCursor >= m.playlistCount() {
		m.playlistPane = 0
		return m, nil
	}
	plKey := m.selectedPlaylistKey()
	vids := m.playlistVidCache[plKey]
	n := len(vids)

	switch {
	case key.Matches(msg, m.keys.Left), key.Matches(msg, m.keys.Escape):
		m.playlistPane = 0
	case key.Matches(msg, m.keys.Up):
		m.playlistVidCursor, m.playlistVidVS = vsMove(m.playlistVidCursor, m.playlistVidVS, n, -1, m.pageSize())
	case key.Matches(msg, m.keys.Down):
		m.playlistVidCursor, m.playlistVidVS = vsMove(m.playlistVidCursor, m.playlistVidVS, n, +1, m.pageSize())
	case key.Matches(msg, m.keys.PageUp):
		m.playlistVidCursor, m.playlistVidVS = vsPage(m.playlistVidCursor, m.playlistVidVS, n, -1, m.pageSize())
	case key.Matches(msg, m.keys.PageDown):
		m.playlistVidCursor, m.playlistVidVS = vsPage(m.playlistVidCursor, m.playlistVidVS, n, +1, m.pageSize())
	case key.Matches(msg, m.keys.DrillDown):
		if v, ok := m.currentVideo(); ok {
			m.downloadAndPlay(v)
		}
	case key.Matches(msg, m.keys.Delete):
		if m.playlistVidCursor < n {
			vid := vids[m.playlistVidCursor]
			if m.selectedPlaylistIsYT() {
				go func() { _ = m.ytClient.RemoveFromPlaylist(plKey, vid.ID) }()
			} else {
				localID := parseLocalPlaylistID(plKey)
				_ = m.db.RemoveFromPlaylist(localID, vid.ID)
			}
			// Optimistic removal from cache.
			updated := make([]youtube.Video, 0, len(vids)-1)
			for _, v := range vids {
				if v.ID != vid.ID {
					updated = append(updated, v)
				}
			}
			m.playlistVidCache[plKey] = updated
			m.playlistVidCursor, m.playlistVidVS = vsMove(clamp(m.playlistVidCursor, len(updated)), m.playlistVidVS, len(updated), 0, m.pageSize())
		}
	case key.Matches(msg, m.keys.Download):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeVideo)
		}
	case key.Matches(msg, m.keys.DownloadAudio):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeAudio)
		}
	case key.Matches(msg, m.keys.Subscribe):
		return m.subscribeCurrentChannel()
	}
	return m, nil
}

// ── Search ────────────────────────────────────────────────────────────────────

func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Filter key refocuses the search input when results are shown.
	if key.Matches(msg, m.keys.Filter) {
		m.searchFocused = true
		m.searchInput.Focus()
		return m, textinput.Blink
	}
	// ── Channel drill-down ───────────────────────────────────────────────────
	if m.searchChSel != nil {
		n := len(m.searchChVideos)
		switch {
		case key.Matches(msg, m.keys.Left), key.Matches(msg, m.keys.Escape):
			m.searchChSel = nil
			m.searchChVideos = nil
			m.localFilter = ""
			m.localFilterInput.SetValue("")
		case key.Matches(msg, m.keys.Up):
			m.searchChVidCursor, m.searchChVidVS = vsMove(m.searchChVidCursor, m.searchChVidVS, n, -1, m.pageSize())
		case key.Matches(msg, m.keys.Down):
			m.searchChVidCursor, m.searchChVidVS = vsMove(m.searchChVidCursor, m.searchChVidVS, n, +1, m.pageSize())
		case key.Matches(msg, m.keys.PageUp):
			m.searchChVidCursor, m.searchChVidVS = vsPage(m.searchChVidCursor, m.searchChVidVS, n, -1, m.pageSize())
		case key.Matches(msg, m.keys.PageDown):
			m.searchChVidCursor, m.searchChVidVS = vsPage(m.searchChVidCursor, m.searchChVidVS, n, +1, m.pageSize())
		case key.Matches(msg, m.keys.Play):
			if v, ok := m.currentVideo(); ok {
				m.downloadAndPlay(v)
			}
		case key.Matches(msg, m.keys.PlayAudio):
			if v, ok := m.currentVideo(); ok {
				m.downloadAndPlayAudio(v)
			}
		case key.Matches(msg, m.keys.Download):
			if v, ok := m.currentVideo(); ok {
				m.startDownload(v, downloader.TypeVideo)
			}
		case key.Matches(msg, m.keys.DownloadAudio):
			if v, ok := m.currentVideo(); ok {
				m.startDownload(v, downloader.TypeAudio)
			}
		case key.Matches(msg, m.keys.WatchLater):
			if v, ok := m.currentVideo(); ok {
				m.addToWatchLater(v)
				delete(m.playlistVidCache, ytWatchLaterID)
			}
		case key.Matches(msg, m.keys.CopyURL):
			m.copyCurrentURL()
		}
		return m, nil
	}

	// ── Channel + video results ───────────────────────────────────────────────
	totalChannels := len(m.searchChannels)
	totalVideos := len(m.searchVideos)
	total := totalChannels + totalVideos

	switch {
	case key.Matches(msg, m.keys.Up):
		m.searchCursor = clamp(m.searchCursor-1, total)
		m.updateSearchVS(totalChannels, totalVideos)
	case key.Matches(msg, m.keys.Down):
		m.searchCursor = clamp(m.searchCursor+1, total)
		m.updateSearchVS(totalChannels, totalVideos)
	case key.Matches(msg, m.keys.PageUp):
		m.searchCursor = clamp(m.searchCursor-m.pageSize(), total)
		m.updateSearchVS(totalChannels, totalVideos)
	case key.Matches(msg, m.keys.PageDown):
		m.searchCursor = clamp(m.searchCursor+m.pageSize(), total)
		m.updateSearchVS(totalChannels, totalVideos)
	case key.Matches(msg, m.keys.DrillDown), key.Matches(msg, m.keys.Right):
		if m.searchCursor < totalChannels {
			ch := m.searchChannels[m.searchCursor]
			m.searchChSel = &ch
			m.searchChVideos = nil
			m.searchChVidCursor = 0
			m.searchChLoading = true
			return m, youtube.FetchChannelVideos(m.cfg, ch.URL, ch.ID, "search")
		}
		// DrillDown on a video row plays it.
		if v, ok := m.currentVideo(); ok {
			m.downloadAndPlay(v)
		}
	case key.Matches(msg, m.keys.Play):
		if v, ok := m.currentVideo(); ok {
			m.downloadAndPlay(v)
		}
	case key.Matches(msg, m.keys.PlayAudio):
		if v, ok := m.currentVideo(); ok {
			m.downloadAndPlayAudio(v)
		}
	case key.Matches(msg, m.keys.Download):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeVideo)
		}
	case key.Matches(msg, m.keys.DownloadAudio):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeAudio)
		}
	case key.Matches(msg, m.keys.WatchLater):
		if v, ok := m.currentVideo(); ok {
			m.addToWatchLater(v)
			delete(m.playlistVidCache, ytWatchLaterID)
		}
	case key.Matches(msg, m.keys.AddList):
		if v, ok := m.currentVideo(); ok {
			m.openAddOverlay(v)
		}
	case key.Matches(msg, m.keys.CopyURL):
		m.copyCurrentURL()
	case key.Matches(msg, m.keys.Subscribe):
		return m.subscribeCurrentChannel()
	}
	return m, nil
}

func (m Model) handleSearchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	blurSearch := func() {
		m.searchFocused = false
		m.searchInput.Blur()
	}
	switch msg.String() {
	case "up":
		if len(m.searchHistory) == 0 {
			return m, nil
		}
		if m.searchHistIdx == -1 {
			m.searchDraft = m.searchInput.Value()
		}
		next := m.searchHistIdx + 1
		if next < len(m.searchHistory) {
			m.searchHistIdx = next
			m.searchInput.SetValue(m.searchHistory[m.searchHistIdx])
			m.searchInput.CursorEnd()
		}
		return m, nil
	case "down":
		if m.searchHistIdx == -1 {
			return m, nil
		}
		prev := m.searchHistIdx - 1
		if prev < 0 {
			m.searchHistIdx = -1
			m.searchInput.SetValue(m.searchDraft)
			m.searchInput.CursorEnd()
		} else {
			m.searchHistIdx = prev
			m.searchInput.SetValue(m.searchHistory[m.searchHistIdx])
			m.searchInput.CursorEnd()
		}
		return m, nil
	case "enter":
		q := m.searchInput.Value()
		if q == "" {
			blurSearch()
			return m, nil
		}
		m.lastQuery = q
		m.searchLoading = true
		m.searchHistIdx = -1
		blurSearch()
		_ = m.db.AddHistory("", "search", q)
		return m, youtube.Search(m.cfg, q)
	case "esc":
		if m.searchHistIdx != -1 {
			m.searchHistIdx = -1
			m.searchInput.SetValue(m.searchDraft)
			m.searchInput.CursorEnd()
			return m, nil
		}
		blurSearch()
		return m, nil
	case "tab":
		blurSearch()
		idx := m.currentTabIndex()
		m.activeTab = m.tabs[(idx+1)%len(m.tabs)]
		return m, m.onTabActivated()
	case "shift+tab":
		blurSearch()
		idx := m.currentTabIndex()
		m.activeTab = m.tabs[(idx+len(m.tabs)-1)%len(m.tabs)]
		return m, m.onTabActivated()
	case "f2":
		blurSearch()
		return m.switchToTabPos(0)
	case "f3":
		blurSearch()
		return m.switchToTabPos(1)
	case "f4":
		blurSearch()
		return m.switchToTabPos(2)
	case "f5":
		blurSearch()
		return m.switchToTabPos(3)
	case "f6":
		blurSearch()
		return m.switchToTabPos(4)
	case "f7":
		blurSearch()
		return m.switchToTabPos(5)
	case "f8":
		blurSearch()
		return m.switchToTabPos(6)
	default:
		m.searchHistIdx = -1
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
	case key.Matches(msg, m.keys.Up):
		m.dlCursor, m.dlVS = vsMove(m.dlCursor, m.dlVS, n, -1, m.pageSize())
	case key.Matches(msg, m.keys.Down):
		m.dlCursor, m.dlVS = vsMove(m.dlCursor, m.dlVS, n, +1, m.pageSize())
	case key.Matches(msg, m.keys.Play):
		if m.dlCursor < n {
			item := items[m.dlCursor]
			if item.Status == downloader.StatusComplete {
				if lv, ok := m.localVideoIDs[item.Video.ID]; ok {
					m.launchVideo(lv)
				}
			} else {
				m.playAfterDownload[item.Video.ID] = true
				m.setStatus("Will play video after download: "+truncate(item.Video.Title, 50), false)
			}
		}
	case key.Matches(msg, m.keys.PlayAudio):
		if m.dlCursor < n {
			m.downloadAndPlayAudio(items[m.dlCursor].Video)
		}
	case key.Matches(msg, m.keys.HideChannel):
		if m.dlCursor < n {
			v := items[m.dlCursor].Video
			m.hideChannel(v.ChannelID, v.Channel)
		}
	case key.Matches(msg, m.keys.Delete):
		if m.dlCursor < n {
			item := items[m.dlCursor]
			m.downloader.Remove(item.Video.ID)
			// Also remove any downloaded or partial file and DB record.
			if item.FilePath != "" {
				_ = os.Remove(item.FilePath)
			}
			_ = m.db.DeleteLocalVideo(item.Video.ID)
			_ = m.db.AddHistory(item.Video.ID, "delete", "")
			if lv, err := m.db.LocalVideos(); err == nil {
				m.localVideos = lv
			}
			m.dlCursor, m.dlVS = vsMove(clamp(m.dlCursor, len(m.downloader.Items())), m.dlVS, len(m.downloader.Items()), 0, m.pageSize())
			m.setStatus("Deleted: "+truncate(item.Video.Title, 50), false)
		}
	case key.Matches(msg, m.keys.CopyURL):
		m.copyCurrentURL()
	case key.Matches(msg, m.keys.Subscribe):
		return m.subscribeCurrentChannel()
	}
	return m, nil
}

// ── Local ─────────────────────────────────────────────────────────────────────

func (m Model) updateLocal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.localVideos)
	switch {
	case key.Matches(msg, m.keys.Up):
		m.localCursor, m.localVS = vsMove(m.localCursor, m.localVS, n, -1, m.pageSize())
	case key.Matches(msg, m.keys.Down):
		m.localCursor, m.localVS = vsMove(m.localCursor, m.localVS, n, +1, m.pageSize())
	case key.Matches(msg, m.keys.PageUp):
		m.localCursor, m.localVS = vsPage(m.localCursor, m.localVS, n, -1, m.pageSize())
	case key.Matches(msg, m.keys.PageDown):
		m.localCursor, m.localVS = vsPage(m.localCursor, m.localVS, n, +1, m.pageSize())
	case key.Matches(msg, m.keys.Play):
		if m.localCursor < n {
			m.launchVideo(m.localVideos[m.localCursor])
		}
	case key.Matches(msg, m.keys.Delete):
		if m.localCursor < n {
			lv := m.localVideos[m.localCursor]
			_ = os.Remove(lv.FilePath)
			_ = m.db.DeleteLocalVideo(lv.ID)
			_ = m.db.AddHistory(lv.ID, "delete", "")
			if lv2, err := m.db.LocalVideos(); err == nil {
				m.localVideos = lv2
			}
			m.localCursor, m.localVS = vsMove(clamp(m.localCursor, len(m.localVideos)), m.localVS, len(m.localVideos), 0, m.pageSize())
			m.setStatus("Deleted: "+truncate(lv.Title, 50), false)
		}
	case key.Matches(msg, m.keys.CopyURL):
		m.copyCurrentURL()
	case key.Matches(msg, m.keys.Subscribe):
		return m.subscribeCurrentChannel()
	}
	return m, nil
}

// ── History ───────────────────────────────────────────────────────────────────

func (m Model) updateHistory(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.histDetailVideoID != "" {
		switch {
		case key.Matches(msg, m.keys.Escape), key.Matches(msg, m.keys.Left):
			m.histDetailVideoID = ""
			m.histDetail = nil
		}
		return m, nil
	}
	n := len(m.histEntries)
	switch {
	case key.Matches(msg, m.keys.Up):
		m.histCursor, m.histVS = vsMove(m.histCursor, m.histVS, n, -1, m.pageSize())
	case key.Matches(msg, m.keys.Down):
		m.histCursor, m.histVS = vsMove(m.histCursor, m.histVS, n, +1, m.pageSize())
	case key.Matches(msg, m.keys.PageUp):
		m.histCursor, m.histVS = vsPage(m.histCursor, m.histVS, n, -1, m.pageSize())
	case key.Matches(msg, m.keys.PageDown):
		m.histCursor, m.histVS = vsPage(m.histCursor, m.histVS, n, +1, m.pageSize())
	case key.Matches(msg, m.keys.Play):
		if m.histCursor < n {
			e := m.histEntries[m.histCursor]
			if e.EventType != "search" {
				if lv, ok := m.localVideoIDs[e.VideoID]; ok {
					m.launchVideo(lv)
				} else {
					m.setStatus("Not downloaded: "+truncate(e.Title, 40), true)
				}
			}
		}
	case key.Matches(msg, m.keys.DrillDown), key.Matches(msg, m.keys.Right):
		if m.histCursor < n {
			e := m.histEntries[m.histCursor]
			if e.EventType == "search" {
				// Navigate to search tab with query pre-filled; user presses Enter again to search.
				m.activeTab = tabSearch
				cmd := m.onTabActivated()
				m.searchInput.SetValue(e.Details)
				m.searchInput.CursorEnd()
				return m, cmd
			}
			if entries, err := m.db.VideoHistory(e.VideoID); err == nil {
				m.histDetailVideoID = e.VideoID
				m.histDetail = entries
			}
		}
	case key.Matches(msg, m.keys.Delete):
		if m.histCursor < n {
			e := m.histEntries[m.histCursor]
			if e.EventType == "search" {
				_ = m.db.DeleteSearchHistory(e.Details)
				m.setStatus("Removed search: "+truncate(e.Details, 50), false)
			} else {
				if lv, ok := m.localVideoIDs[e.VideoID]; ok {
					_ = os.Remove(lv.FilePath)
					_ = m.db.DeleteLocalVideo(lv.ID)
					if lv2, err := m.db.LocalVideos(); err == nil {
						m.localVideos = lv2
						m.localVideoIDs = buildLocalIDMap(lv2)
					}
				}
				_ = m.db.DeleteVideoHistory(e.VideoID)
				m.setStatus("Deleted: "+truncate(e.Title, 50), false)
			}
			m.histEntries = append(m.histEntries[:m.histCursor], m.histEntries[m.histCursor+1:]...)
			if m.histCursor >= len(m.histEntries) && m.histCursor > 0 {
				m.histCursor--
			}
		}
	case key.Matches(msg, m.keys.HideChannel):
		if m.histCursor < n {
			e := m.histEntries[m.histCursor]
			if e.EventType != "search" {
				m.hideChannel(e.ChannelID, e.Channel)
			}
		}
	case key.Matches(msg, m.keys.Subscribe):
		return m.subscribeCurrentChannel()
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
			if m.ytClient != nil {
				return m, youtube.CreateYTPlaylist(m.ytClient, name)
			}
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
	n := m.overlayPlaylistCount()
	switch msg.String() {
	case "esc", "q":
		m.addOverlay = false
	case "k", "up":
		m.addOverlaySel = clamp(m.addOverlaySel-1, n)
	case "j", "down":
		m.addOverlaySel = clamp(m.addOverlaySel+1, n)
	case "enter":
		v := m.addVideo
		idx := m.addOverlaySel
		if m.ytPlLoaded && m.ytClient != nil && idx < len(m.ytPlaylists) {
			pl := m.ytPlaylists[idx]
			go func() { _ = m.ytClient.AddToPlaylist(pl.ID, v.ID) }()
			delete(m.playlistVidCache, pl.ID)
			m.setStatus(fmt.Sprintf("Added to '%s'", pl.Title), false)
		} else if idx < len(m.playlists) {
			pl := m.playlists[idx]
			_ = m.db.AddToPlaylist(pl.ID, v.ID)
			delete(m.playlistVidCache, fmt.Sprintf("local:%d", pl.ID))
			m.setStatus(fmt.Sprintf("Added to '%s'", pl.Name), false)
		}
		m.addOverlay = false
	}
	return m, nil
}

// overlayPlaylistCount is the total selectable rows in the add-to-playlist overlay.
func (m Model) overlayPlaylistCount() int {
	if m.ytPlLoaded && m.ytClient != nil {
		return len(m.ytPlaylists)
	}
	return len(m.playlists)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (m *Model) downloadAndPlay(v youtube.Video) {
	m.playAfterDownload[v.ID] = true
	m.startDownload(v, downloader.TypeVideo)
}

func (m *Model) downloadAndPlayAudio(v youtube.Video) {
	m.playAfterDownload[v.ID+":audio"] = true
	m.startDownload(v, downloader.TypeAudio)
}

func (m *Model) startDownload(v youtube.Video, dlType downloader.DownloadType) {
	m.downloader.Start(v, dlType)
	label := "video"
	if dlType == downloader.TypeAudio {
		label = "audio"
	}
	m.setStatus(fmt.Sprintf("Queued %s: %s", label, truncate(v.Title, 50)), false)
}

// ── Nvim-style scroll helpers ─────────────────────────────────────────────────

// vsMove moves cursor by delta and adjusts viewStart only when cursor leaves
// the visible window (scrolloff=0, like nvim default).
func vsMove(cursor, vs, n, delta, height int) (newCursor, newVS int) {
	c := cursor + delta
	if c < 0 {
		c = 0
	}
	if c >= n {
		c = n - 1
	}
	if c < vs {
		vs = c
	}
	if c >= vs+height {
		vs = c - height + 1
	}
	if vs < 0 {
		vs = 0
	}
	return c, vs
}

// vsPage advances one full page in direction (+1 down, -1 up), preserving the
// cursor's relative position within the viewport.
func vsPage(cursor, vs, n, direction, height int) (newCursor, newVS int) {
	relPos := cursor - vs
	newVS = vs + direction*height
	if newVS < 0 {
		newVS = 0
	}
	if newVS+height > n {
		newVS = n - height
		if newVS < 0 {
			newVS = 0
		}
	}
	newCursor = newVS + relPos
	if newCursor < 0 {
		newCursor = 0
	}
	if newCursor >= n {
		newCursor = n - 1
	}
	return newCursor, newVS
}

// updateSearchVS keeps searchVS in sync after searchCursor moves.
// Channels are always fully visible; VS only applies to the video sub-list.
func (m *Model) updateSearchVS(nCh, nVid int) {
	if m.searchCursor >= nCh && nVid > 0 {
		_, m.searchVS = vsMove(m.searchCursor-nCh, m.searchVS, nVid, 0, m.pageSize())
	} else {
		m.searchVS = 0
	}
}

// vsJump jumps to a target line and centers it in the viewport (like nvim gg/G).
func vsJump(target, n, height int) (newCursor, newVS int) {
	c := target
	if c < 0 {
		c = 0
	}
	if c >= n {
		c = n - 1
	}
	vs := c - height/2
	if vs < 0 {
		vs = 0
	}
	if vs+height > n {
		vs = n - height
		if vs < 0 {
			vs = 0
		}
	}
	return c, vs
}

func (m *Model) pageSize() int {
	// Use the maximum possible overhead (tab bar + 2-row status + section header
	// + filter bar = up to 6 rows) so we never skip rows between pages.
	// Worst case is a couple of overlap rows, which is always preferable to gaps.
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

// mergeVideos merges incoming into existing by video ID; incoming wins on conflict.
func mergeVideos(existing, incoming []youtube.Video) []youtube.Video {
	m := make(map[string]youtube.Video, len(existing)+len(incoming))
	for _, v := range existing {
		m[v.ID] = v
	}
	for _, v := range incoming {
		m[v.ID] = v
	}
	out := make([]youtube.Video, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// filterDownloaded removes videos that are already in the local library.
func filterDownloaded(videos []youtube.Video, local map[string]db.LocalVideo) []youtube.Video {
	out := videos[:0]
	for _, v := range videos {
		if _, ok := local[v.ID]; !ok {
			out = append(out, v)
		}
	}
	return out
}

// filterHidden removes videos the user has explicitly hidden from recommended.
func filterHidden(videos []youtube.Video, hidden map[string]bool) []youtube.Video {
	out := videos[:0]
	for _, v := range videos {
		if !hidden[v.ID] {
			out = append(out, v)
		}
	}
	return out
}

// filterBlacklisted removes videos whose channel is blacklisted.
// As a side effect it enriches name-only blacklist entries with the channel ID.
func filterBlacklisted(videos []youtube.Video, list []config.BlacklistedChannel, cfg *config.Config) []youtube.Video {
	out := videos[:0]
	for _, v := range videos {
		if bl, matched := matchBlacklisted(v, list); matched {
			if bl >= 0 && cfg.BlacklistedChannels[bl].ID == "" && v.ChannelID != "" {
				cfg.BlacklistedChannels[bl].ID = v.ChannelID
				go cfg.Save()
			}
			continue
		}
		out = append(out, v)
	}
	return out
}

// matchBlacklisted returns the index in list and true if the video's channel is blacklisted.
// Matches by ID first (exact), then by name (case-insensitive) for entries without an ID.
func matchBlacklisted(v youtube.Video, list []config.BlacklistedChannel) (int, bool) {
	for i, bl := range list {
		if bl.ID != "" && bl.ID == v.ChannelID {
			return i, true
		}
		if bl.ID == "" && strings.EqualFold(bl.Name, v.Channel) {
			return i, true
		}
	}
	return -1, false
}

// removeVideoByID returns a new slice with the given video ID removed.
func removeVideoByID(videos []youtube.Video, id string) []youtube.Video {
	out := make([]youtube.Video, 0, len(videos))
	for _, v := range videos {
		if v.ID != id {
			out = append(out, v)
		}
	}
	return out
}

func removeChannelVideos(videos []youtube.Video, channelID, channelName string) []youtube.Video {
	out := make([]youtube.Video, 0, len(videos))
	for _, v := range videos {
		matchID := channelID != "" && v.ChannelID == channelID
		matchName := channelName != "" && strings.EqualFold(v.Channel, channelName)
		if !matchID && !matchName {
			out = append(out, v)
		}
	}
	return out
}

func filterSubscribed(videos []youtube.Video, subscribed map[string]bool) []youtube.Video {
	if len(subscribed) == 0 {
		return videos
	}
	out := make([]youtube.Video, 0, len(videos))
	for _, v := range videos {
		if subscribed[v.ChannelID] {
			continue
		}
		if v.Channel != "" && subscribed["name:"+strings.ToLower(v.Channel)] {
			continue
		}
		out = append(out, v)
	}
	return out
}

// ytPlaylistSetChanged returns true if the two playlist lists differ.
func ytPlaylistSetChanged(a, b []youtube.YTPlaylist) bool {
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

// channelSetChanged returns true if the two lists differ in channel membership.
func channelSetChanged(a, b []youtube.Channel) bool {
	if len(a) != len(b) {
		return true
	}
	ids := make(map[string]bool, len(a))
	for _, ch := range a {
		ids[ch.ID] = true
	}
	for _, ch := range b {
		if !ids[ch.ID] {
			return true
		}
	}
	return false
}

func removeChannelByID(channels []youtube.Channel, id string) []youtube.Channel {
	out := make([]youtube.Channel, 0, len(channels))
	for _, ch := range channels {
		if ch.ID != id {
			out = append(out, ch)
		}
	}
	return out
}

// launchVideo starts playback of a local video, resuming from last position.
func (m *Model) launchVideo(lv db.LocalVideo) {
	if _, err := os.Stat(lv.FilePath); err != nil {
		m.setStatus("File not found: "+truncate(lv.Title, 50), true)
		return
	}
	startAt := time.Duration(lv.LastPositionMs) * time.Millisecond
	if err := m.playerBackend.Launch(lv.FilePath, startAt); err != nil {
		m.setStatus("play: "+err.Error(), true)
		return
	}
	m.playingVideoID = lv.ID
	_ = m.db.SetVideoStatus(lv.ID, db.StatusStarted)
	_ = m.db.AddHistory(lv.ID, "play", "")
	if lv2, err := m.db.LocalVideos(); err == nil {
		m.localVideos = lv2
		m.localVideoIDs = buildLocalIDMap(lv2)
	}
	label := truncate(lv.Title, 50)
	if startAt > 0 {
		m.setStatus(fmt.Sprintf("Playing (from %s): %s", formatDuration(startAt), label), false)
	} else {
		m.setStatus("Playing: "+label, false)
	}
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// addToWatchLater queues a video for YouTube Watch Later and updates status.
func (m *Model) addToWatchLater(v youtube.Video) {
	if m.ytClient == nil {
		m.setStatus("Watch Later: browser not configured (set 'browser' in config)", true)
		return
	}
	go func() { _ = m.ytClient.AddToPlaylist(ytWatchLaterID, v.ID) }()
	delete(m.playlistVidCache, ytWatchLaterID)
	m.setStatus("Added to Watch Later: "+truncate(v.Title, 50), false)
}

// subscribeCurrentChannel subscribes to the channel of the current video or selected channel.
func (m Model) subscribeCurrentChannel() (tea.Model, tea.Cmd) {
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
}

func (m Model) unsubscribeCurrentChannel() (tea.Model, tea.Cmd) {
	chID, chName := m.currentChannelInfo()
	return m.unsubscribeChannel(chID, chName)
}

func (m Model) unsubscribeChannel(chID, chName string) (tea.Model, tea.Cmd) {
	if m.ytClient == nil {
		m.setStatus("unsubscribe: configure 'browser' in config to enable", true)
		return m, nil
	}
	if chID == "" {
		m.setStatus("unsubscribe: no channel", true)
		return m, nil
	}
	return m, youtube.UnsubscribeFromChannel(m.ytClient, chID, chName)
}

// currentChannelInfo returns the channel ID and name for the currently focused item.
func (m Model) currentChannelInfo() (id, name string) {
	if m.activeTab == tabSubscriptions && m.subMode == subModeChannels && m.subChPane == 0 {
		sorted := m.sortedChannels()
		if m.subChCursor < len(sorted) {
			ch := sorted[m.subChCursor]
			return ch.ID, ch.Name
		}
	}
	if m.activeTab == tabSearch {
		if m.searchChSel != nil {
			return m.searchChSel.ID, m.searchChSel.Name
		}
		if m.searchCursor < len(m.searchChannels) {
			ch := m.searchChannels[m.searchCursor]
			return ch.ID, ch.Name
		}
	}
	if m.activeTab == tabHistory && m.histDetailVideoID == "" {
		if m.histCursor < len(m.histEntries) {
			e := m.histEntries[m.histCursor]
			return e.ChannelID, e.Channel
		}
	}
	if v, ok := m.currentVideo(); ok {
		return v.ChannelID, v.Channel
	}
	return "", ""
}

// copyCurrentURL copies the selected video's URL to the system clipboard.
func (m *Model) copyCurrentURL() {
	v, ok := m.currentVideo()
	if !ok {
		return
	}
	if err := clipboard.WriteAll(v.URL); err != nil {
		m.setStatus("clipboard: "+err.Error(), true)
	} else {
		m.setStatus("Copied: "+v.URL, false)
	}
}

// hideChannel immediately blacklists a channel and removes it from all in-memory feeds.
func (m *Model) hideChannel(channelID, channelName string) {
	if channelID == "" && channelName == "" {
		return
	}
	m.cfg.AddBlacklistedChannel(channelID, channelName)
	go m.cfg.Save()
	m.removeChannelFromFeeds(channelID, channelName)
	m.setStatus("Blacklisted channel: "+channelName, false)
}

// checkVideoHideAutoBlacklist auto-blacklists a channel when hidden-played >= cfg.ChannelStrikes.
func (m *Model) checkVideoHideAutoBlacklist(channelID, channelName string) {
	if channelID == "" {
		return
	}
	hidden, played, err := m.db.ChannelHideStats(channelID)
	if err != nil || hidden-played < m.cfg.ChannelStrikes {
		return
	}
	m.hideChannel(channelID, channelName)
}

// removeChannelFromFeeds strips a channel's videos from all in-memory video lists.
func (m *Model) removeChannelFromFeeds(channelID, channelName string) {
	m.recVideos = removeChannelVideos(m.recVideos, channelID, channelName)
	m.recCursor, m.recVS = vsMove(clamp(m.recCursor, len(m.recVideos)), m.recVS, len(m.recVideos), 0, m.pageSize())
	m.subVideos = removeChannelVideos(m.subVideos, channelID, channelName)
	m.subCursor, m.subVS = vsMove(clamp(m.subCursor, len(m.subVideos)), m.subVS, len(m.subVideos), 0, m.pageSize())
	m.subChVideos = removeChannelVideos(m.subChVideos, channelID, channelName)
	m.subChVidCursor, m.subChVidVS = vsMove(clamp(m.subChVidCursor, len(m.subChVideos)), m.subChVidVS, len(m.subChVideos), 0, m.pageSize())
}
