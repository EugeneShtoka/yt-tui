package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/EugeneShtoka/yt-tui/internal/config"
)

type keyMap struct {
	Up            key.Binding
	Down          key.Binding
	Left          key.Binding // back navigation: configurable Back keys + ← arrow
	Right         key.Binding
	PageUp        key.Binding
	PageDown      key.Binding
	Tab           key.Binding
	ShiftTab      key.Binding
	Download      key.Binding
	DownloadAudio key.Binding
	Play          key.Binding
	PlayAudio     key.Binding
	Delete        key.Binding
	HideVideo     key.Binding
	HideChannel   key.Binding
	CopyURL       key.Binding
	Unsubscribe   key.Binding
	DrillDown     key.Binding // open/select; plays video in video contexts
	Escape        key.Binding
	Quit          key.Binding
	ToggleMode    key.Binding
	WatchLater    key.Binding
	Subscribe     key.Binding
	AddList       key.Binding
	NewList       key.Binding
	Refresh       key.Binding
	ForceRefresh  key.Binding
	Help          key.Binding
	Filter        key.Binding // activate local filter input
	GotoBottom    key.Binding // go to last row (or Nth with number prefix)
}

func buildKeyMap(kb config.KeyBindings) keyMap {
	// b supports comma-separated keys: "p,enter" binds both p and enter.
	b := func(k, help string) key.Binding {
		parts := strings.Split(k, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return key.NewBinding(key.WithKeys(parts...), key.WithHelp(parts[0], help))
	}

	// Back: configurable keys ("h,backspace" default) always also includes ← arrow.
	backParts := strings.Split(kb.Back, ",")
	for i := range backParts {
		backParts[i] = strings.TrimSpace(backParts[i])
	}
	backKeys := append(backParts, "left")
	backHelp := backParts[0] + "/←/⌫"

	return keyMap{
		Up:       key.NewBinding(key.WithKeys("k", "up"),        key.WithHelp("k/↑", "up")),
		Down:     key.NewBinding(key.WithKeys("j", "down"),      key.WithHelp("j/↓", "down")),
		Left:     key.NewBinding(key.WithKeys(backKeys...),      key.WithHelp(backHelp, "back")),
		Right:    key.NewBinding(key.WithKeys("l", "right"),     key.WithHelp("l/→", "right")),
		PageUp:   key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("^u", "page up")),
		PageDown: key.NewBinding(key.WithKeys("ctrl+d", "pgdn"), key.WithHelp("^d", "page down")),

		Tab:      key.NewBinding(key.WithKeys("tab"),       key.WithHelp("tab", "next tab")),
		ShiftTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),

		Download:      b(kb.Download,      "download video"),
		DownloadAudio: b(kb.DownloadAudio, "download audio"),
		Play:          b(kb.Play,          "play video"),
		PlayAudio:     b(kb.PlayAudio,     "play audio"),
		Delete:        b(kb.Delete,        "delete"),
		HideVideo:     b(kb.HideVideo,     "hide video"),
		HideChannel:   b(kb.HideChannel,   "hide channel"),
		Unsubscribe:   b(kb.Unsubscribe,   "unsubscribe"),
		CopyURL:       b(kb.CopyURL,       "copy URL"),
		DrillDown:     b(kb.DrillDown,     "open"),
		Escape:        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Quit:          key.NewBinding(key.WithKeys(kb.Quit, "ctrl+c"), key.WithHelp(kb.Quit, "quit")),

		ToggleMode: b(kb.ToggleMode,    "toggle mode"),
		WatchLater: b(kb.WatchLater,    "watch later"),
		Subscribe:  b(kb.Subscribe,     "subscribe"),
		AddList:    b(kb.AddToPlaylist, "add to playlist"),
		NewList:    b(kb.NewPlaylist,   "new playlist"),
		Refresh:      b(kb.Refresh,      "refresh"),
		ForceRefresh: b(kb.ForceRefresh, "force refresh"),
		Help:         b(kb.Help,         "help"),
		Filter:     b(kb.Filter,        "filter"),
		GotoBottom: b(kb.GotoBottom,    "go to bottom"),
	}
}
