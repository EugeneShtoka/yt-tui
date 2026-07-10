package ui

import (
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/charmbracelet/bubbles/key"
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
	Subscribe     key.Binding
	RenameChannel key.Binding
	TagChannel    key.Binding
	AddList       key.Binding
	NewList       key.Binding
	Refresh       key.Binding
	ForceRefresh  key.Binding
	Help          key.Binding
	Filter        key.Binding // activate local filter input
	GotoBottom    key.Binding // go to last row
	GotoLine      key.Binding // go to line N (with number prefix)
	VideoInfo     key.Binding // open video details popup
	OpenLinks     key.Binding // open link list from video description
	OpenChapters  key.Binding // open chapter list from video metadata
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

	return keyMap{
		Up:       b(kb.Up, "up"),
		Down:     b(kb.Down, "down"),
		Left:     b(kb.Back, "back"),
		Right:    b(kb.Right, "right"),
		PageUp:   b(kb.PageUp, "page up"),
		PageDown: b(kb.PageDown, "page down"),

		Tab:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
		ShiftTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),

		Download:      b(kb.Download, "download video"),
		DownloadAudio: b(kb.DownloadAudio, "download audio"),
		Play:          b(kb.Play, "stream video"),
		PlayAudio:     b(kb.PlayAudio, "stream audio"),
		Delete:        b(kb.Delete, "delete"),
		HideVideo:     b(kb.HideVideo, "hide video"),
		HideChannel:   b(kb.HideChannel, "hide channel"),
		Unsubscribe:   b(kb.Unsubscribe, "unsubscribe"),
		CopyURL:       b(kb.CopyURL, "copy URL"),
		DrillDown:     b(kb.DrillDown, "open"),
		Escape:        b(kb.Close, "close"),
		Quit:          key.NewBinding(key.WithKeys(kb.Quit, "ctrl+c"), key.WithHelp(kb.Quit, "quit")),

		ToggleMode:    b(kb.ToggleMode, "toggle mode"),
		Subscribe:     b(kb.Subscribe, "subscribe"),
		RenameChannel: b(kb.RenameChannel, "rename channel"),
		TagChannel:    b(kb.TagChannel, "edit tags"),
		AddList:       b(kb.AddToPlaylist, "add to playlist"),
		NewList:       b(kb.NewPlaylist, "new playlist"),
		Refresh:       b(kb.Refresh, "refresh"),
		ForceRefresh:  b(kb.ForceRefresh, "force refresh"),
		Help:          b(kb.Help, "help"),
		Filter:        b(kb.Filter, "filter"),
		GotoBottom:    b(kb.GotoBottom, "go to bottom"),
		GotoLine:      b(kb.GotoLine, "go to line"),
		VideoInfo:     b(kb.VideoInfo, "video info"),
		OpenLinks:     b(kb.OpenLinks, "open links"),
		OpenChapters:  b(kb.OpenChapters, "chapters"),
	}
}
