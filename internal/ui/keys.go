package ui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/EugeneShtoka/yt-tui/internal/config"
)

type keyMap struct {
	Up            key.Binding
	Down          key.Binding
	Left          key.Binding
	Right         key.Binding
	PageUp        key.Binding
	PageDown      key.Binding
	Tab           key.Binding
	ShiftTab      key.Binding
	Download      key.Binding
	DownloadAudio key.Binding
	Play          key.Binding
	Delete        key.Binding
	HideChannel   key.Binding
	CopyURL       key.Binding
	Enter         key.Binding
	Escape        key.Binding
	Quit          key.Binding
	ToggleMode    key.Binding
	WatchLater    key.Binding
	AddList       key.Binding
	NewList       key.Binding
	Refresh       key.Binding
	Help          key.Binding
}

func buildKeyMap(kb config.KeyBindings) keyMap {
	b := func(k, help string) key.Binding {
		return key.NewBinding(key.WithKeys(k), key.WithHelp(k, help))
	}
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("k", "up"),        key.WithHelp("k/↑", "up")),
		Down:     key.NewBinding(key.WithKeys("j", "down"),      key.WithHelp("j/↓", "down")),
		Left:     key.NewBinding(key.WithKeys("h", "left"),      key.WithHelp("h/←", "left")),
		Right:    key.NewBinding(key.WithKeys("l", "right"),     key.WithHelp("l/→", "right")),
		PageUp:   key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("^u", "page up")),
		PageDown: key.NewBinding(key.WithKeys("ctrl+d", "pgdn"), key.WithHelp("^d", "page down")),

		Tab:      key.NewBinding(key.WithKeys("tab"),       key.WithHelp("tab", "next tab")),
		ShiftTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),

		Download:      b(kb.Download,      "download video"),
		DownloadAudio: b(kb.DownloadAudio, "download audio"),
		Play:          b(kb.Play,          "play"),
		Delete:        b(kb.Delete,        "delete"),
		HideChannel:   b(kb.HideChannel,   "hide channel"),
		CopyURL:       b(kb.CopyURL,       "copy URL"),
		Enter:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Escape:        key.NewBinding(key.WithKeys("esc"),   key.WithHelp("esc", "back")),
		Quit:          key.NewBinding(key.WithKeys(kb.Quit, "ctrl+c"), key.WithHelp(kb.Quit, "quit")),

		ToggleMode: b(kb.ToggleMode,    "toggle mode"),
		WatchLater: b(kb.WatchLater,    "watch later"),
		AddList:    b(kb.AddToPlaylist, "add to playlist"),
		NewList:    b(kb.NewPlaylist,   "new playlist"),
		Refresh:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Help:       b(kb.Help, "help"),
	}
}
