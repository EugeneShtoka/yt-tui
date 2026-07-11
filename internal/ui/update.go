package ui

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if kittyCapable() && m.vidDetailThumbB64 != "" {
			thumbW, thumbH := m.thumbDimensions()
			tabBarH := lipgloss.Height(m.renderTabBar())
			thumbRow := tabBarH + 2
			thumbCol := m.width - vidDetailPanelW + 2
			m.vidDetailKittyOverlay = kittyImageOverlay(m.vidDetailThumbB64, thumbRow, thumbCol, thumbW, thumbH)
		}
		return m, nil

	case positionTickMsg:
		if pos, ok := m.playerBackend.Position(); ok && m.playingVideoID != "" {
			ms := pos.Milliseconds()
			// Local files downloaded with --sponsorblock-remove have compressed timelines.
			// Convert file position → original timeline before saving so streaming and
			// local playback share the same position space.
			if _, isLocal := m.localVideoIDs[m.playingVideoID]; isLocal && len(m.playingSBSegments) > 0 {
				ms = adjustedToOriginalMs(ms, m.playingSBSegments)
			}
			m.videoPositions[m.playingVideoID] = ms
			_ = m.db.SaveVideoPosition(m.playingVideoID, ms)
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
			_ = m.db.LogActivity(db.ActivityEntry{
				Type: "subscribe", IsLocal: false,
				ChannelID: msg.ChannelID, ChannelName: msg.ChannelName,
			})
		}
		return m, nil

	case youtube.UnsubscribeMsg:
		if msg.Err != nil {
			m.setStatus("unsubscribe failed: "+msg.Err.Error(), true)
		} else {
			m.setStatus("Unsubscribed from: "+msg.ChannelName, false)
			delete(m.subscribedChannelIDs, msg.ChannelID)
			delete(m.subscribedChannelIDs, "name:"+strings.ToLower(msg.ChannelName))
			m.subChannels = removeChannelByID(m.subChannels, msg.ChannelID)
			m.subVideos = removeChannelVideos(m.subVideos, msg.ChannelID, msg.ChannelName)
			m.subCursor, m.subVS = vsMove(clamp(m.subCursor, len(m.subVideos)), m.subVS, len(m.subVideos), 0, m.pageSize(), false)
			m.subChVideos = removeChannelVideos(m.subChVideos, msg.ChannelID, msg.ChannelName)
			m.subChVidCursor, m.subChVidVS = vsMove(clamp(m.subChVidCursor, len(m.subChVideos)), m.subChVidVS, len(m.subChVideos), 0, m.pageSize(), false)
			go m.db.DeleteChannelVideos(msg.ChannelID)
		}
		return m, nil

	case youtube.RemoveYTPlaylistVideoMsg:
		if msg.Err != nil {
			m.setStatus("remove from playlist: "+msg.Err.Error(), true)
		}
		return m, nil

	case youtube.CreatePlaylistMsg:
		if msg.Err != nil {
			m.addAfterCreate = false
			m.setStatus("create playlist: "+msg.Err.Error(), true)
		} else {
			m.ytPlaylists = append(m.ytPlaylists, youtube.YTPlaylist{ID: msg.ID, Title: msg.Name})
			_ = m.db.LogActivity(db.ActivityEntry{
				Type: "create_playlist", IsLocal: false,
				PlaylistID: msg.ID, PlaylistName: msg.Name,
			})
			if m.addAfterCreate {
				m.addAfterCreate = false
				v := m.addVideo
				plID := msg.ID
				go func() { _ = m.ytClient.AddToPlaylist(plID, v.ID) }()
				delete(m.playlistVidCache, plID)
				_ = m.db.LogActivity(db.ActivityEntry{
					Type: "add_to_playlist", IsLocal: false,
					PlaylistID: msg.ID, PlaylistName: msg.Name,
					VideoID: v.ID, VideoTitle: v.Title,
				})
				m.setStatus(fmt.Sprintf("Created '%s' and added video", msg.Name), false)
			} else {
				m.setStatus("Created playlist: "+msg.Name, false)
			}
		}
		return m, nil

	case youtube.VideoDetailsMsg:
		if msg.Err != nil {
			m.vidDetailLoading = false
			m.vidDetailOverlay = false
			m.pendingDirectOverlay = ""
			m.setStatus("video details: "+msg.Err.Error(), true)
			return m, nil
		}
		details := msg.Details
		m.vidDetailVideo = &details
		m.vidDetailLinks = nil
		m.vidDetailDescLines = wordWrap(details.Description, vidDetailPanelW-2)
		_ = m.db.SaveVideoDetailsCache(details.Video.ID, details.Description, details.ThumbnailURL, details.Subscribers)

		// Process chapters: filter SponsorBlock chapters, adjust timecodes.
		if len(details.Chapters) > 0 {
			displayChapters, sbSegs := processChapters(details.Chapters)
			m.vidDetailChapters = &displayChapters
			_ = m.db.SaveVideoChapters(details.Video.ID, displayChapters)
			if len(sbSegs) > 0 {
				_ = m.db.SaveVideoSBSegments(details.Video.ID, sbSegs)
			}
		} else {
			m.vidDetailChapters = nil
		}

		// Handle direct overlay open (chapters/links without info panel).
		if m.pendingDirectOverlay != "" {
			overlay := m.pendingDirectOverlay
			m.pendingDirectOverlay = ""
			switch overlay {
			case "chapters":
				if m.vidDetailChapters != nil && len(*m.vidDetailChapters) > 0 {
					m.chapterOverlayItems = *m.vidDetailChapters
					m.chapterOverlaySel = 0
					m.chapterOverlay = true
				} else {
					m.setStatus("no chapters available", false)
				}
			case "links":
				urls := extractLinks(details.Description)
				m.vidDetailLinks = &urls
				_ = m.db.SaveVideoLinks(details.Video.ID, urls)
				if len(urls) == 0 {
					m.setStatus("no links in description", false)
				} else {
					m.linkOverlayURLs = urls
					m.linkOverlaySel = 0
					m.linkOverlay = true
				}
			}
			m.vidDetailLoading = false
			return m, nil
		}

		if details.ThumbnailURL != "" {
			// Keep loading=true until thumbnail arrives so panel renders once with image.
			return m, loadThumbnailCmd(details.ThumbnailURL)
		}
		m.vidDetailLoading = false
		return m, nil

	case thumbnailLoadedMsg:
		m.vidDetailLoading = false
		m.vidDetailThumb = msg.img
		if msg.img != nil {
			if kittyCapable() {
				m.vidDetailThumbB64 = encodeThumbB64(msg.img)
				thumbW, thumbH := m.thumbDimensions()
				tabBarH := lipgloss.Height(m.renderTabBar())
				thumbRow := tabBarH + 2
				thumbCol := m.width - vidDetailPanelW + 2
				m.vidDetailKittyOverlay = kittyImageOverlay(m.vidDetailThumbB64, thumbRow, thumbCol, thumbW, thumbH)
			} else {
				thumbW, thumbH := m.thumbDimensions()
				m.vidDetailThumbRendered = renderThumbnail(msg.img, thumbW, thumbH)
			}
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
			// Preserve locally-added channels (those in current list but absent from YT result).
			ytIDs := make(map[string]bool, len(msg.Channels))
			for _, ch := range msg.Channels {
				ytIDs[ch.ID] = true
			}
			var localOnly []youtube.Channel
			for _, ch := range m.subChannels {
				if !ytIDs[ch.ID] {
					localOnly = append(localOnly, ch)
				}
			}
			merged := append(msg.Channels, localOnly...)

			// Update channel list only when membership changed (added or removed).
			if channelSetChanged(m.subChannels, merged) {
				// Preserve user-set alias and tags from the current in-memory list.
				existing := make(map[string]youtube.Channel, len(m.subChannels))
				for _, ch := range m.subChannels {
					existing[ch.ID] = ch
				}
				m.subChannels = merged
				for i, ch := range m.subChannels {
					if old, ok := existing[ch.ID]; ok {
						m.subChannels[i].Alias = old.Alias
						m.subChannels[i].Tags = old.Tags
					}
				}
			}
			for _, ch := range m.subChannels {
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
			}(m.subChannels, m.recVideos)
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
		if m.cmdMode {
			var cmd tea.Cmd
			m.cmdInput, cmd = m.cmdInput.Update(msg)
			return m, cmd
		}
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

	case cmdErrMsg:
		m.setStatus("editor: "+msg.err.Error(), true)
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
	switch {
	case key.Matches(msg, m.keys.Escape):
		m.localFilterFocused = false
		m.localFilterInput.SetValue("")
		m.localFilter = ""
		m.localFilterCursor = 0
		return m, nil
	case key.Matches(msg, m.keys.DrillDown):
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

var cmdAllCompletions = []string{
	"config",
	"clear cache",
	"clear history",
	"clear downloads",
	"clear recommended",
	"tab recommended",
	"tab subscriptions",
	"tab channels",
	"tab playlists",
	"tab search",
	"tab downloading",
	"tab local",
	"tab history",
	"tab activity",
}

// cmdCompletionsFor returns completions one word at a time.
// Before a space: completes the command name only (e.g. "t" → "tab ").
// After a space: completes the subcommand (e.g. "tab r" → "tab recommended").
func cmdCompletionsFor(input string) []string {
	spaceIdx := strings.Index(input, " ")
	if spaceIdx < 0 {
		// First-word completion: return unique command words (+ space if they have subcommands).
		seen := map[string]bool{}
		var out []string
		for _, c := range cmdAllCompletions {
			parts := strings.SplitN(c, " ", 2)
			fw := parts[0]
			if !strings.HasPrefix(fw, input) || seen[fw] {
				continue
			}
			seen[fw] = true
			if len(parts) > 1 {
				out = append(out, fw+" ")
			} else {
				out = append(out, fw)
			}
		}
		return out
	}
	// Second-word completion: match full commands against first word + subcommand prefix.
	firstWord := input[:spaceIdx]
	subPrefix := input[spaceIdx+1:]
	var out []string
	for _, c := range cmdAllCompletions {
		parts := strings.SplitN(c, " ", 2)
		if parts[0] == firstWord && len(parts) > 1 && strings.HasPrefix(parts[1], subPrefix) {
			out = append(out, c)
		}
	}
	return out
}

func (m Model) handleCmdInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Escape) || msg.String() == "ctrl+c":
		m.cmdMode = false
		m.cmdCompletions = nil
		m.cmdLastTabValue = ""
		m.cmdInput.SetValue("")
		m.cmdInput.Blur()
		return m, nil
	case key.Matches(msg, m.keys.DrillDown):
		val := strings.TrimSpace(m.cmdInput.Value())
		m.cmdMode = false
		m.cmdCompletions = nil
		m.cmdLastTabValue = ""
		m.cmdInput.SetValue("")
		m.cmdInput.Blur()
		return m.execCommand(val)
	case msg.String() == "tab":
		input := m.cmdInput.Value()
		// Recompute if input changed since last Tab, or no completions yet.
		if len(m.cmdCompletions) == 0 || input != m.cmdLastTabValue {
			m.cmdCompletions = cmdCompletionsFor(input)
			m.cmdCompIdx = 0
		} else {
			m.cmdCompIdx = (m.cmdCompIdx + 1) % len(m.cmdCompletions)
		}
		if len(m.cmdCompletions) > 0 {
			newVal := m.cmdCompletions[m.cmdCompIdx]
			m.cmdInput.SetValue(newVal)
			m.cmdInput.CursorEnd()
			// If we just completed a word boundary (trailing space), next Tab
			// should explore subcommands fresh rather than continue this cycle.
			if strings.HasSuffix(newVal, " ") {
				m.cmdLastTabValue = ""
			} else {
				m.cmdLastTabValue = newVal
			}
		}
		return m, nil
	default:
		m.cmdLastTabValue = ""
		m.cmdCompletions = nil
		var cmd tea.Cmd
		m.cmdInput, cmd = m.cmdInput.Update(msg)
		return m, cmd
	}
}

func (m Model) execCommand(input string) (Model, tea.Cmd) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return m, nil
	}
	switch parts[0] {
	case "config":
		return m.openConfigInEditor()
	case "clear":
		if len(parts) < 2 {
			m.setStatus("usage: clear <cache|history|downloads|recommended>", true)
			return m, nil
		}
		return m.execClear(parts[1])
	case "tab":
		if len(parts) < 2 {
			m.setStatus("usage: tab <name>", true)
			return m, nil
		}
		name := parts[1]
		id, ok := tabIDByName[name]
		if !ok {
			m.setStatus("unknown tab: "+name, true)
			return m, nil
		}
		for _, t := range m.tabs {
			if t == id {
				m.activeTab = id
				if id == tabSearch {
					m.searchFocused = true
					m.searchInput.Focus()
					return m, textinput.Blink
				}
				return m, nil
			}
		}
		m.setStatus("tab not in layout: "+name, true)
		return m, nil
	default:
		m.setStatus("unknown command: "+parts[0], true)
		return m, nil
	}
}

func (m Model) execClear(what string) (Model, tea.Cmd) {
	switch what {
	case "cache":
		if err := m.db.ClearVideoDetailsCache(); err != nil {
			m.setStatus("clear cache: "+err.Error(), true)
		} else {
			m.setStatus("video details cache cleared", false)
		}
	case "history":
		if err := m.db.ClearHistory(); err != nil {
			m.setStatus("clear history: "+err.Error(), true)
		} else {
			m.history.clear()
			m.streamedVideoIDs = make(map[string]bool)
			m.setStatus("history cleared", false)
		}
	case "downloads":
		paths, err := m.db.ClearDownloads()
		if err != nil {
			m.setStatus("clear downloads: "+err.Error(), true)
		} else {
			go func() {
				for _, p := range paths {
					_ = os.Remove(p)
				}
			}()
			m.localVideos = nil
			m.localVideoIDs = make(map[string]db.LocalVideo)
			m.localCursor = 0
			m.setStatus(fmt.Sprintf("cleared %d downloads", len(paths)), false)
		}
	case "recommended":
		if err := m.db.ClearRecommended(); err != nil {
			m.setStatus("clear recommended: "+err.Error(), true)
		} else {
			m.recVideos = nil
			m.recCursor = 0
			m.recLoaded = false
			m.setStatus("recommended cleared", false)
		}
	default:
		m.setStatus("unknown: clear "+what+" (cache|history|downloads|recommended)", true)
	}
	return m, nil
}

func (m Model) openConfigInEditor() (Model, tea.Cmd) {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command(editor, m.cfg.ConfigFile)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return cmdErrMsg{err}
		}
		return nil
	})
}

type cmdErrMsg struct{ err error }

func tabName(id int) string {
	if id >= 0 && id < numTabIDs {
		return tabMeta[id].name
	}
	return "unknown"
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	debug.Log("key=%q tab=%s localFilterFocused=%v localFilter=%q searchFocused=%v createMode=%v addOverlay=%v showHelp=%v pendingChord=%q gPending=%v numPrefix=%q",
		msg.String(), tabName(m.activeTab), m.localFilterFocused, m.localFilter, m.searchFocused,
		m.createMode, m.addOverlay, m.showHelp, m.pendingChord, m.gPending, m.numPrefix)

	if m.cmdMode {
		debug.Log("→ handleCmdInput (cmdMode=true)")
		return m.handleCmdInput(msg)
	}
	if m.localFilterFocused {
		debug.Log("→ handleLocalFilter (localFilterFocused=true)")
		return m.handleLocalFilter(msg)
	}
	if m.searchFocused {
		debug.Log("→ handleSearchInput (searchFocused=true)")
		return m.handleSearchInput(msg)
	}
	if m.createTypeMode {
		debug.Log("→ handleCreateTypeInput (createTypeMode=true)")
		return m.handleCreateTypeInput(msg)
	}
	if m.createMode {
		debug.Log("→ handleCreateInput (createMode=true)")
		return m.handleCreateInput(msg)
	}
	if m.activeTab == tabChannels && m.subChEditMode != 0 {
		debug.Log("→ handleChannelEditInput (subChEditMode=%d)", m.subChEditMode)
		return m.handleChannelEditInput(msg)
	}
	if m.addOverlay {
		debug.Log("→ handleAddOverlay (addOverlay=true)")
		return m.handleAddOverlay(msg)
	}
	if m.linkOverlay {
		debug.Log("→ handleLinkOverlay (linkOverlay=true)")
		return m.handleLinkOverlay(msg)
	}
	if m.chapterOverlay {
		debug.Log("→ handleChapterOverlay (chapterOverlay=true)")
		return m.handleChapterOverlay(msg)
	}
	if m.vidDetailOverlay {
		debug.Log("→ handleVideoDetailKey (vidDetailOverlay=true)")
		return m.handleVideoDetailKey(msg)
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

	// ── GotoLine / GotoBottom ────────────────────────────────────────────
	// GotoLine fires first when a number prefix is pending; GotoBottom always goes to last.
	// When both keys are the same, the number-prefix check disambiguates them.
	if m.numPrefix != "" && key.Matches(msg, m.keys.GotoLine) {
		n := m.parseNumPrefix()
		m.numPrefix = ""
		m.gPending = false
		m.jumpToLine(n - 1)
		return m, nil
	}
	if key.Matches(msg, m.keys.GotoBottom) {
		m.numPrefix = ""
		m.gPending = false
		m.jumpToLast()
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

	// ── Command mode trigger ──────────────────────────────────────────────
	if s == ":" {
		m.cmdMode = true
		m.cmdInput.SetValue("")
		m.cmdInput.Focus()
		return m, textinput.Blink
	}

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
	if s == kb.Subscribe && m.contextSupportsSubscribe() {
		m.pendingChord = kb.Subscribe
		m.chordBuffer = ""
		debug.Log("→ subscribe chord pending")
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
	case key.Matches(msg, m.keys.VideoInfo):
		if v, ok := m.currentVideo(); ok && v.URL != "" {
			return m, (&m).openVideoDetail(v)
		}
		return m, nil
	case key.Matches(msg, m.keys.OpenLinks):
		if v, ok := m.currentVideo(); ok {
			return m.openLinksForVideo(v)
		}
	case key.Matches(msg, m.keys.OpenChapters):
		if v, ok := m.currentVideo(); ok {
			return m.openChaptersForVideo(v)
		}
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
	case tabChannels:
		return m.updateSubChannels(msg)
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
	case tabActivity:
		return m.updateActivity(msg)
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
		activeSorted := m.sortedChannels()
		if m.subChTagsMode && m.subChPane == 1 {
			activeSorted = m.sortedChannelsInTag(m.subChTagSel)
		}
		var selID string
		if m.subChCursor < len(activeSorted) {
			selID = activeSorted[m.subChCursor].ID
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
		case "tags":
			m.subChSort = subChSortTags
		}
		afterSort := m.sortedChannels()
		if m.subChTagsMode && m.subChPane == 1 {
			afterSort = m.sortedChannelsInTag(m.subChTagSel)
		}
		if selID != "" {
			for i, ch := range afterSort {
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
		m.subSort = vidSort
		sortVideos(m.subVideos, vidSort)
	case tabChannels:
		if m.subChTagsMode && m.subChPane == 1 {
			m.subChTagSort = vidSort
		} else if !m.subChTagsMode && m.subChPane == 1 {
			m.subChVidSort = vidSort
			sortVideos(m.subChVideos, vidSort)
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
	case tabChannels:
		if !m.subChLoading {
			m.subChLoading = true
			if m.subChLoaded {
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
	case tabActivity:
		m.loadActivity()
	}
	return nil
}

func (m *Model) loadHistory() tea.Cmd {
	m.history.load(m.db, func(s string) { m.setStatus(s, true) })
	return nil
}

func (m *Model) loadActivity() {
	m.activity.load(m.db, func(s string) { m.setStatus(s, true) })
}

func (m Model) updateActivity(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if e, ok := m.activity.update(msg, m.keys, m.pageSize(), m.cfg.CircularNav); ok {
		return m.navigateToActivity(e)
	}
	return m, nil
}

func (m Model) navigateToActivity(e db.ActivityEntry) (tea.Model, tea.Cmd) {
	switch e.Type {
	case "subscribe":
		m.activeTab = tabChannels
		channels := m.sortedChannels()
		for i, ch := range channels {
			if ch.ID == e.ChannelID {
				m.subChCursor = i
				cmd := m.openChannelVideos(ch, false)
				return m, cmd
			}
		}
		m.setStatus("No longer subscribed to: "+e.ChannelName, true)
		return m, m.onTabActivated()
	case "create_playlist", "add_to_playlist":
		m.activeTab = tabPlaylists
		if e.PlaylistLocalID != 0 {
			offset := 0
			if m.ytPlLoaded {
				offset = len(m.ytPlaylists)
			}
			for i, pl := range m.playlists {
				if pl.ID == e.PlaylistLocalID {
					m.playlistCursor = offset + i
					m.playlistPane = 1
					cmd := m.fetchCurrentPlaylistVideos()
					return m, cmd
				}
			}
		} else if e.PlaylistID != "" && m.ytPlLoaded {
			for i, pl := range m.ytPlaylists {
				if pl.ID == e.PlaylistID {
					m.playlistCursor = i
					m.playlistPane = 1
					cmd := m.fetchCurrentPlaylistVideos()
					return m, cmd
				}
			}
		}
		m.setStatus("Playlist no longer exists: "+e.PlaylistName, true)
		return m, m.onTabActivated()
	}
	return m, nil
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
		m.subChLoading = true
		return youtube.FetchSubscribedChannels(m.cfg)
	case tabChannels:
		if !m.subChTagsMode && m.subChPane == 1 {
			return m.fetchChannelLatest(m.subChActiveID)
		}
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
	case tabActivity:
		m.loadActivity()
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
		return m.forceRefreshAllChannels()
	case tabChannels:
		if !m.subChTagsMode && m.subChPane == 1 {
			return youtube.FetchChannelVideos(m.cfg, m.channelURL(m.subChActiveID), m.subChActiveID, "subscriptions")
		}
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
		m.recCursor, m.recVS = vsMove(m.recCursor, m.recVS, n, -1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.Down):
		m.recCursor, m.recVS = vsMove(m.recCursor, m.recVS, n, +1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.PageUp):
		m.recCursor, m.recVS = vsPage(m.recCursor, m.recVS, n, -1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.PageDown):
		m.recCursor, m.recVS = vsPage(m.recCursor, m.recVS, n, +1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.HideVideo):
		if v, ok := m.currentVideo(); ok {
			_ = m.db.HideRecVideo(v.ID)
			m.recHidden[v.ID] = true
			m.recVideos = removeVideoByID(m.recVideos, v.ID)
			m.recCursor, m.recVS = vsMove(clamp(m.recCursor, len(m.recVideos)), m.recVS, len(m.recVideos), 0, m.pageSize(), false)
			m.setStatus("Hidden: "+truncate(v.Title, 50), false)
			m.checkVideoHideAutoBlacklist(v.ChannelID, v.Channel)
		}
	case key.Matches(msg, m.keys.HideChannel):
		debug.Log("recommended: hide channel matched")
		if v, ok := m.currentVideo(); ok {
			m.hideChannel(v.ChannelID, v.Channel)
		}
	}
	m.handleVideoAction(msg)
	return m, nil
}

// ── Subscriptions ─────────────────────────────────────────────────────────────

func (m Model) updateSubscriptions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.updateSubAll(msg)
}

func (m Model) updateSubAll(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.subVideos)
	switch {
	case key.Matches(msg, m.keys.Up):
		m.subCursor, m.subVS = vsMove(m.subCursor, m.subVS, n, -1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.Down):
		m.subCursor, m.subVS = vsMove(m.subCursor, m.subVS, n, +1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.PageUp):
		m.subCursor, m.subVS = vsPage(m.subCursor, m.subVS, n, -1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.PageDown):
		m.subCursor, m.subVS = vsPage(m.subCursor, m.subVS, n, +1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.Unsubscribe):
		return m.unsubscribeCurrentChannel()
	}
	m.handleVideoAction(msg)
	return m, nil
}

func (m Model) updateSubChannels(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// ToggleMode: switch between flat channel list and tags-grouped view.
	if key.Matches(msg, m.keys.ToggleMode) {
		m.subChTagsMode = !m.subChTagsMode
		m.subChPane = 0
		m.subChTagCursor = 0
		m.subChTagVS = 0
		return m, nil
	}

	if m.subChTagsMode {
		return m.updateSubChannelsTags(msg)
	}

	// ── Flat mode ─────────────────────────────────────────────────────────────
	if m.subChPane == 0 {
		sorted := m.sortedChannels()
		n := len(sorted)
		switch {
		case key.Matches(msg, m.keys.Up):
			m.subChCursor, m.subChVS = vsMove(m.subChCursor, m.subChVS, n, -1, m.pageSize(), m.cfg.CircularNav)
		case key.Matches(msg, m.keys.Down):
			m.subChCursor, m.subChVS = vsMove(m.subChCursor, m.subChVS, n, +1, m.pageSize(), m.cfg.CircularNav)
		case key.Matches(msg, m.keys.DrillDown), key.Matches(msg, m.keys.Right):
			if m.subChCursor < n {
				return m, m.openChannelVideos(sorted[m.subChCursor], false)
			}
		case key.Matches(msg, m.keys.RenameChannel):
			if m.subChCursor < n {
				ch := sorted[m.subChCursor]
				m.subChEditInput.SetValue(ch.Alias)
				m.subChEditInput.Placeholder = "alias (empty to clear)…"
				m.subChEditInput.Focus()
				m.subChEditMode = 1
			}
		case key.Matches(msg, m.keys.TagChannel):
			if m.subChCursor < n {
				ch := sorted[m.subChCursor]
				m.subChEditInput.SetValue(strings.Join(ch.Tags, ", "))
				m.subChEditInput.Placeholder = "comma-separated tags…"
				m.subChEditInput.Focus()
				m.subChEditMode = 2
			}
		case key.Matches(msg, m.keys.Unsubscribe):
			if m.subChCursor < n {
				ch := sorted[m.subChCursor]
				return m.unsubscribeChannel(ch.ID, ch.Name)
			}
		}
		return m, nil
	}

	// ── Flat mode: video pane ─────────────────────────────────────────────────
	return m.updateSubChVideoPane(msg, 0)
}

// updateSubChannelsTags handles key events for the tags-grouped view.
func (m Model) updateSubChannelsTags(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.subChPane {
	case 0: // tag list
		items := m.tagListItems()
		n := len(items)
		switch {
		case key.Matches(msg, m.keys.Up):
			m.subChTagCursor, m.subChTagVS = vsMove(m.subChTagCursor, m.subChTagVS, n, -1, m.pageSize(), m.cfg.CircularNav)
		case key.Matches(msg, m.keys.Down):
			m.subChTagCursor, m.subChTagVS = vsMove(m.subChTagCursor, m.subChTagVS, n, +1, m.pageSize(), m.cfg.CircularNav)
		case key.Matches(msg, m.keys.DrillDown), key.Matches(msg, m.keys.Right):
			if m.subChTagCursor < n {
				m.subChTagSel = items[m.subChTagCursor]
				m.subChCursor = 0
				m.subChVS = 0
				m.subChPane = 1
			}
		}

	case 1: // video list for selected tag (reuses subChCursor/subChVS)
		vids := m.tagVideos()
		n := len(vids)
		switch {
		case key.Matches(msg, m.keys.Left), key.Matches(msg, m.keys.Escape):
			m.subChPane = 0
			m.subChCursor = 0
			m.subChVS = 0
		case key.Matches(msg, m.keys.Up):
			m.subChCursor, m.subChVS = vsMove(m.subChCursor, m.subChVS, n, -1, m.pageSize(), m.cfg.CircularNav)
		case key.Matches(msg, m.keys.Down):
			m.subChCursor, m.subChVS = vsMove(m.subChCursor, m.subChVS, n, +1, m.pageSize(), m.cfg.CircularNav)
		case key.Matches(msg, m.keys.PageUp):
			m.subChCursor, m.subChVS = vsPage(m.subChCursor, m.subChVS, n, -1, m.pageSize(), m.cfg.CircularNav)
		case key.Matches(msg, m.keys.PageDown):
			m.subChCursor, m.subChVS = vsPage(m.subChCursor, m.subChVS, n, +1, m.pageSize(), m.cfg.CircularNav)
		case key.Matches(msg, m.keys.VideoInfo):
			if v, ok := m.currentVideo(); ok && v.URL != "" {
				return m, (&m).openVideoDetail(v)
			}
		}
		m.handleVideoAction(msg)
	}
	return m, nil
}

// openChannelVideos loads videos for ch, setting subChPane to nextPane+1.
// inTagsMode=true means we're going to pane 2; false means pane 1.
func (m *Model) openChannelVideos(ch youtube.Channel, inTagsMode bool) tea.Cmd {
	targetPane := 1
	if inTagsMode {
		targetPane = 2
	}
	m.subChVidCursor = 0
	m.subChPane = targetPane
	if ch.ID == m.subChActiveID && len(m.subChVideos) > 0 {
		m.subChVidLoading = false
		m.subChVidRefreshing = true
		return youtube.FetchChannelLatestN(m.cfg, ch.URL, ch.ID, m.cfg.ChannelLatestCount)
	}
	m.subChActiveID = ch.ID
	if cached, err := m.db.GetChannelVideos(ch.ID); err == nil && len(cached) > 0 {
		m.subChVideos = cached
		m.subChVidLoading = false
		m.subChVidRefreshing = true
		return youtube.FetchChannelLatestN(m.cfg, ch.URL, ch.ID, m.cfg.ChannelLatestCount)
	}
	m.subChVideos = nil
	m.subChVidLoading = true
	m.subChVidRefreshing = false
	return youtube.FetchChannelVideos(m.cfg, ch.URL, ch.ID, "subscriptions")
}

// updateSubChVideoPane handles key events for the channel video pane.
// backPane is the pane to return to on Back (0 for flat, 1 for tags mode).
func (m Model) updateSubChVideoPane(msg tea.KeyMsg, backPane int) (tea.Model, tea.Cmd) {
	n := len(m.subChVideos)
	switch {
	case key.Matches(msg, m.keys.Left), key.Matches(msg, m.keys.Escape):
		m.subChPane = backPane
	case key.Matches(msg, m.keys.Up):
		m.subChVidCursor, m.subChVidVS = vsMove(m.subChVidCursor, m.subChVidVS, n, -1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.Down):
		m.subChVidCursor, m.subChVidVS = vsMove(m.subChVidCursor, m.subChVidVS, n, +1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.PageUp):
		m.subChVidCursor, m.subChVidVS = vsPage(m.subChVidCursor, m.subChVidVS, n, -1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.PageDown):
		m.subChVidCursor, m.subChVidVS = vsPage(m.subChVidCursor, m.subChVidVS, n, +1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.Unsubscribe):
		return m.unsubscribeCurrentChannel()
	}
	m.handleVideoAction(msg)
	return m, nil
}

// editTargetChannelID returns the channel ID of the channel being edited (flat mode only).
func (m Model) editTargetChannelID() string {
	sorted := m.sortedChannels()
	if m.subChCursor < len(sorted) {
		return sorted[m.subChCursor].ID
	}
	return ""
}

// parseTags splits a comma-separated tag string into a trimmed, non-empty slice.
func parseTags(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
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
			m.playlistCursor, m.playlistVS = vsMove(m.playlistCursor, m.playlistVS, n, -1, m.pageSize(), m.cfg.CircularNav)
		case key.Matches(msg, m.keys.Down):
			m.playlistCursor, m.playlistVS = vsMove(m.playlistCursor, m.playlistVS, n, +1, m.pageSize(), m.cfg.CircularNav)
		case key.Matches(msg, m.keys.DrillDown), key.Matches(msg, m.keys.Right):
			if m.playlistCursor < n {
				cmd := m.loadCurrentPlaylistVideos()
				m.playlistPane = 1
				m.playlistVidCursor = 0
				return m, cmd
			}
		case key.Matches(msg, m.keys.NewList):
			if m.ytClient != nil {
				m.createTypeMode = true
				m.createTypeSel = 0
			} else {
				m.createModeYT = false
				m.createInput.SetValue("")
				m.createInput.Placeholder = "Playlist name…"
				m.createInput.Focus()
				m.createMode = true
				return m, textinput.Blink
			}
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
			} else {
				localIdx := idx
				if m.ytPlLoaded {
					localIdx -= len(m.ytPlaylists)
				}
				if localIdx >= 0 && localIdx < len(m.playlists) {
					pl := m.playlists[localIdx]
					_ = m.db.DeletePlaylist(pl.ID)
					delete(m.playlistVidCache, fmt.Sprintf("local:%d", pl.ID))
					playlists, _ := m.db.Playlists()
					m.playlists = playlists
				}
			}
			m.playlistCursor, m.playlistVS = vsMove(clamp(m.playlistCursor, m.playlistCount()), m.playlistVS, m.playlistCount(), 0, m.pageSize(), false)
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
		m.playlistVidCursor, m.playlistVidVS = vsMove(m.playlistVidCursor, m.playlistVidVS, n, -1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.Down):
		m.playlistVidCursor, m.playlistVidVS = vsMove(m.playlistVidCursor, m.playlistVidVS, n, +1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.PageUp):
		m.playlistVidCursor, m.playlistVidVS = vsPage(m.playlistVidCursor, m.playlistVidVS, n, -1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.PageDown):
		m.playlistVidCursor, m.playlistVidVS = vsPage(m.playlistVidCursor, m.playlistVidVS, n, +1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.DrillDown):
		if v, ok := m.currentVideo(); ok {
			m.downloadAndPlay(v)
		}
	case key.Matches(msg, m.keys.Delete):
		if m.playlistVidCursor < n {
			vid := vids[m.playlistVidCursor]
			var cmd tea.Cmd
			if m.selectedPlaylistIsYT() {
				cmd = youtube.RemoveYTPlaylistVideo(m.ytClient, plKey, vid.ID)
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
			m.playlistVidCursor, m.playlistVidVS = vsMove(clamp(m.playlistVidCursor, len(updated)), m.playlistVidVS, len(updated), 0, m.pageSize(), false)
			return m, cmd
		}
	case key.Matches(msg, m.keys.Download):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeVideo)
		}
	case key.Matches(msg, m.keys.DownloadAudio):
		if v, ok := m.currentVideo(); ok {
			m.startDownload(v, downloader.TypeAudio)
		}

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
			m.searchChVidCursor, m.searchChVidVS = vsMove(m.searchChVidCursor, m.searchChVidVS, n, -1, m.pageSize(), m.cfg.CircularNav)
		case key.Matches(msg, m.keys.Down):
			m.searchChVidCursor, m.searchChVidVS = vsMove(m.searchChVidCursor, m.searchChVidVS, n, +1, m.pageSize(), m.cfg.CircularNav)
		case key.Matches(msg, m.keys.PageUp):
			m.searchChVidCursor, m.searchChVidVS = vsPage(m.searchChVidCursor, m.searchChVidVS, n, -1, m.pageSize(), m.cfg.CircularNav)
		case key.Matches(msg, m.keys.PageDown):
			m.searchChVidCursor, m.searchChVidVS = vsPage(m.searchChVidCursor, m.searchChVidVS, n, +1, m.pageSize(), m.cfg.CircularNav)
		case key.Matches(msg, m.keys.Play):
			if v, ok := m.currentVideo(); ok {
				m.playVideo(v)
			}
		case key.Matches(msg, m.keys.PlayAudio):
			if v, ok := m.currentVideo(); ok {
				m.playAudio(v)
			}
		case key.Matches(msg, m.keys.Download):
			if v, ok := m.currentVideo(); ok {
				m.startDownload(v, downloader.TypeVideo)
			}
		case key.Matches(msg, m.keys.DownloadAudio):
			if v, ok := m.currentVideo(); ok {
				m.startDownload(v, downloader.TypeAudio)
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
		// DrillDown on a video row: delegate to shared helper below.
		// Explicit return prevents helper from firing a second DrillDown.
		m.handleVideoAction(msg)
		return m, nil
	}
	m.handleVideoAction(msg)
	return m, nil
}

func (m Model) handleChannelEditInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Escape):
		m.subChEditMode = 0
		m.subChEditInput.Blur()
	case key.Matches(msg, m.keys.DrillDown):
		val := strings.TrimSpace(m.subChEditInput.Value())
		chID := m.editTargetChannelID()
		if chID != "" {
			if m.subChEditMode == 1 {
				_ = m.db.SetChannelAlias(chID, val)
				for i, ch := range m.subChannels {
					if ch.ID == chID {
						m.subChannels[i].Alias = val
						break
					}
				}
				if val == "" {
					m.setStatus("Alias cleared", false)
				} else {
					m.setStatus("Alias set: "+val, false)
				}
			} else {
				tags := parseTags(val)
				_ = m.db.SetChannelTags(chID, tags)
				for i, ch := range m.subChannels {
					if ch.ID == chID {
						m.subChannels[i].Tags = tags
						break
					}
				}
				m.setStatus("Tags updated", false)
			}
		}
		m.subChEditMode = 0
		m.subChEditInput.Blur()
	default:
		var cmd tea.Cmd
		m.subChEditInput, cmd = m.subChEditInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleSearchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	blurSearch := func() {
		m.searchFocused = false
		m.searchInput.Blur()
	}
	switch {
	case msg.String() == "up": // arrow-only: avoid vim Up binding ('k') typing in box
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
	case msg.String() == "down": // arrow-only: avoid vim Down binding ('j') typing in box
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
	case key.Matches(msg, m.keys.DrillDown):
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
	case key.Matches(msg, m.keys.Escape):
		if m.searchHistIdx != -1 {
			m.searchHistIdx = -1
			m.searchInput.SetValue(m.searchDraft)
			m.searchInput.CursorEnd()
			return m, nil
		}
		blurSearch()
		return m, nil
	case key.Matches(msg, m.keys.Tab):
		blurSearch()
		idx := m.currentTabIndex()
		m.activeTab = m.tabs[(idx+1)%len(m.tabs)]
		return m, m.onTabActivated()
	case key.Matches(msg, m.keys.ShiftTab):
		blurSearch()
		idx := m.currentTabIndex()
		m.activeTab = m.tabs[(idx+len(m.tabs)-1)%len(m.tabs)]
		return m, m.onTabActivated()
	case msg.String() == "f2":
		blurSearch()
		return m.switchToTabPos(0)
	case msg.String() == "f3":
		blurSearch()
		return m.switchToTabPos(1)
	case msg.String() == "f4":
		blurSearch()
		return m.switchToTabPos(2)
	case msg.String() == "f5":
		blurSearch()
		return m.switchToTabPos(3)
	case msg.String() == "f6":
		blurSearch()
		return m.switchToTabPos(4)
	case msg.String() == "f7":
		blurSearch()
		return m.switchToTabPos(5)
	case msg.String() == "f8":
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
		m.dlCursor, m.dlVS = vsMove(m.dlCursor, m.dlVS, n, -1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.Down):
		m.dlCursor, m.dlVS = vsMove(m.dlCursor, m.dlVS, n, +1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.Play):
		if m.dlCursor < n {
			item := items[m.dlCursor]
			if item.Status == downloader.StatusComplete {
				if lv, ok := m.localVideoIDs[item.Video.ID]; ok {
					m.launchVideo(lv)
				}
			} else {
				m.playVideo(item.Video)
			}
		}
	case key.Matches(msg, m.keys.PlayAudio):
		if m.dlCursor < n {
			m.playAudio(items[m.dlCursor].Video)
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
			m.dlCursor, m.dlVS = vsMove(clamp(m.dlCursor, len(m.downloader.Items())), m.dlVS, len(m.downloader.Items()), 0, m.pageSize(), false)
			m.setStatus("Deleted: "+truncate(item.Video.Title, 50), false)
		}
	case key.Matches(msg, m.keys.CopyURL):
		m.copyCurrentURL()

	}
	return m, nil
}

// ── Local ─────────────────────────────────────────────────────────────────────

func (m Model) updateLocal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.localVideos)
	switch {
	case key.Matches(msg, m.keys.Up):
		m.localCursor, m.localVS = vsMove(m.localCursor, m.localVS, n, -1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.Down):
		m.localCursor, m.localVS = vsMove(m.localCursor, m.localVS, n, +1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.PageUp):
		m.localCursor, m.localVS = vsPage(m.localCursor, m.localVS, n, -1, m.pageSize(), m.cfg.CircularNav)
	case key.Matches(msg, m.keys.PageDown):
		m.localCursor, m.localVS = vsPage(m.localCursor, m.localVS, n, +1, m.pageSize(), m.cfg.CircularNav)
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
			m.localCursor, m.localVS = vsMove(clamp(m.localCursor, len(m.localVideos)), m.localVS, len(m.localVideos), 0, m.pageSize(), false)
			m.setStatus("Deleted: "+truncate(lv.Title, 50), false)
		}
	case key.Matches(msg, m.keys.CopyURL):
		m.copyCurrentURL()

	}
	return m, nil
}

// ── History ───────────────────────────────────────────────────────────────────

func (m Model) updateHistory(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	intent := m.history.update(msg, m.keys, m.pageSize(), m.cfg.CircularNav, m.db)
	switch intent.kind {
	case histIntentDrillSearch:
		// Navigate to search tab with query pre-filled; user presses Enter again to search.
		m.activeTab = tabSearch
		cmd := m.onTabActivated()
		m.searchInput.SetValue(intent.entry.Details)
		m.searchInput.CursorEnd()
		return m, cmd
	case histIntentPlay:
		e := intent.entry
		v := youtube.Video{ID: e.VideoID, Title: e.Title, URL: "https://www.youtube.com/watch?v=" + e.VideoID}
		m.playVideo(v)
	case histIntentDelete:
		e := intent.entry
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
			_ = m.db.DeleteVideoPosition(e.VideoID)
			delete(m.streamedVideoIDs, e.VideoID)
			delete(m.videoPositions, e.VideoID)
			m.setStatus("Deleted: "+truncate(e.Title, 50), false)
		}
	case histIntentHide:
		m.hideChannel(intent.entry.ChannelID, intent.entry.Channel)
	}
	return m, nil
}

// ── Playlist create type selector ─────────────────────────────────────────────

func (m Model) handleCreateTypeInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.createTypeMode = false
	case "up", "k":
		m.createTypeSel = 0
	case "down", "j":
		m.createTypeSel = 1
	case "enter":
		m.createTypeMode = false
		m.createModeYT = m.createTypeSel == 1
		m.createInput.SetValue("")
		m.createInput.Placeholder = "Playlist name…"
		m.createInput.Focus()
		m.createMode = true
		return m, textinput.Blink
	}
	return m, nil
}

// ── Playlist create input ─────────────────────────────────────────────────────

func (m Model) handleCreateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.DrillDown):
		name := m.createInput.Value()
		isYT := m.createModeYT
		m.createMode = false
		m.createModeYT = false
		m.createInput.Blur()
		if name != "" {
			if isYT && m.ytClient != nil {
				return m, youtube.CreateYTPlaylist(m.ytClient, name)
			}
			if id, err := m.db.CreatePlaylist(name); err != nil {
				m.addAfterCreate = false
				m.setStatus("create playlist: "+err.Error(), true)
			} else {
				playlists, _ := m.db.Playlists()
				m.playlists = playlists
				_ = m.db.LogActivity(db.ActivityEntry{
					Type: "create_playlist", IsLocal: true,
					PlaylistLocalID: id, PlaylistName: name,
				})
				if m.addAfterCreate {
					m.addAfterCreate = false
					_ = m.db.AddToPlaylist(id, m.addVideo.ID)
					delete(m.playlistVidCache, fmt.Sprintf("local:%d", id))
					_ = m.db.LogActivity(db.ActivityEntry{
						Type: "add_to_playlist", IsLocal: true,
						PlaylistLocalID: id, PlaylistName: name,
						VideoID: m.addVideo.ID, VideoTitle: m.addVideo.Title,
					})
					m.setStatus(fmt.Sprintf("Created '%s' and added video", name), false)
				} else {
					m.setStatus("Created playlist: "+name, false)
				}
			}
		} else {
			m.addAfterCreate = false
		}
	case key.Matches(msg, m.keys.Escape):
		m.addAfterCreate = false
		m.createMode = false
		m.createModeYT = false
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
	m.addOverlayCreateMode = false
}

func (m Model) closeVideoDetail() Model {
	m.vidDetailOverlay = false
	m.vidDetailVideo = nil
	m.vidDetailThumb = nil
	m.vidDetailLinks = nil
	m.vidDetailChapters = nil
	m.vidDetailDescLines = nil
	m.vidDetailThumbB64 = ""
	m.vidDetailThumbRendered = ""
	m.vidDetailKittyOverlay = ""
	m.vidDetailLoading = false
	m.chapterOverlay = false
	return m
}

func (m Model) handleVideoDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	kb := m.cfg.Keybindings
	// Resolve pending chords without closing the panel — unless a tab switch
	// actually happened, in which case close the panel.
	if m.pendingChord != "" {
		prevTab := m.activeTab
		result, cmd := m.resolveChord(msg.String())
		nm := result.(Model)
		if nm.activeTab != prevTab {
			nm = nm.closeVideoDetail()
			nm.vidDetailDescVS = 0
		}
		return nm, cmd
	}
	// Tab chord: initiate — second key resolves above.
	if msg.String() == kb.TabChord {
		m.pendingChord = kb.TabChord
		m.chordBuffer = ""
		return m, nil
	}
	if msg.String() == kb.GotoPrefix {
		if m.gPending {
			m.gPending = false
			m.vidDetailDescVS = 0
		} else {
			m.gPending = true
		}
		return m, nil
	}
	m.gPending = false
	switch {
	case key.Matches(msg, m.keys.GotoBottom):
		m.vidDetailDescVS = len(m.vidDetailDescLines) // clamped to maxVS in renderVideoDetailPanel
	case key.Matches(msg, m.keys.Escape), key.Matches(msg, m.keys.Left), key.Matches(msg, m.keys.Quit):
		m = m.closeVideoDetail()
		m.vidDetailDescVS = 0
	case key.Matches(msg, m.keys.Down):
		m.vidDetailDescVS++
	case key.Matches(msg, m.keys.Up):
		if m.vidDetailDescVS > 0 {
			m.vidDetailDescVS--
		}
	case key.Matches(msg, m.keys.PageDown):
		m.vidDetailDescVS += m.pageSize()
	case key.Matches(msg, m.keys.PageUp):
		m.vidDetailDescVS -= m.pageSize()
		if m.vidDetailDescVS < 0 {
			m.vidDetailDescVS = 0
		}
	case key.Matches(msg, m.keys.OpenLinks):
		if m.vidDetailVideo != nil {
			if m.vidDetailLinks == nil {
				urls := extractLinks(m.vidDetailVideo.Description)
				m.vidDetailLinks = &urls
				_ = m.db.SaveVideoLinks(m.vidDetailVideo.Video.ID, urls)
			}
			if len(*m.vidDetailLinks) == 0 {
				m.setStatus("no links in description", false)
			} else {
				m.linkOverlayURLs = *m.vidDetailLinks
				m.linkOverlaySel = 0
				m.linkOverlay = true
			}
		}
	case key.Matches(msg, m.keys.OpenChapters):
		if m.vidDetailChapters != nil && len(*m.vidDetailChapters) > 0 {
			m.chapterOverlayItems = *m.vidDetailChapters
			m.chapterOverlaySel = 0
			m.chapterOverlay = true
		} else {
			m.setStatus("no chapters available", false)
		}
	}
	return m, nil
}

// moveOverlayCursor handles gg/G/Up/Down navigation shared by all overlays.
// Returns (newSel, true) when a nav key was consumed; clears m.gPending on any
// non-GotoPrefix key so callers need not do it themselves.
func (m *Model) moveOverlayCursor(sel, n int, msg tea.KeyMsg) (int, bool) {
	if msg.String() == m.cfg.Keybindings.GotoPrefix {
		if m.gPending {
			m.gPending = false
			return 0, true
		}
		m.gPending = true
		return sel, true
	}
	m.gPending = false
	switch {
	case key.Matches(msg, m.keys.GotoBottom):
		if n > 0 {
			return n - 1, true
		}
		return sel, true
	case key.Matches(msg, m.keys.Up):
		if sel > 0 {
			return sel - 1, true
		}
		if m.cfg.CircularNav && n > 0 {
			return n - 1, true
		}
		return sel, true
	case key.Matches(msg, m.keys.Down):
		if sel < n-1 {
			return sel + 1, true
		}
		if m.cfg.CircularNav {
			return 0, true
		}
		return sel, true
	}
	return sel, false
}

func (m Model) handleLinkOverlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.linkOverlayURLs)
	if newSel, handled := (&m).moveOverlayCursor(m.linkOverlaySel, n, msg); handled {
		m.linkOverlaySel = newSel
		return m, nil
	}
	switch {
	case key.Matches(msg, m.keys.DrillDown):
		if n > 0 {
			if err := exec.Command("xdg-open", m.linkOverlayURLs[m.linkOverlaySel].URL).Start(); err != nil {
				m.setStatus("open URL: "+err.Error(), true)
			} else if m.cfg.CloseOnLinkOpen {
				m.linkOverlay = false
			}
		}
	case key.Matches(msg, m.keys.Escape), key.Matches(msg, m.keys.Quit):
		m.linkOverlay = false
	case key.Matches(msg, m.keys.CopyURL):
		if n > 0 {
			u := m.linkOverlayURLs[m.linkOverlaySel].URL
			if err := clipboard.WriteAll(u); err != nil {
				m.setStatus("clipboard: "+err.Error(), true)
			} else {
				m.setStatus("copied: "+u, false)
			}
		}
	}
	return m, nil
}

func (m Model) handleChapterOverlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.chapterOverlayItems)
	if newSel, handled := (&m).moveOverlayCursor(m.chapterOverlaySel, n, msg); handled {
		m.chapterOverlaySel = newSel
		return m, nil
	}
	switch {
	case key.Matches(msg, m.keys.Escape), key.Matches(msg, m.keys.Quit):
		m.chapterOverlay = false
	case key.Matches(msg, m.keys.Play):
		if n > 0 && m.vidDetailVideo != nil {
			m.playVideoFromChapter(m.chapterOverlayItems[m.chapterOverlaySel])
		}
	case key.Matches(msg, m.keys.PlayAudio):
		if n > 0 && m.vidDetailVideo != nil {
			m.playAudioFromChapter(m.chapterOverlayItems[m.chapterOverlaySel])
		}
	case key.Matches(msg, m.keys.CopyURL):
		if n > 0 && m.vidDetailVideo != nil {
			ch := m.chapterOverlayItems[m.chapterOverlaySel]
			secs := int(ch.OriginalStart)
			u := fmt.Sprintf("https://www.youtube.com/watch?v=%s&t=%d", m.vidDetailVideo.Video.ID, secs)
			if err := clipboard.WriteAll(u); err != nil {
				m.setStatus("clipboard: "+err.Error(), true)
			} else {
				m.setStatus("copied: "+u, false)
			}
		}
	}
	return m, nil
}

var linkRe = regexp.MustCompile(`https?://[^\s\]>)"']+`)

func extractLinks(desc string) []db.Link {
	seen := make(map[string]bool)
	var out []db.Link
	for _, line := range strings.Split(desc, "\n") {
		for _, loc := range linkRe.FindAllStringIndex(line, -1) {
			url := strings.TrimRight(line[loc[0]:loc[1]], ".,;:!?)'\"")
			if seen[url] {
				continue
			}
			seen[url] = true
			label := strings.TrimRight(strings.TrimSpace(line[:loc[0]]), ":,;-–—•►▶→")
			label = strings.TrimSpace(label)
			out = append(out, db.Link{Label: label, URL: url})
		}
	}
	return out
}

func (m Model) handleAddOverlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.addOverlayCreateMode {
		return m.handleAddOverlayCreate(msg)
	}
	n := m.overlayPlaylistCount()
	if newSel, handled := (&m).moveOverlayCursor(m.addOverlaySel, n, msg); handled {
		m.addOverlaySel = newSel
		return m, nil
	}
	switch {
	case key.Matches(msg, m.keys.Escape), key.Matches(msg, m.keys.Quit):
		m.addOverlay = false
	case key.Matches(msg, m.keys.DrillDown):
		v := m.addVideo
		idx := m.addOverlaySel
		base := m.overlayCreateBase()
		if idx == base { // "Create local list"
			m.addOverlayCreateMode = true
			m.addOverlayCreateYT = false
			m.addOverlayInput.SetValue("")
			m.addOverlayInput.Placeholder = "List name…"
			m.addOverlayInput.Focus()
			return m, textinput.Blink
		}
		if m.ytClient != nil && idx == base+1 { // "Create remote playlist"
			m.addOverlayCreateMode = true
			m.addOverlayCreateYT = true
			m.addOverlayInput.SetValue("")
			m.addOverlayInput.Placeholder = "Playlist name…"
			m.addOverlayInput.Focus()
			return m, textinput.Blink
		}
		if m.ytPlLoaded && m.ytClient != nil && idx < len(m.ytPlaylists) {
			pl := m.ytPlaylists[idx]
			go func() { _ = m.ytClient.AddToPlaylist(pl.ID, v.ID) }()
			delete(m.playlistVidCache, pl.ID)
			_ = m.db.LogActivity(db.ActivityEntry{
				Type: "add_to_playlist", IsLocal: false,
				PlaylistID: pl.ID, PlaylistName: pl.Title,
				VideoID: v.ID, VideoTitle: v.Title,
			})
			m.setStatus(fmt.Sprintf("Added to '%s'", pl.Title), false)
		} else if idx < len(m.playlists) {
			pl := m.playlists[idx]
			_ = m.db.AddToPlaylist(pl.ID, v.ID)
			delete(m.playlistVidCache, fmt.Sprintf("local:%d", pl.ID))
			_ = m.db.LogActivity(db.ActivityEntry{
				Type: "add_to_playlist", IsLocal: true,
				PlaylistLocalID: pl.ID, PlaylistName: pl.Name,
				VideoID: v.ID, VideoTitle: v.Title,
			})
			m.setStatus(fmt.Sprintf("Added to '%s'", pl.Name), false)
		}
		m.addOverlay = false
	}
	return m, nil
}

func (m Model) handleAddOverlayCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Escape):
		m.addOverlayCreateMode = false
		m.addOverlayInput.Blur()
	case msg.String() == "enter":
		name := m.addOverlayInput.Value()
		m.addOverlayCreateMode = false
		m.addOverlayInput.Blur()
		m.addOverlay = false
		if name != "" {
			if m.addOverlayCreateYT && m.ytClient != nil {
				m.addAfterCreate = true
				return m, youtube.CreateYTPlaylist(m.ytClient, name)
			}
			if id, err := m.db.CreatePlaylist(name); err != nil {
				m.setStatus("create playlist: "+err.Error(), true)
			} else {
				playlists, _ := m.db.Playlists()
				m.playlists = playlists
				_ = m.db.AddToPlaylist(id, m.addVideo.ID)
				delete(m.playlistVidCache, fmt.Sprintf("local:%d", id))
				_ = m.db.LogActivity(db.ActivityEntry{
					Type: "create_playlist", IsLocal: true,
					PlaylistLocalID: id, PlaylistName: name,
				})
				_ = m.db.LogActivity(db.ActivityEntry{
					Type: "add_to_playlist", IsLocal: true,
					PlaylistLocalID: id, PlaylistName: name,
					VideoID: m.addVideo.ID, VideoTitle: m.addVideo.Title,
				})
				m.setStatus(fmt.Sprintf("Created '%s' and added video", name), false)
			}
		}
	default:
		var cmd tea.Cmd
		m.addOverlayInput, cmd = m.addOverlayInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// overlayPlaylistCount is the total selectable rows in the add-to-playlist overlay.
// Includes create entries: 2 when YT client is present (local + remote), 1 otherwise (local only).
func (m Model) overlayPlaylistCount() int {
	if m.ytPlLoaded && m.ytClient != nil {
		return len(m.ytPlaylists) + 2
	}
	return len(m.playlists) + 1
}

// overlayCreateBase returns the index of the first create entry in the overlay.
func (m Model) overlayCreateBase() int {
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

func (m *Model) streamVideo(v youtube.Video) {
	startAt := time.Duration(0)
	if ms, ok := m.db.VideoPosition(v.ID); ok {
		startAt = time.Duration(ms) * time.Millisecond
		m.videoPositions[v.ID] = ms
	}
	if err := m.playerBackend.Launch(v.URL, startAt); err != nil {
		m.setStatus("stream: "+err.Error(), true)
		return
	}
	m.playingVideoID = v.ID
	m.playingSBSegments = nil // mpv handles SponsorBlock live; MPRIS reports original timeline
	m.streamedVideoIDs[v.ID] = true
	_ = m.db.UpsertVideo(v.ID, v.Title, v.Channel, v.ChannelID, v.Duration, v.ViewCount, v.UploadDate, v.URL)
	_ = m.db.AddHistory(v.ID, "streamVideo", "")
	label := truncate(v.Title, 50)
	if startAt > 0 {
		m.setStatus(fmt.Sprintf("Streaming (from %s): %s", formatDuration(startAt), label), false)
	} else {
		m.setStatus("Streaming: "+label, false)
	}
}

func (m *Model) streamAudio(v youtube.Video) {
	startAt := time.Duration(0)
	if ms, ok := m.db.VideoPosition(v.ID); ok {
		startAt = time.Duration(ms) * time.Millisecond
		m.videoPositions[v.ID] = ms
	}
	if err := m.playerBackend.LaunchAudio(v.URL, startAt); err != nil {
		m.setStatus("stream audio: "+err.Error(), true)
		return
	}
	m.playingVideoID = v.ID
	m.playingSBSegments = nil // mpv handles SponsorBlock live; MPRIS reports original timeline
	m.streamedVideoIDs[v.ID] = true
	_ = m.db.UpsertVideo(v.ID, v.Title, v.Channel, v.ChannelID, v.Duration, v.ViewCount, v.UploadDate, v.URL)
	_ = m.db.AddHistory(v.ID, "streamAudio", "")
	label := truncate(v.Title, 50)
	if startAt > 0 {
		m.setStatus(fmt.Sprintf("Streaming audio (from %s): %s", formatDuration(startAt), label), false)
	} else {
		m.setStatus("Streaming audio: "+label, false)
	}
}

// handleVideoAction dispatches the shared video actions present in most tabs.
// Returns true when msg matched a key. DrillDown maps to downloadAndPlay; tabs with
// a different DrillDown meaning should handle it before calling this and return early.
func (m *Model) handleVideoAction(msg tea.KeyMsg) bool {
	v, ok := m.currentVideo()
	switch {
	case key.Matches(msg, m.keys.DrillDown):
		if ok {
			m.downloadAndPlay(v)
		}
	case key.Matches(msg, m.keys.Play):
		if ok {
			m.playVideo(v)
		}
	case key.Matches(msg, m.keys.PlayAudio):
		if ok {
			m.playAudio(v)
		}
	case key.Matches(msg, m.keys.Download):
		if ok {
			m.startDownload(v, downloader.TypeVideo)
		}
	case key.Matches(msg, m.keys.DownloadAudio):
		if ok {
			m.startDownload(v, downloader.TypeAudio)
		}
	case key.Matches(msg, m.keys.AddList):
		if ok {
			m.openAddOverlay(v)
		}
	case key.Matches(msg, m.keys.CopyURL):
		m.copyCurrentURL()
	default:
		return false
	}
	return true
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
func vsMove(cursor, vs, n, delta, height int, circular bool) (newCursor, newVS int) {
	if n <= 0 {
		return 0, 0
	}
	c := cursor + delta
	if circular {
		c = ((c % n) + n) % n
	} else {
		if c < 0 {
			c = 0
		}
		if c >= n {
			c = n - 1
		}
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
func vsPage(cursor, vs, n, direction, height int, circular bool) (newCursor, newVS int) {
	relPos := cursor - vs
	newVS = vs + direction*height
	if newVS < 0 {
		if circular && n > 0 {
			newVS = max(0, n-height)
		} else {
			newVS = 0
		}
	}
	if newVS+height > n {
		if circular {
			newVS = 0
		} else {
			newVS = n - height
			if newVS < 0 {
				newVS = 0
			}
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
		_, m.searchVS = vsMove(m.searchCursor-nCh, m.searchVS, nVid, 0, m.pageSize(), false)
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
	out := make([]youtube.Video, 0, len(videos))
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
	out := make([]youtube.Video, 0, len(videos))
	for _, v := range videos {
		if _, ok := local[v.ID]; !ok {
			out = append(out, v)
		}
	}
	return out
}

// filterHidden removes videos the user has explicitly hidden from recommended.
func filterHidden(videos []youtube.Video, hidden map[string]bool) []youtube.Video {
	out := make([]youtube.Video, 0, len(videos))
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
	out := make([]youtube.Video, 0, len(videos))
	for _, v := range videos {
		if bl, matched := matchBlacklisted(v, list); matched {
			if bl >= 0 && cfg.BlacklistedChannels[bl].ID == "" && v.ChannelID != "" {
				cfg.SetBlacklistID(bl, v.ChannelID)
				cfg.SaveAsync()
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

// playVideo plays locally if downloaded, otherwise streams.
func (m *Model) playVideo(v youtube.Video) {
	if lv, ok := m.localVideoIDs[v.ID]; ok {
		m.launchVideo(lv)
	} else {
		m.streamVideo(v)
	}
}

// playAudio plays the local file in audio-only mode if downloaded, otherwise streams audio.
func (m *Model) playAudio(v youtube.Video) {
	if lv, ok := m.localVideoIDs[v.ID]; ok {
		m.launchVideoAudio(lv)
	} else {
		m.streamAudio(v)
	}
}

// launchVideoAudio plays a local video file in audio-only mode.
func (m *Model) launchVideoAudio(lv db.LocalVideo) {
	if _, err := os.Stat(lv.FilePath); err != nil {
		m.setStatus("File not found: "+truncate(lv.Title, 50), true)
		return
	}
	posMs, _ := m.db.VideoPosition(lv.ID)
	sbSegs := m.loadSBSegmentsForVideo(lv.ID)
	fileMs := posMs
	if len(sbSegs) > 0 {
		fileMs = originalToAdjustedMs(posMs, sbSegs)
	}
	startAt := time.Duration(fileMs) * time.Millisecond
	if err := m.playerBackend.LaunchAudio(lv.FilePath, startAt); err != nil {
		m.setStatus("play audio: "+err.Error(), true)
		return
	}
	m.playingVideoID = lv.ID
	m.playingSBSegments = sbSegs
	_ = m.db.AddHistory(lv.ID, "playAudio", "")
	label := truncate(lv.Title, 50)
	if startAt > 0 {
		m.setStatus(fmt.Sprintf("Playing audio (from %s): %s", formatDuration(startAt), label), false)
	} else {
		m.setStatus("Playing audio: "+label, false)
	}
}

// launchVideo starts playback of a local video, resuming from last position.
func (m *Model) launchVideo(lv db.LocalVideo) {
	if _, err := os.Stat(lv.FilePath); err != nil {
		m.setStatus("File not found: "+truncate(lv.Title, 50), true)
		return
	}
	posMs, _ := m.db.VideoPosition(lv.ID)
	sbSegs := m.loadSBSegmentsForVideo(lv.ID)
	fileMs := posMs
	if len(sbSegs) > 0 {
		fileMs = originalToAdjustedMs(posMs, sbSegs)
	}
	startAt := time.Duration(fileMs) * time.Millisecond
	if err := m.playerBackend.Launch(lv.FilePath, startAt); err != nil {
		m.setStatus("play: "+err.Error(), true)
		return
	}
	m.playingVideoID = lv.ID
	m.playingSBSegments = sbSegs
	_ = m.db.SetVideoStatus(lv.ID, db.StatusStarted)
	_ = m.db.AddHistory(lv.ID, "playVideo", "")
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
	// Check if this is a local-only subscription.
	for _, ch := range m.subChannels {
		if ch.ID == chID {
			if ch.IsLocal {
				return m.unsubscribeLocal(chID, chName)
			}
			break
		}
	}
	return m.unsubscribeChannel(chID, chName)
}

func (m Model) unsubscribeLocal(chID, chName string) (tea.Model, tea.Cmd) {
	if chID == "" {
		m.setStatus("unsubscribe: no channel", true)
		return m, nil
	}
	_ = m.db.RemoveSubscribedChannel(chID)
	m.subChannels = removeChannelByID(m.subChannels, chID)
	delete(m.subscribedChannelIDs, chID)
	delete(m.subscribedChannelIDs, "name:"+strings.ToLower(chName))
	// Strip the channel's videos from subscription feeds and purge from DB.
	m.subVideos = removeChannelVideos(m.subVideos, chID, chName)
	m.subCursor, m.subVS = vsMove(clamp(m.subCursor, len(m.subVideos)), m.subVS, len(m.subVideos), 0, m.pageSize(), false)
	m.subChVideos = removeChannelVideos(m.subChVideos, chID, chName)
	m.subChVidCursor, m.subChVidVS = vsMove(clamp(m.subChVidCursor, len(m.subChVideos)), m.subChVidVS, len(m.subChVideos), 0, m.pageSize(), false)
	go m.db.DeleteChannelVideos(chID)
	m.setStatus("Removed local subscription: "+chName, false)
	// Trigger a fresh recommended fetch so the channel's videos drip back in.
	m.recPage = 0
	m.recLoading = true
	m.recRefreshing = m.recLoaded
	return m, youtube.FetchRecommended(m.cfg)
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
	if m.activeTab == tabChannels && !m.subChTagsMode && m.subChPane == 0 {
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
	if m.activeTab == tabHistory && m.history.detailVideoID == "" {
		if m.history.cursor < len(m.history.entries) {
			e := m.history.entries[m.history.cursor]
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
	m.cfg.SaveAsync()
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
	m.recCursor, m.recVS = vsMove(clamp(m.recCursor, len(m.recVideos)), m.recVS, len(m.recVideos), 0, m.pageSize(), false)
	m.subVideos = removeChannelVideos(m.subVideos, channelID, channelName)
	m.subCursor, m.subVS = vsMove(clamp(m.subCursor, len(m.subVideos)), m.subVS, len(m.subVideos), 0, m.pageSize(), false)
	m.subChVideos = removeChannelVideos(m.subChVideos, channelID, channelName)
	m.subChVidCursor, m.subChVidVS = vsMove(clamp(m.subChVidCursor, len(m.subChVideos)), m.subChVidVS, len(m.subChVideos), 0, m.pageSize(), false)
}

// ── SponsorBlock chapter processing ──────────────────────────────────────────

// processChapters splits raw yt-dlp chapters (which include [SponsorBlock] entries
// when --sponsorblock-chapters was passed) into display chapters and SB segments.
// Display chapters have their timecodes adjusted to reflect the local-file timeline
// (after SB cuts); chapters whose boundaries coincide with a SB segment (±3 s on
// both start AND end) are dropped entirely.
func processChapters(all []youtube.Chapter) ([]db.Chapter, []db.SBSegment) {
	const tol = 3.0
	var sbChapters []youtube.Chapter
	var realChapters []youtube.Chapter
	for _, ch := range all {
		if strings.HasPrefix(ch.Title, "[SponsorBlock]") {
			sbChapters = append(sbChapters, ch)
		} else {
			realChapters = append(realChapters, ch)
		}
	}

	segs := make([]db.SBSegment, len(sbChapters))
	for i, ch := range sbChapters {
		segs[i] = db.SBSegment{Start: ch.StartTime, End: ch.EndTime}
	}

	var out []db.Chapter
	for _, ch := range realChapters {
		skip := false
		for _, sb := range segs {
			if math.Abs(ch.StartTime-sb.Start) <= tol && math.Abs(ch.EndTime-sb.End) <= tol {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		out = append(out, db.Chapter{
			Title:         ch.Title,
			OriginalStart: ch.StartTime,
			OriginalEnd:   ch.EndTime,
			AdjustedStart: originalToAdjustedSec(ch.StartTime, segs),
			AdjustedEnd:   originalToAdjustedSec(ch.EndTime, segs),
		})
	}
	return out, segs
}

// originalToAdjustedSec converts an original-timeline position (seconds) to the
// adjusted local-file position after SB segments have been removed.
func originalToAdjustedSec(origSec float64, segs []db.SBSegment) float64 {
	offset := 0.0
	for _, seg := range segs {
		if origSec < seg.Start {
			break
		}
		if origSec < seg.End {
			return seg.Start - offset
		}
		offset += seg.End - seg.Start
	}
	return origSec - offset
}

// originalToAdjustedMs converts an original-timeline position (ms) to the adjusted
// local-file position in ms.
func originalToAdjustedMs(origMs int64, segs []db.SBSegment) int64 {
	offset := int64(0)
	for _, seg := range segs {
		segStartMs := int64(seg.Start * 1000)
		segEndMs := int64(seg.End * 1000)
		if origMs < segStartMs {
			break
		}
		if origMs < segEndMs {
			return segStartMs - offset
		}
		offset += segEndMs - segStartMs
	}
	return origMs - offset
}

// adjustedToOriginalMs converts a local-file position (ms) back to the original
// video timeline in ms, undoing the SB cuts.
func adjustedToOriginalMs(adjMs int64, segs []db.SBSegment) int64 {
	offset := int64(0)
	for _, seg := range segs {
		segDur := int64((seg.End - seg.Start) * 1000)
		segStartAdj := int64(seg.Start*1000) - offset
		if adjMs < segStartAdj {
			break
		}
		offset += segDur
	}
	return adjMs + offset
}

// ── Direct overlay helpers (chapters/links without opening info panel) ────────

// loadSBSegmentsForVideo fetches the stored SponsorBlock segments for a video from
// the DB cache. Returns nil if none are stored.
func (m *Model) loadSBSegmentsForVideo(id string) []db.SBSegment {
	if cached, ok, _ := m.db.GetVideoDetailsCache(id); ok && cached.SBSegments != nil && len(*cached.SBSegments) > 0 {
		return *cached.SBSegments
	}
	return nil
}

// openVideoDetail opens the video-detail overlay for v, resetting all detail
// state and serving from cache when available.
func (m *Model) openVideoDetail(v youtube.Video) tea.Cmd {
	m.vidDetailOverlay = true
	m.vidDetailLoading = true
	m.vidDetailVideo = nil
	m.vidDetailThumb = nil
	m.vidDetailDescVS = 0
	m.vidDetailLinks = nil
	m.vidDetailChapters = nil
	m.vidDetailDescLines = nil
	m.vidDetailThumbB64 = ""
	m.vidDetailThumbRendered = ""
	m.vidDetailKittyOverlay = ""
	if cached, ok, _ := m.db.GetVideoDetailsCache(v.ID); ok {
		details := youtube.VideoDetails{Video: v, Description: cached.Description, ThumbnailURL: cached.ThumbnailURL, Subscribers: cached.Subscribers}
		m.vidDetailVideo = &details
		m.vidDetailLinks = cached.Links
		m.vidDetailChapters = cached.Chapters
		m.vidDetailDescLines = wordWrap(cached.Description, vidDetailPanelW-2)
		if cached.ThumbnailURL != "" {
			return loadThumbnailCmd(cached.ThumbnailURL)
		}
		m.vidDetailLoading = false
		return nil
	}
	return youtube.FetchVideoDetails(m.cfg, v.URL)
}

// openChaptersForVideo opens the chapter overlay for v, loading from cache if
// available or triggering a video-details fetch otherwise. vidDetailVideo is always
// set so the overlay's y/p actions can reference the video.
func (m Model) openChaptersForVideo(v youtube.Video) (tea.Model, tea.Cmd) {
	if v.URL == "" {
		return m, nil
	}
	if m.vidDetailVideo == nil || m.vidDetailVideo.Video.ID != v.ID {
		vd := youtube.VideoDetails{Video: v}
		m.vidDetailVideo = &vd
	}
	if cached, ok, _ := m.db.GetVideoDetailsCache(v.ID); ok && cached.Chapters != nil {
		if len(*cached.Chapters) > 0 {
			m.chapterOverlayItems = *cached.Chapters
			m.chapterOverlaySel = 0
			m.chapterOverlay = true
		} else {
			m.setStatus("no chapters available", false)
		}
		return m, nil
	}
	m.pendingDirectOverlay = "chapters"
	m.setStatus("Loading chapters…", false)
	return m, youtube.FetchVideoDetails(m.cfg, v.URL)
}

// openLinksForVideo opens the link overlay for v, loading from cache if available
// or triggering a fetch otherwise.
func (m Model) openLinksForVideo(v youtube.Video) (tea.Model, tea.Cmd) {
	if v.URL == "" {
		return m, nil
	}
	if m.vidDetailVideo == nil || m.vidDetailVideo.Video.ID != v.ID {
		vd := youtube.VideoDetails{Video: v}
		m.vidDetailVideo = &vd
	}
	if cached, ok, _ := m.db.GetVideoDetailsCache(v.ID); ok {
		var links []db.Link
		if cached.Links != nil {
			links = *cached.Links
		} else {
			links = extractLinks(cached.Description)
			_ = m.db.SaveVideoLinks(v.ID, links)
			m.vidDetailLinks = &links
		}
		if len(links) == 0 {
			m.setStatus("no links in description", false)
		} else {
			m.linkOverlayURLs = links
			m.linkOverlaySel = 0
			m.linkOverlay = true
		}
		return m, nil
	}
	m.pendingDirectOverlay = "links"
	m.setStatus("Loading links…", false)
	return m, youtube.FetchVideoDetails(m.cfg, v.URL)
}

// ── Chapter playback from overlay ─────────────────────────────────────────────

// playVideoFromChapter seeks to the chapter's time and starts video playback.
// Local files use the adjusted (file) timestamp; streaming uses the original.
func (m *Model) playVideoFromChapter(ch db.Chapter) {
	if m.vidDetailVideo == nil {
		return
	}
	v := m.vidDetailVideo.Video
	label := truncate(v.Title, 50)
	if lv, ok := m.localVideoIDs[v.ID]; ok {
		if _, err := os.Stat(lv.FilePath); err != nil {
			m.setStatus("File not found: "+label, true)
			return
		}
		startAt := time.Duration(ch.AdjustedStart * float64(time.Second))
		if err := m.playerBackend.Launch(lv.FilePath, startAt); err != nil {
			m.setStatus("play: "+err.Error(), true)
			return
		}
		m.playingVideoID = v.ID
		m.playingSBSegments = m.loadSBSegmentsForVideo(v.ID)
		_ = m.db.AddHistory(v.ID, "playVideo", "")
		m.setStatus(fmt.Sprintf("Playing (from %s): %s", fmtChapterTime(ch.AdjustedStart), label), false)
	} else {
		startAt := time.Duration(ch.OriginalStart * float64(time.Second))
		if err := m.playerBackend.Launch(v.URL, startAt); err != nil {
			m.setStatus("stream: "+err.Error(), true)
			return
		}
		m.playingVideoID = v.ID
		m.playingSBSegments = nil
		m.streamedVideoIDs[v.ID] = true
		_ = m.db.UpsertVideo(v.ID, v.Title, v.Channel, v.ChannelID, v.Duration, v.ViewCount, v.UploadDate, v.URL)
		_ = m.db.AddHistory(v.ID, "streamVideo", "")
		m.setStatus(fmt.Sprintf("Streaming (from %s): %s", fmtChapterTime(ch.OriginalStart), label), false)
	}
}

// playAudioFromChapter seeks to the chapter's time and starts audio-only playback.
func (m *Model) playAudioFromChapter(ch db.Chapter) {
	if m.vidDetailVideo == nil {
		return
	}
	v := m.vidDetailVideo.Video
	label := truncate(v.Title, 50)
	if lv, ok := m.localVideoIDs[v.ID]; ok {
		if _, err := os.Stat(lv.FilePath); err != nil {
			m.setStatus("File not found: "+label, true)
			return
		}
		startAt := time.Duration(ch.AdjustedStart * float64(time.Second))
		if err := m.playerBackend.LaunchAudio(lv.FilePath, startAt); err != nil {
			m.setStatus("play audio: "+err.Error(), true)
			return
		}
		m.playingVideoID = v.ID
		m.playingSBSegments = m.loadSBSegmentsForVideo(v.ID)
		_ = m.db.AddHistory(v.ID, "playAudio", "")
		m.setStatus(fmt.Sprintf("Playing audio (from %s): %s", fmtChapterTime(ch.AdjustedStart), label), false)
	} else {
		startAt := time.Duration(ch.OriginalStart * float64(time.Second))
		if err := m.playerBackend.LaunchAudio(v.URL, startAt); err != nil {
			m.setStatus("stream audio: "+err.Error(), true)
			return
		}
		m.playingVideoID = v.ID
		m.playingSBSegments = nil
		m.streamedVideoIDs[v.ID] = true
		_ = m.db.UpsertVideo(v.ID, v.Title, v.Channel, v.ChannelID, v.Duration, v.ViewCount, v.UploadDate, v.URL)
		_ = m.db.AddHistory(v.ID, "streamAudio", "")
		m.setStatus(fmt.Sprintf("Streaming audio (from %s): %s", fmtChapterTime(ch.OriginalStart), label), false)
	}
}
