package ui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up         key.Binding
	Down       key.Binding
	Left       key.Binding
	Right      key.Binding
	PageUp     key.Binding
	PageDown   key.Binding
	Tab        key.Binding
	ShiftTab   key.Binding
	F2         key.Binding
	F3         key.Binding
	F4         key.Binding
	F5         key.Binding
	F6         key.Binding
	F7         key.Binding
	F8         key.Binding
	Download   key.Binding
	DownloadAudio key.Binding
	Play       key.Binding
	Delete     key.Binding
	Search     key.Binding
	Enter      key.Binding
	Escape     key.Binding
	Quit       key.Binding
	ToggleMode key.Binding
	WatchLater key.Binding
	AddList    key.Binding
	NewList    key.Binding
	Refresh    key.Binding
	Help       key.Binding
}

var keys = keyMap{
	Up:       key.NewBinding(key.WithKeys("k", "up"),       key.WithHelp("k/↑", "up")),
	Down:     key.NewBinding(key.WithKeys("j", "down"),     key.WithHelp("j/↓", "down")),
	Left:     key.NewBinding(key.WithKeys("h", "left"),     key.WithHelp("h/←", "left")),
	Right:    key.NewBinding(key.WithKeys("l", "right"),    key.WithHelp("l/→", "right")),
	PageUp:   key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("^u", "page up")),
	PageDown: key.NewBinding(key.WithKeys("ctrl+d", "pgdn"), key.WithHelp("^d", "page down")),

	Tab:      key.NewBinding(key.WithKeys("tab"),       key.WithHelp("tab", "next tab")),
	ShiftTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),

	F2: key.NewBinding(key.WithKeys("f2"), key.WithHelp("F2", "Recommended")),
	F3: key.NewBinding(key.WithKeys("f3"), key.WithHelp("F3", "Subscriptions")),
	F4: key.NewBinding(key.WithKeys("f4"), key.WithHelp("F4", "Playlists")),
	F5: key.NewBinding(key.WithKeys("f5"), key.WithHelp("F5", "Search")),
	F6: key.NewBinding(key.WithKeys("f6"), key.WithHelp("F6", "Downloading")),
	F7: key.NewBinding(key.WithKeys("f7"), key.WithHelp("F7", "Local")),
	F8: key.NewBinding(key.WithKeys("f8"), key.WithHelp("F8", "History")),

	Download:      key.NewBinding(key.WithKeys("s"),     key.WithHelp("s", "download video")),
	DownloadAudio: key.NewBinding(key.WithKeys("S"),     key.WithHelp("S", "download audio")),
	Play:          key.NewBinding(key.WithKeys("p"),     key.WithHelp("p", "play")),
	Delete:        key.NewBinding(key.WithKeys("d"),     key.WithHelp("d", "delete")),
	Search:        key.NewBinding(key.WithKeys("/"),     key.WithHelp("/", "search")),
	Enter:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	Escape:        key.NewBinding(key.WithKeys("esc"),   key.WithHelp("esc", "back")),
	Quit:          key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),

	ToggleMode: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "toggle mode")),
	WatchLater: key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "watch later")),
	AddList:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add to playlist")),
	NewList:    key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new playlist")),
	Refresh:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
}
