package app

import (
	"context"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/command"
	"github.com/EugeneShtoka/yt-tui/internal/tui/component"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	ovpkg "github.com/EugeneShtoka/yt-tui/internal/tui/overlay"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/tab"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Root is the top-level BubbleTea model.
// It owns focus, size, global key routing, and the tab/overlay stack.
type Root struct {
	backend api.Backend
	cfg     *config.Config
	keys    keymap.KeyMap
	cmds    command.Registry

	width, height int

	tabBar    component.TabBar
	statusBar component.StatusBar

	tabs      []tuipkg.Tab
	activeIdx int

	overlays []ovpkg.Overlay
}

// New constructs the Root with the current tab set.
func New(backend api.Backend, cfg config.Config) Root {
	keys := keymap.Build(cfg.Keybindings)

	tabs := []tuipkg.Tab{
		tab.NewRecommended(backend, keys, cfg.CircularNav),
		tab.NewSubscriptions(backend, keys, cfg.CircularNav),
		tab.NewChannels(backend, keys, cfg.CircularNav, cfg.ChannelLatestCount),
		tab.NewPlaylists(backend, keys, cfg.CircularNav),
		tab.NewSearch(backend, keys, cfg.CircularNav),
		tab.NewDownloading(backend, keys, cfg.CircularNav),
		tab.NewHistory(backend, keys, cfg.CircularNav),
		tab.NewActivity(backend, keys, cfg.CircularNav),
		tab.NewLocal(backend, keys, cfg.CircularNav),
	}

	titles := make([]string, len(tabs))
	for i, t := range tabs {
		titles[i] = t.Title()
	}

	var cmds command.Registry
	cmds.Register(globalCommands(backend)...)

	right := keys.Help.Help().Key + ": help  " + keys.Quit.Help().Key + ": quit"

	return Root{
		backend:   backend,
		cfg:       &cfg,
		keys:      keys,
		cmds:      cmds,
		tabBar:    component.NewTabBar(titles),
		statusBar: component.NewStatusBar(right),
		tabs:      tabs,
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

	case tea.KeyMsg:
		return r.handleKey(m)

	case tuipkg.OpenOverlayMsg:
		return r.handleOpenOverlay(m)

	case ovpkg.PopOverlayMsg:
		return r.handlePopOverlay()

	case tuipkg.PlayVideoMsg:
		// TODO: wire player
		return r, func() tea.Msg {
			return tuipkg.StatusMsg{Text: "playback not yet wired in v2", IsErr: true}
		}

	case tuipkg.LaunchLocalVideoMsg:
		// TODO: wire player
		return r, func() tea.Msg {
			return tuipkg.StatusMsg{Text: "local playback not yet wired in v2", IsErr: true}
		}

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
		r, fwdCmd := r.updateActiveTab(m)
		return r, tea.Batch(navCmd, fwdCmd)

	case tuipkg.StatusMsg:
		sb, cmd := r.statusBar.Update(msg)
		r.statusBar = sb.(component.StatusBar)
		return r, cmd

	default:
		// Route to both the top overlay and the active tab so each can handle
		// its own private response messages (e.g. vdDetailsMsg → VideoDetail,
		// DownloadItemsChangedMsg → Downloading tab).
		var c1, c2 tea.Cmd
		if len(r.overlays) > 0 {
			r, c1 = r.updateTopOverlay(msg)
		}
		r, c2 = r.updateActiveTab(msg)
		return r, tea.Batch(c1, c2)
	}
}

func (r Root) View() string {
	tabBar := r.tabBar.View()
	status := r.statusBar.View()
	contentH := r.height - lipgloss.Height(tabBar) - lipgloss.Height(status)

	content := r.activeTab().View()

	var kittySeq string
	for _, o := range r.overlays {
		var kseq string
		content, kseq = o.Render(content, r.width, contentH)
		if kseq != "" {
			kittySeq = kseq
		}
	}

	if actual := lipgloss.Height(content); actual < contentH {
		content += strings.Repeat("\n", contentH-actual)
	}

	return lipgloss.JoinVertical(lipgloss.Left, tabBar, content, status) + kittySeq
}

// ── message handlers ──────────────────────────────────────────────────────────

func (r Root) handleResize(w, h int) (Root, tea.Cmd) {
	r.width, r.height = w, h
	r.tabBar = r.tabBar.WithWidth(w)
	r.statusBar = r.statusBar.WithWidth(w)

	tabBarH := lipgloss.Height(r.tabBar.View())
	statusH := lipgloss.Height(r.statusBar.View())
	contentH := h - tabBarH - statusH
	contentW := w - r.overlayWidthReduction()

	sizeMsg := tuipkg.ContentSizeMsg{Width: contentW, Height: contentH}
	var cmds []tea.Cmd
	for i, t := range r.tabs {
		updated, cmd := t.Update(sizeMsg)
		r.tabs[i] = updated.(tuipkg.Tab)
		cmds = append(cmds, cmd)
	}
	return r, tea.Batch(cmds...)
}

func (r Root) handleOpenOverlay(m tuipkg.OpenOverlayMsg) (Root, tea.Cmd) {
	switch m.Kind {
	case "video_detail":
		vd, cmd := ovpkg.NewVideoDetail(r.backend, r.keys, m.Video, r.cfg.CloseOnLinkOpen, r.cfg.CircularNav)
		r.overlays = append(r.overlays, vd)
		_, resizeCmd := r.handleResize(r.width, r.height)
		return r, tea.Batch(cmd, resizeCmd)
	case "add_to_playlist":
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

func (r Root) handleKey(msg tea.KeyMsg) (Root, tea.Cmd) {
	if len(r.overlays) > 0 {
		return r.updateTopOverlay(msg)
	}
	if r.activeTab().InterceptsInput() {
		return r.updateActiveTab(msg)
	}

	switch {
	case key.Matches(msg, r.keys.Quit):
		return r, tea.Quit
	case key.Matches(msg, r.keys.Tab):
		return r.cycleTab(+1)
	case key.Matches(msg, r.keys.ShiftTab):
		return r.cycleTab(-1)
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
	if m.Query != "" && m.Tab == tuipkg.TabSearch {
		q := m.Query
		return r, func() tea.Msg { return tuipkg.SearchActivateMsg{Query: q} }
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
	return r, cmd
}

func (r Root) updateTopOverlay(msg tea.Msg) (Root, tea.Cmd) {
	n := len(r.overlays)
	updated, cmd := r.overlays[n-1].Update(msg)
	r.overlays[n-1] = updated.(ovpkg.Overlay)
	return r, cmd
}

func (r Root) cycleTab(dir int) (Root, tea.Cmd) {
	n := len(r.tabs)
	r.activeIdx = ((r.activeIdx + dir) % n + n) % n
	r.tabBar = r.tabBar.WithActive(r.activeIdx)
	return r, nil
}

func (r Root) overlayWidthReduction() int {
	for _, o := range r.overlays {
		if red := o.WidthReduction(); red > 0 {
			return red
		}
	}
	return 0
}
