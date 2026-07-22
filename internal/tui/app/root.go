package app

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/device/player"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/command"
	"github.com/EugeneShtoka/yt-tui/internal/tui/component"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	ovpkg "github.com/EugeneShtoka/yt-tui/internal/tui/overlay"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/tab"
	"github.com/atotto/clipboard"
)

// History event type constants — stored in DB; must match DB migration strings.
const (
	evtStreamVideo = "streamVideo"
	evtStreamAudio = "streamAudio"
	evtPlayVideo   = "playVideo"
	evtPlayAudio   = "playAudio"
)

// playerStartedMsg is a root-internal signal emitted by playCmd after the
// player process has been launched. Root handles it by showing status and
// scheduling playerWaitCmd to track process exit.
type playerStartedMsg struct {
	videoID string
	sess    *player.Session
	text    string
}

// Root is the top-level BubbleTea model.
// It owns focus, size, global key routing, and the tab/overlay stack.
type Root struct {
	backend api.Backend
	cfg     *config.Config
	player  player.Backend
	keys    keymap.KeyMap
	cmds    command.Registry

	width, height int

	tabBar    component.TabBar
	statusBar component.StatusBar

	tabs      []tuipkg.Tab
	activeIdx int

	overlays []ovpkg.Overlay

	tabChordActive bool
	tabChordKeys   map[string]tuipkg.TabID
}

// New constructs the Root with the current tab set.
// pl may be nil if no player binary was found; play actions will show an error.
func New(backend api.Backend, cfg *config.Config, pl player.Backend) Root {
	keys := keymap.Build(cfg.Keybindings)

	tabs := []tuipkg.Tab{
		tab.NewRecommended(backend, keys, cfg.CircularNav),
		tab.NewSubscriptions(backend, keys, cfg.CircularNav),
		tab.NewChannels(backend, keys, cfg.CircularNav, cfg.ChannelLatestCount),
		tab.NewTags(backend, keys, cfg.CircularNav),
		tab.NewPlaylists(backend, keys, cfg.CircularNav),
		tab.NewSearch(backend, keys, cfg.CircularNav),
		tab.NewDownloading(backend, keys, cfg.CircularNav),
		tab.NewLocal(backend, keys, cfg.CircularNav),
		tab.NewHistory(backend, keys, cfg.CircularNav),
		tab.NewActivity(backend, keys, cfg.CircularNav),
	}

	titles := make([]string, len(tabs))
	for i, t := range tabs {
		titles[i] = t.Title()
	}

	var cmds command.Registry
	cmds.Register(globalCommands(backend)...)

	right := keys.Help.Help().Key + ": help  " + keys.Quit.Help().Key + ": quit"

	tk := cfg.Keybindings.TabKeys
	tabChordKeys := map[string]tuipkg.TabID{
		tk.Recommended:   tuipkg.TabRecommended,
		tk.Subscriptions: tuipkg.TabSubscriptions,
		tk.Channels:      tuipkg.TabChannels,
		tk.Tags:          tuipkg.TabTags,
		tk.Playlists:     tuipkg.TabPlaylists,
		tk.Search:        tuipkg.TabSearch,
		tk.Downloading:   tuipkg.TabDownloading,
		tk.Local:         tuipkg.TabLocal,
		tk.History:       tuipkg.TabHistory,
		tk.Activity:      tuipkg.TabActivity,
	}

	return Root{
		backend:      backend,
		cfg:          cfg,
		player:       pl,
		keys:         keys,
		cmds:         cmds,
		tabBar:       component.NewTabBar(titles),
		statusBar:    component.NewStatusBar(right),
		tabs:         tabs,
		tabChordKeys: tabChordKeys,
	}
}

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (r Root) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(r.tabs))
	for _, t := range r.tabs {
		cmds = append(cmds, t.Init())
	}
	return tea.Batch(cmds...)
}

func (r Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tea.WindowSizeMsg:
		return r.handleResize(m.Width, m.Height)

	case tea.KeyPressMsg:
		return r.handleKey(m)

	case tuipkg.OpenOverlayMsg:
		return r.handleOpenOverlay(m)

	case ovpkg.PopOverlayMsg:
		return r.handlePopOverlay()

	case tuipkg.PlayVideoMsg:
		v, audio := m.Video, m.AudioOnly
		evt := evtStreamVideo
		if audio {
			evt = evtStreamAudio
		}
		return r, r.playCmd(v.ID, v.URL, v.Title, audio, evt)

	case tuipkg.LaunchLocalVideoMsg:
		lv := m.Video
		// For local videos, pass empty fallbackURL — InProc returns the file path,
		// Remote returns the daemon's /media/{id} URL.
		return r, r.playCmd(lv.ID, "", lv.Title, false, evtPlayVideo)

	case playerStartedMsg:
		return r, tea.Batch(
			func() tea.Msg { return tuipkg.StatusMsg{Text: m.text} },
			func() tea.Msg { return tuipkg.HistoryChangedMsg{} },
			r.playerWaitCmd(m.videoID, m.sess),
		)

	case tuipkg.RefreshPositionsMsg:
		var bcmds []tea.Cmd
		for i, t := range r.tabs {
			updated, cmd := t.Update(msg)
			r.tabs[i] = updated.(tuipkg.Tab)
			bcmds = append(bcmds, cmd)
		}
		return r, tea.Batch(bcmds...)

	case tuipkg.EnqueueMsg:
		v, audio := m.Video, m.AudioOnly
		return r, func() tea.Msg {
			if err := r.backend.Enqueue(context.Background(), v, audio); err != nil {
				return tuipkg.StatusMsg{Text: "enqueue: " + err.Error(), IsErr: true}
			}
			return tuipkg.EnqueueSucceededMsg{Title: v.Title, AudioOnly: audio}
		}

	case tuipkg.EnqueueSucceededMsg:
		label := "video"
		if m.AudioOnly {
			label = "audio"
		}
		return r, tea.Batch(
			func() tea.Msg {
				return tuipkg.StatusMsg{Text: "Queued " + label + ": " + render.Truncate(m.Title, 50)}
			},
			func() tea.Msg { return tuipkg.DownloadItemsChangedMsg{} },
		)

	case tuipkg.CopyURLMsg:
		url := m.URL
		return r, func() tea.Msg {
			if err := clipboard.WriteAll(url); err != nil {
				return tuipkg.StatusMsg{Text: "clipboard: " + err.Error(), IsErr: true}
			}
			return tuipkg.StatusMsg{Text: "Copied: " + url}
		}

	case tuipkg.NavigateMsg:
		return r.handleNavigate(m)

	case tuipkg.HideChannelMsg:
		return r.handleHideChannel(m)

	case tuipkg.UnsubscribeMsg:
		return r.handleUnsubscribe(m)

	case tuipkg.NavigateToChannelMsg:
		return r.handleNavigate(tuipkg.NavigateMsg{Tab: tuipkg.TabChannels})

	case tuipkg.NavigateToPlaylistMsg:
		var navCmd tea.Cmd
		r, navCmd = r.handleNavigate(tuipkg.NavigateMsg{Tab: tuipkg.TabPlaylists})
		var fwdCmd tea.Cmd
		r, fwdCmd = r.updateActiveTab(m)
		return r, tea.Batch(navCmd, fwdCmd)

	case tuipkg.StatusMsg:
		sb, cmd := r.statusBar.Update(msg)
		r.statusBar = sb.(component.StatusBar)
		return r, cmd

	default:
		// Broadcast to all tabs so that background responses (e.g. subLoadedMsg,
		// chsLoadedMsg) reach their owner tab regardless of which tab is active.
		// Also update the top overlay so it can handle its own private messages.
		var c1 tea.Cmd
		if len(r.overlays) > 0 {
			r, c1 = r.updateTopOverlay(msg)
		}
		var bcmds []tea.Cmd
		if c1 != nil {
			bcmds = append(bcmds, c1)
		}
		for i, t := range r.tabs {
			updated, cmd := t.Update(msg)
			r.tabs[i] = updated.(tuipkg.Tab)
			if cmd != nil {
				bcmds = append(bcmds, cmd)
			}
		}
		return r, tea.Batch(bcmds...)
	}
}

func (r Root) View() tea.View {
	tabBar := r.tabBar.Render()
	status := r.statusBar.Render()
	contentH := r.height - lipgloss.Height(tabBar) - lipgloss.Height(status)

	content := r.activeTab().View().Content

	for _, o := range r.overlays {
		content, _ = o.Render(content, r.width, contentH)
	}

	if actual := lipgloss.Height(content); actual < contentH {
		content += strings.Repeat("\n", contentH-actual)
	}

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, tabBar, content, status))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// ── message handlers ──────────────────────────────────────────────────────────

func (r Root) handleResize(w, h int) (Root, tea.Cmd) {
	r.width, r.height = w, h
	r.tabBar = r.tabBar.WithWidth(w)
	r.statusBar = r.statusBar.WithWidth(w)

	tabBarH := lipgloss.Height(r.tabBar.Render())
	statusH := lipgloss.Height(r.statusBar.Render())
	contentH := h - tabBarH - statusH
	contentW := w - r.overlayWidthReduction()

	sizeMsg := tuipkg.ContentSizeMsg{Width: contentW, Height: contentH}
	var cmds []tea.Cmd
	for i, t := range r.tabs {
		updated, cmd := t.Update(sizeMsg)
		r.tabs[i] = updated.(tuipkg.Tab)
		cmds = append(cmds, cmd)
	}

	// Forward size info to overlays so they can compute Kitty image positions.
	ovMsg := ovpkg.OverlaySizeMsg{
		ContentW: contentW,
		ContentH: contentH,
		KittyRow: tabBarH + 2, // 1-indexed: past tab bar + top panel border
	}
	for i, o := range r.overlays {
		updated, cmd := o.Update(ovMsg)
		r.overlays[i] = updated.(ovpkg.Overlay)
		cmds = append(cmds, cmd)
	}

	return r, tea.Batch(cmds...)
}

func (r Root) handleOpenOverlay(m tuipkg.OpenOverlayMsg) (Root, tea.Cmd) {
	switch m.Kind {
	case tuipkg.OverlayVideoDetail:
		vd, cmd := ovpkg.NewVideoDetail(r.backend, r.keys, m.Video, r.cfg.CloseOnLinkOpen, r.cfg.CircularNav)
		r.overlays = append(r.overlays, vd)
		// Use the returned Root so the overlay receives its size via OverlaySizeMsg.
		var resizeCmd tea.Cmd
		r, resizeCmd = r.handleResize(r.width, r.height)
		return r, tea.Batch(cmd, resizeCmd)
	case tuipkg.OverlayAddToPlaylist:
		atp, cmd := ovpkg.NewAddToPlaylist(r.backend, r.keys, m.Video, r.cfg.CircularNav)
		r.overlays = append(r.overlays, atp)
		return r, cmd
	}
	return r, nil
}

func (r Root) handlePopOverlay() (Root, tea.Cmd) {
	n := len(r.overlays)
	if n == 0 {
		return r, nil
	}
	hadWidthReduction := r.overlays[n-1].WidthReduction() > 0
	r.overlays = r.overlays[:n-1]
	if hadWidthReduction {
		return r.handleResize(r.width, r.height)
	}
	return r, nil
}

func (r Root) handleKey(msg tea.KeyPressMsg) (Root, tea.Cmd) {
	if len(r.overlays) > 0 {
		return r.updateTopOverlay(msg)
	}
	if r.activeTab().InterceptsInput() {
		return r.updateActiveTab(msg)
	}

	if r.tabChordActive {
		r.tabChordActive = false
		if tabID, ok := r.tabChordKeys[msg.String()]; ok {
			return r.handleNavigate(tuipkg.NavigateMsg{Tab: tabID})
		}
		return r, nil // unrecognized chord key — discard
	}

	switch {
	case key.Matches(msg, r.keys.Quit):
		return r, tea.Quit
	case key.Matches(msg, r.keys.Tab):
		return r.cycleTab(+1)
	case key.Matches(msg, r.keys.ShiftTab):
		return r.cycleTab(-1)
	case key.Matches(msg, r.keys.TabChord):
		r.tabChordActive = true
		return r, nil
	}

	return r.updateActiveTab(msg)
}

func (r Root) handleNavigate(m tuipkg.NavigateMsg) (Root, tea.Cmd) {
	for i, t := range r.tabs {
		if t.ID() == m.Tab {
			r.activeIdx = i
			r.tabBar = r.tabBar.WithActive(i)
			break
		}
	}
	r.statusBar = r.statusBar.WithHints(r.tabHints())
	if m.Tab == tuipkg.TabSearch {
		if m.Query != "" {
			q := m.Query
			return r, func() tea.Msg { return tuipkg.SearchActivateMsg{Query: q} }
		}
		return r, func() tea.Msg { return tuipkg.SearchFocusInputMsg{} }
	}
	return r, nil
}

func (r Root) handleHideChannel(m tuipkg.HideChannelMsg) (Root, tea.Cmd) {
	ch := m.Channel
	return r, func() tea.Msg {
		_ = r.backend.HideRecVideo(context.Background(), ch.ID)
		return tuipkg.StatusMsg{Text: "Hidden: " + ch.Name}
	}
}

func (r Root) handleUnsubscribe(m tuipkg.UnsubscribeMsg) (Root, tea.Cmd) {
	ch := m.Channel
	return r, func() tea.Msg {
		if err := r.backend.Unsubscribe(context.Background(), ch); err != nil {
			return tuipkg.StatusMsg{Text: "unsubscribe: " + err.Error(), IsErr: true}
		}
		return tuipkg.StatusMsg{Text: "Unsubscribed: " + ch.Name}
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (r Root) activeTab() tuipkg.Tab { return r.tabs[r.activeIdx] }

func (r Root) updateActiveTab(msg tea.Msg) (Root, tea.Cmd) {
	updated, cmd := r.tabs[r.activeIdx].Update(msg)
	r.tabs[r.activeIdx] = updated.(tuipkg.Tab)
	r.statusBar = r.statusBar.WithHints(r.tabHints())
	return r, cmd
}

func (r Root) tabHints() string {
	hints := r.activeTab().ShortHelp()
	parts := make([]string, 0, len(hints))
	for _, b := range hints {
		h := b.Help()
		if h.Key != "" && h.Desc != "" {
			parts = append(parts, h.Key+": "+h.Desc)
		}
	}
	return strings.Join(parts, "  ")
}

func (r Root) updateTopOverlay(msg tea.Msg) (Root, tea.Cmd) {
	n := len(r.overlays)
	updated, cmd := r.overlays[n-1].Update(msg)
	r.overlays[n-1] = updated.(ovpkg.Overlay)
	return r, cmd
}

func (r Root) cycleTab(dir int) (Root, tea.Cmd) {
	n := len(r.tabs)
	r.activeIdx = ((r.activeIdx+dir)%n + n) % n
	r.tabBar = r.tabBar.WithActive(r.activeIdx)
	r.statusBar = r.statusBar.WithHints(r.tabHints())
	if r.activeTab().ID() == tuipkg.TabSearch {
		return r, func() tea.Msg { return tuipkg.SearchFocusInputMsg{} }
	}
	return r, nil
}

func (r Root) playCmd(id, fallbackURL, title string, audioOnly bool, eventType string) tea.Cmd {
	return func() tea.Msg {
		if r.player == nil {
			return tuipkg.StatusMsg{Text: "no video player found — install mpv or vlc", IsErr: true}
		}
		src, resolveErr := r.backend.ResolveSource(context.Background(), id, fallbackURL)
		if resolveErr != nil {
			return tuipkg.StatusMsg{Text: "resolve source: " + resolveErr.Error(), IsErr: true}
		}
		posMs, _ := r.backend.VideoPosition(context.Background(), id)
		pos := time.Duration(posMs) * time.Millisecond
		var sess *player.Session
		var launchErr error
		if audioOnly {
			sess, launchErr = r.player.LaunchAudio(src.URI, title, pos)
		} else {
			sess, launchErr = r.player.Launch(src.URI, title, pos)
		}
		if launchErr != nil {
			return tuipkg.StatusMsg{Text: "player: " + launchErr.Error(), IsErr: true}
		}
		_ = r.backend.AddHistory(context.Background(), id, eventType, "")
		// Periodic position saves — bound to this session so a concurrent
		// playback can't write its position under this video's ID.
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-sess.Done():
					return
				case <-ticker.C:
					if p, _ := sess.Position(); p > 0 {
						_ = r.backend.SaveVideoPosition(context.Background(), id, p.Milliseconds())
					}
				}
			}
		}()
		return playerStartedMsg{videoID: id, sess: sess, text: "Playing: " + render.Truncate(title, 60)}
	}
}

// playerWaitCmd blocks until the player process exits, saves the final position,
// then triggers a UI refresh so tabs show the updated playback progress.
func (r Root) playerWaitCmd(id string, sess *player.Session) tea.Cmd {
	return func() tea.Msg {
		<-sess.Done()
		if p, _ := sess.Position(); p > 0 {
			_ = r.backend.SaveVideoPosition(context.Background(), id, p.Milliseconds())
		}
		return tuipkg.RefreshPositionsMsg{}
	}
}

func (r Root) overlayWidthReduction() int {
	for _, o := range r.overlays {
		if red := o.WidthReduction(); red > 0 {
			return red
		}
	}
	return 0
}
