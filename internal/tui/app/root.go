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
	cfg     config.Config
	keys    keymap.KeyMap
	cmds    command.Registry

	width, height int

	tabBar    component.TabBar
	statusBar component.StatusBar

	tabs      []tuipkg.Tab
	activeIdx int
}

// New constructs the Root with the current tab set.
func New(backend api.Backend, cfg config.Config) Root {
	keys := keymap.Build(cfg.Keybindings)

	tabs := []tuipkg.Tab{
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
		cfg:       cfg,
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

	case tuipkg.PlayVideoMsg:
		// TODO: wire player in Phase 4 once tabs reach parity.
		return r, func() tea.Msg {
			return tuipkg.StatusMsg{Text: "playback not yet wired in v2", IsErr: true}
		}

	case tuipkg.LaunchLocalVideoMsg:
		// TODO: wire player in Phase 4 once tabs reach parity.
		return r, func() tea.Msg {
			return tuipkg.StatusMsg{Text: "local playback not yet wired in v2", IsErr: true}
		}

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

	case tuipkg.NavigateToChannelMsg:
		return r.handleNavigate(tuipkg.NavigateMsg{Tab: tuipkg.TabChannels})

	case tuipkg.NavigateToPlaylistMsg:
		return r.handleNavigate(tuipkg.NavigateMsg{Tab: tuipkg.TabPlaylists})

	case tuipkg.StatusMsg:
		sb, cmd := r.statusBar.Update(msg)
		r.statusBar = sb.(component.StatusBar)
		return r, cmd

	default:
		updated, cmd := r.tabs[r.activeIdx].Update(msg)
		r.tabs[r.activeIdx] = updated.(tuipkg.Tab)
		return r, cmd
	}
}

func (r Root) View() string {
	tabBar := r.tabBar.View()
	status := r.statusBar.View()
	contentH := r.height - lipgloss.Height(tabBar) - lipgloss.Height(status)

	content := r.tabs[r.activeIdx].View()

	if actual := lipgloss.Height(content); actual < contentH {
		content += strings.Repeat("\n", contentH-actual)
	}

	return lipgloss.JoinVertical(lipgloss.Left, tabBar, content, status)
}

// ── message handlers ──────────────────────────────────────────────────────────

func (r Root) handleResize(w, h int) (Root, tea.Cmd) {
	r.width, r.height = w, h
	r.tabBar = r.tabBar.WithWidth(w)
	r.statusBar = r.statusBar.WithWidth(w)

	tabBarH := lipgloss.Height(r.tabBar.View())
	statusH := lipgloss.Height(r.statusBar.View())
	contentH := h - tabBarH - statusH

	sizeMsg := tuipkg.ContentSizeMsg{Width: w, Height: contentH}
	var cmds []tea.Cmd
	for i, t := range r.tabs {
		updated, cmd := t.Update(sizeMsg)
		r.tabs[i] = updated.(tuipkg.Tab)
		cmds = append(cmds, cmd)
	}
	return r, tea.Batch(cmds...)
}

func (r Root) handleKey(msg tea.KeyMsg) (Root, tea.Cmd) {
	switch {
	case key.Matches(msg, r.keys.Quit):
		return r, tea.Quit
	case key.Matches(msg, r.keys.Tab):
		return r.cycleTab(+1)
	case key.Matches(msg, r.keys.ShiftTab):
		return r.cycleTab(-1)
	}

	updated, cmd := r.tabs[r.activeIdx].Update(msg)
	r.tabs[r.activeIdx] = updated.(tuipkg.Tab)
	return r, cmd
}

func (r Root) cycleTab(dir int) (Root, tea.Cmd) {
	n := len(r.tabs)
	r.activeIdx = ((r.activeIdx + dir) % n + n) % n
	r.tabBar = r.tabBar.WithActive(r.activeIdx)
	return r, nil
}

func (r Root) handleNavigate(m tuipkg.NavigateMsg) (Root, tea.Cmd) {
	for i, t := range r.tabs {
		if t.ID() == m.Tab {
			r.activeIdx = i
			r.tabBar = r.tabBar.WithActive(i)
			break
		}
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
