package config

import "reflect"

// The json tags on the keybinding/panel structs mirror their toml tags so the
// portable config profile (carried in an export bundle as opaque JSON) uses the
// same snake_case names on disk. They are inert for the TOML encoder, which
// config is otherwise persisted with.
type SubscribeKeys struct {
	Remote string `toml:"remote" json:"remote"`
	Local  string `toml:"local" json:"local"`
}

type PlaylistKeys struct {
	Remote string `toml:"remote" json:"remote"`
	Local  string `toml:"local" json:"local"`
}

type SortKeys struct {
	Date        string `toml:"date" json:"date"`
	Views       string `toml:"views" json:"views"`
	Name        string `toml:"name" json:"name"`
	Channel     string `toml:"channel" json:"channel"`
	Duration    string `toml:"duration" json:"duration"`
	Subscribers string `toml:"subscribers" json:"subscribers"`
	Tags        string `toml:"tags" json:"tags"`
	Size        string `toml:"size" json:"size"`
}

type KeyBindings struct {
	Download       string `toml:"download" json:"download"`
	DownloadAudio  string `toml:"download_audio" json:"download_audio"`
	Delete         string `toml:"delete" json:"delete"`
	Play           string `toml:"play" json:"play"`
	PlayAudio      string `toml:"play_audio" json:"play_audio"`
	HideVideo      string `toml:"hide_video" json:"hide_video"`
	HideChannel    string `toml:"hide_channel" json:"hide_channel"`
	CopyURL        string `toml:"copy_url" json:"copy_url"`
	OpenLinks      string `toml:"open_links" json:"open_links"`
	OpenChapters   string `toml:"open_chapters" json:"open_chapters"`
	OpenTranscript string `toml:"open_transcript" json:"open_transcript"`
	CopyTranscript string `toml:"copy_transcript" json:"copy_transcript"` // copy the full transcript (inside the transcript popup)
	NextChapter    string `toml:"next_chapter" json:"next_chapter"`       // jump to next chapter (inside the transcript popup)
	PrevChapter    string `toml:"prev_chapter" json:"prev_chapter"`       // jump to previous chapter (inside the transcript popup)
	AddToPlaylist  string `toml:"add_to_playlist" json:"add_to_playlist"`
	NewPlaylist    string `toml:"new_playlist" json:"new_playlist"`
	WatchLater     string `toml:"watch_later" json:"watch_later"` // add focused video to Watch Later (YT WL when authed, else a local list)
	ToggleMode     string `toml:"toggle_mode" json:"toggle_mode"`
	Subscribe      string `toml:"subscribe" json:"subscribe"`
	Unsubscribe    string `toml:"unsubscribe" json:"unsubscribe"`
	Block          string `toml:"block" json:"block"` // toggle block/unblock on the selected channel
	RenameChannel  string `toml:"rename_channel" json:"rename_channel"`
	TagChannel     string `toml:"tag_channel" json:"tag_channel"`
	PanelMode      string `toml:"panel_mode" json:"panel_mode"`         // open the view/mode picker for the current panel
	Export         string `toml:"export" json:"export"`                 // export all app data to a bundle file
	Import         string `toml:"import" json:"import"`                 // import a bundle file (opens the preview overlay)
	CommandPrompt  string `toml:"command_prompt" json:"command_prompt"` // open the command palette (free-text ":" input)
	Help           string `toml:"help" json:"help"`
	Quit           string `toml:"quit" json:"quit"`
	Close          string `toml:"close" json:"close"` // close/cancel overlays (always includes esc)

	Refresh      string `toml:"refresh" json:"refresh"`             // re-query / latest fetch
	ForceRefresh string `toml:"force_refresh" json:"force_refresh"` // full fetch for all channels
	VideoInfo    string `toml:"video_info" json:"video_info"`       // open video details popup
	FocusSwitch  string `toml:"focus_switch" json:"focus_switch"`   // toggle focus between tab and info panel

	Up         string `toml:"up" json:"up"`                   // move cursor up (always includes ↑ arrow)
	Down       string `toml:"down" json:"down"`               // move cursor down (always includes ↓ arrow)
	Right      string `toml:"right" json:"right"`             // move right / forward (always includes → arrow)
	PageUp     string `toml:"page_up" json:"page_up"`         // page up (always includes pgup)
	PageDown   string `toml:"page_down" json:"page_down"`     // page down (always includes pgdn)
	DrillDown  string `toml:"drill_down" json:"drill_down"`   // open/select; plays video in video contexts
	Back       string `toml:"back" json:"back"`               // go back / close pane (always includes ← arrow)
	Filter     string `toml:"filter" json:"filter"`           // activate local filter input
	TabChord   string `toml:"tab_chord" json:"tab_chord"`     // first key of tab-switch chord
	SortChord  string `toml:"sort_chord" json:"sort_chord"`   // first key of sort chord
	GotoPrefix string `toml:"goto_prefix" json:"goto_prefix"` // first key of goto-top chord (press twice)
	GotoBottom string `toml:"goto_bottom" json:"goto_bottom"` // go to last row
	GotoLine   string `toml:"goto_line" json:"goto_line"`     // go to line N (requires number prefix; defaults to same as goto_bottom)

	SortKeys      SortKeys      `toml:"sort_keys" json:"sort_keys"`
	SubscribeKeys SubscribeKeys `toml:"subscribe_keys" json:"subscribe_keys"`
	PlaylistKeys  PlaylistKeys  `toml:"playlist_keys" json:"playlist_keys"`

	// TabKeys maps a tab-chord hotkey (the key pressed after TabChord) to a
	// panel name (see ClientConfig.Panels). Keyed by hotkey so a duplicate is
	// structurally impossible; referencing panels by name (not index) survives
	// panel reordering. Empty falls back to defaultTabKeys(). A positional
	// TabChord+1..9 fallback (Nth panel) is added by the TUI regardless.
	TabKeys map[string]string `toml:"tab_keys" json:"tab_keys"`
}

func defaultKeyBindings() KeyBindings { //nolint:funlen // flat default table: one field assignment per binding, no logic to extract
	return KeyBindings{
		Download:       "d",
		DownloadAudio:  "D",
		Delete:         "x",
		Play:           "p",
		PlayAudio:      "P",
		HideVideo:      "b",
		HideChannel:    "B",
		CopyURL:        "y",
		OpenLinks:      "L",
		OpenChapters:   "C",
		OpenTranscript: "e",
		CopyTranscript: "y",
		NextChapter:    "]",
		PrevChapter:    "[",
		AddToPlaylist:  "a",
		NewPlaylist:    "n",
		WatchLater:     "w",
		ToggleMode:     "m",
		Subscribe:      "S",
		Unsubscribe:    "u",
		Block:          "X",
		RenameChannel:  "A",
		TagChannel:     "T",
		PanelMode:      "M",
		Export:         "E",
		Import:         "I",
		CommandPrompt:  ":",
		Help:           "?",
		Quit:           "q",
		Close:          "esc",

		Refresh:      "r",
		ForceRefresh: "R",
		VideoInfo:    "i",
		FocusSwitch:  "f",

		Up:         "k,up",
		Down:       "j,down",
		Right:      "l,right",
		PageUp:     "ctrl+u,pgup",
		PageDown:   "ctrl+d,pgdn",
		DrillDown:  "enter",
		Back:       "h,backspace,left",
		Filter:     "/",
		TabChord:   "t",
		SortChord:  "s",
		GotoPrefix: "g",
		GotoBottom: "G",
		GotoLine:   "G",

		SubscribeKeys: SubscribeKeys{Remote: "r", Local: "l"},
		PlaylistKeys:  PlaylistKeys{Remote: "r", Local: "l"},
		SortKeys: SortKeys{
			Date:        "d",
			Views:       "v",
			Name:        "n",
			Channel:     "c",
			Duration:    "D",
			Subscribers: "s",
			Tags:        "t",
			Size:        "z",
		},
		TabKeys: defaultTabKeys(),
	}
}

// defaultTabKeys reproduces the built-in tab hotkeys (hotkey → panel name),
// matching the names in DefaultPanels so the default layout behaves exactly as
// before the panel/map migration.
func defaultTabKeys() map[string]string {
	return map[string]string{
		"f": "feed",
		"c": "channels",
		"t": "tags",
		"p": "playlists",
		"s": "search",
		"d": "downloading",
		"l": "local",
		"h": "history",
		"a": "activity",
	}
}

// fillDefaults ensures no keybinding is empty (happens when config was generated
// before a new binding was added — TOML zeroes nested struct fields not in the file).
func (kb *KeyBindings) fillDefaults() {
	d := defaultKeyBindings()
	fillStringDefaults(reflect.ValueOf(kb).Elem(), reflect.ValueOf(d))
	// TabKeys is a map, not a string/struct field, so fillStringDefaults skips
	// it; seed the built-in hotkeys when the config omitted the table entirely.
	if len(kb.TabKeys) == 0 {
		kb.TabKeys = defaultTabKeys()
	}
}

// fillStringDefaults recursively fills empty string fields in target from defaults.
// Only processes string and struct kinds — safe for KeyBindings and its nested types.
func fillStringDefaults(target, defaults reflect.Value) {
	for i := 0; i < target.NumField(); i++ {
		tv := target.Field(i)
		dv := defaults.Field(i)
		switch tv.Kind() {
		case reflect.String:
			if tv.String() == "" {
				tv.Set(dv)
			}
		case reflect.Struct:
			fillStringDefaults(tv, dv)
		}
	}
}
