package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

type SubscribeKeys struct {
	Remote string `toml:"remote"`
	Local  string `toml:"local"`
}

type PlaylistKeys struct {
	Remote string `toml:"remote"`
	Local  string `toml:"local"`
}

type SortKeys struct {
	Date        string `toml:"date"`
	Views       string `toml:"views"`
	Name        string `toml:"name"`
	Channel     string `toml:"channel"`
	Duration    string `toml:"duration"`
	Subscribers string `toml:"subscribers"`
	Tags        string `toml:"tags"`
}

type TabKeys struct {
	Recommended   string `toml:"recommended"`
	Subscriptions string `toml:"subscriptions"`
	Channels      string `toml:"channels"`
	Playlists     string `toml:"playlists"`
	Search        string `toml:"search"`
	Downloading   string `toml:"downloading"`
	Local         string `toml:"local"`
	History       string `toml:"history"`
	Activity      string `toml:"activity"`
}

type KeyBindings struct {
	Download      string `toml:"download"`
	DownloadAudio string `toml:"download_audio"`
	Delete        string `toml:"delete"`
	Play          string `toml:"play"`
	PlayAudio     string `toml:"play_audio"`
	HideVideo     string `toml:"hide_video"`
	HideChannel   string `toml:"hide_channel"`
	CopyURL       string `toml:"copy_url"`
	OpenLinks     string `toml:"open_links"`
	OpenChapters  string `toml:"open_chapters"`
	AddToPlaylist string `toml:"add_to_playlist"`
	NewPlaylist   string `toml:"new_playlist"`
	ToggleMode    string `toml:"toggle_mode"`
	Subscribe     string `toml:"subscribe"`
	Unsubscribe   string `toml:"unsubscribe"`
	RenameChannel string `toml:"rename_channel"`
	TagChannel    string `toml:"tag_channel"`
	Help          string `toml:"help"`
	Quit          string `toml:"quit"`
	Close         string `toml:"close"` // close/cancel overlays (always includes esc)

	Refresh      string `toml:"refresh"`       // re-query / latest fetch
	ForceRefresh string `toml:"force_refresh"` // full fetch for all channels
	VideoInfo    string `toml:"video_info"`    // open video details popup

	Up         string `toml:"up"`          // move cursor up (always includes ↑ arrow)
	Down       string `toml:"down"`        // move cursor down (always includes ↓ arrow)
	Right      string `toml:"right"`       // move right / forward (always includes → arrow)
	PageUp     string `toml:"page_up"`     // page up (always includes pgup)
	PageDown   string `toml:"page_down"`   // page down (always includes pgdn)
	DrillDown  string `toml:"drill_down"`  // open/select; plays video in video contexts
	Back       string `toml:"back"`        // go back / close pane (always includes ← arrow)
	Filter     string `toml:"filter"`      // activate local filter input
	TabChord   string `toml:"tab_chord"`   // first key of tab-switch chord
	SortChord  string `toml:"sort_chord"`  // first key of sort chord
	GotoPrefix string `toml:"goto_prefix"` // first key of goto-top chord (press twice)
	GotoBottom string `toml:"goto_bottom"` // go to last row
	GotoLine   string `toml:"goto_line"`   // go to line N (requires number prefix; defaults to same as goto_bottom)

	SortKeys      SortKeys      `toml:"sort_keys"`
	TabKeys       TabKeys       `toml:"tab_keys"`
	SubscribeKeys SubscribeKeys `toml:"subscribe_keys"`
	PlaylistKeys  PlaylistKeys  `toml:"playlist_keys"`
}

func defaultKeyBindings() KeyBindings {
	return KeyBindings{
		Download:      "d",
		DownloadAudio: "D",
		Delete:        "x",
		Play:          "p",
		PlayAudio:     "P",
		HideVideo:     "b",
		HideChannel:   "B",
		CopyURL:       "y",
		OpenLinks:     "L",
		OpenChapters:  "C",
		AddToPlaylist: "a",
		NewPlaylist:   "n",
		ToggleMode:    "m",
		Subscribe:     "S",
		Unsubscribe:   "u",
		RenameChannel: "A",
		TagChannel:    "T",
		Help:          "?",
		Quit:          "q",
		Close:         "esc",

		Refresh:      "r",
		ForceRefresh: "R",
		VideoInfo:    "i",

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
		},
		TabKeys: TabKeys{
			Recommended:   "r",
			Subscriptions: "s",
			Channels:      "c",
			Playlists:     "p",
			Search:        "S",
			Downloading:   "d",
			Local:         "l",
			History:       "h",
			Activity:      "a",
		},
	}
}

// fillDefaults ensures no keybinding is empty (happens when config was generated
// before a new binding was added — TOML zeroes nested struct fields not in the file).
func (kb *KeyBindings) fillDefaults() {
	d := defaultKeyBindings()
	fillStringDefaults(reflect.ValueOf(kb).Elem(), reflect.ValueOf(d))
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

type BlacklistedChannel struct {
	ID   string `toml:"id,omitempty"` // YouTube channel ID — stable primary match key
	Name string `toml:"name"`         // human-readable label; fallback match when ID absent
}

// DaemonConfig holds settings owned by the headless yt-tuid daemon: download
// location, browser cookie source, yt-dlp fetch parameters, and feed filters.
// These fields are irrelevant when the TUI connects to a remote daemon.
type DaemonConfig struct {
	DownloadDir                string               `toml:"download_dir"`
	Browser                    string               `toml:"browser"`
	MaxDownloads               int                  `toml:"max_concurrent_downloads"`
	SponsorBlock               bool                 `toml:"sponsorblock"`
	SponsorBlockCats           []string             `toml:"sponsorblock_categories"`
	AudioFormat                string               `toml:"audio_format"`
	RecommendedMaxAgeDays      int                  `toml:"recommended_max_age_days"`
	RecommendedMinDurationSecs int                  `toml:"recommended_min_duration_secs"`
	RecommendedMinViews        int                  `toml:"recommended_min_views"`
	RecommendedFetchCount      int                  `toml:"recommended_fetch_count"`
	RecommendedMaxPages        int                  `toml:"recommended_max_pages"`
	ChannelLatestCount         int                  `toml:"channel_latest_count"`
	ChannelStrikes             int                  `toml:"channel_strikes"`
	StripEmojis                bool                 `toml:"strip_emojis"`
	Subtitles                  bool                 `toml:"subtitles"`
	SubtitleLangs              []string             `toml:"subtitle_langs"`
	BlacklistedChannels        []BlacklistedChannel `toml:"blacklisted_channels"`
}

// ClientConfig holds settings used only by the yt-tui TUI client: local player
// binary, visual theme, tab layout, UI preferences, and key bindings.
// These fields are irrelevant on a headless daemon host.
type ClientConfig struct {
	Player          string      `toml:"player"`
	PlayerBackend   string      `toml:"player_backend"`
	Theme           string      `toml:"theme,omitempty"`
	Tabs            []string    `toml:"tabs"`
	HintMode        string      `toml:"hint_mode"` // "full" | "minimal" | "none"
	CloseOnLinkOpen bool        `toml:"close_on_link_open"`
	CircularNav     bool        `toml:"circular_nav"`
	Keybindings     KeyBindings `toml:"keybindings"`
}

// Config is the unified configuration used in single-binary (InProc) mode.
// It embeds DaemonConfig and ClientConfig so the existing flat config.toml
// layout is preserved — no migration required for existing installations.
// When the TUI runs with --connect, only ClientConfig fields are relevant;
// when yt-tuid runs headlessly, only DaemonConfig fields are relevant.
type Config struct {
	DaemonConfig
	ClientConfig
	DataDir    string `toml:"-"`
	ConfigFile string `toml:"-"`

	// mu guards mutable config fields and serializes file writes.
	// Unexported fields are ignored by the TOML encoder.
	mu sync.Mutex
	// saveReq coalesces async save requests into a single background write.
	saveReq chan struct{}
}

var DefaultTabs = []string{
	"recommended", "subscriptions", "channels", "playlists",
	"search", "downloading", "local", "history", "activity",
}

func defaultConfig() *Config {
	return &Config{
		DaemonConfig: DaemonConfig{
			DownloadDir:           filepath.Join(os.Getenv("HOME"), "Videos", "yt-tui"),
			Browser:               "vivaldi+gnomekeyring",
			MaxDownloads:          3,
			SponsorBlock:          true,
			SponsorBlockCats:      []string{"sponsor", "selfpromo", "interaction"},
			AudioFormat:           "mp3",
			RecommendedMaxAgeDays: 7,
			RecommendedFetchCount: 150,
			RecommendedMaxPages:   3,
			ChannelLatestCount:    3,
			ChannelStrikes:        2,
			StripEmojis:           true,
			Subtitles:             true,
			SubtitleLangs:         []string{"en.*"},
		},
		ClientConfig: ClientConfig{
			Player:          "mpv",
			PlayerBackend:   "mpris",
			Tabs:            DefaultTabs,
			HintMode:        "full",
			CloseOnLinkOpen: true,
			Keybindings:     defaultKeyBindings(),
		},
	}
}

func Load() (*Config, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("Load: %w", err)
	}
	appDir := filepath.Join(configDir, "yt-tui")
	if mkdirErr := os.MkdirAll(appDir, 0750); mkdirErr != nil {
		return nil, fmt.Errorf("Load mkdir: %w", mkdirErr)
	}

	cfg := defaultConfig()

	cfgFile := filepath.Join(appDir, "config.toml")
	data, err := os.ReadFile(cfgFile)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("Load read: %w", err)
	}
	if err == nil {
		if err := toml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("Load unmarshal: %w", err)
		}
		cfg.Keybindings.fillDefaults()
		if cfg.HintMode == "" {
			cfg.HintMode = "full"
		}
		if cfg.ChannelLatestCount <= 0 {
			cfg.ChannelLatestCount = 3
		}
		if cfg.ChannelStrikes <= 0 {
			cfg.ChannelStrikes = 2
		}
		if len(cfg.SubtitleLangs) == 0 {
			cfg.SubtitleLangs = []string{"en.*"}
		}
	}
	// Always re-save so any missing/new keybindings appear in the file.
	if err := cfg.save(cfgFile); err != nil {
		return nil, err
	}

	cfg.DataDir = appDir
	cfg.ConfigFile = cfgFile

	// Start the background save worker now that ConfigFile is known. All saves
	// after startup go through this single goroutine (via SaveAsync) or Save,
	// both serialized by cfg.mu.
	cfg.saveReq = make(chan struct{}, 1)
	go cfg.saveWorker()

	if len(cfg.DownloadDir) > 1 && cfg.DownloadDir[:2] == "~/" {
		cfg.DownloadDir = filepath.Join(os.Getenv("HOME"), cfg.DownloadDir[2:])
	}

	if err := os.MkdirAll(cfg.DownloadDir, 0750); err != nil {
		return nil, fmt.Errorf("Load mkdir download: %w", err)
	}

	return cfg, nil
}

// Save writes the config to disk, serialized against concurrent saves and
// mutations via mu. Safe to call from multiple goroutines.
func (c *Config) Save() error {
	path := c.ConfigFile
	if path == "" {
		configDir, _ := os.UserConfigDir()
		path = filepath.Join(configDir, "yt-tui", "config.toml")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.save(path)
}

// SaveAsync requests a background save without blocking the caller. Multiple
// requests arriving before the worker runs are coalesced into a single write.
// Falls back to a synchronous save if the worker was never started.
func (c *Config) SaveAsync() {
	if c.saveReq == nil {
		go func() { _ = c.Save() }()
		return
	}
	select {
	case c.saveReq <- struct{}{}:
	default: // a save is already pending — coalesce
	}
}

// saveWorker drains coalesced save requests one at a time.
func (c *Config) saveWorker() {
	for range c.saveReq {
		_ = c.Save()
	}
}

// save atomically writes the config: encode to a temp file in the same
// directory, then rename over the target. A crash or encode error leaves the
// existing file untouched. Callers must hold c.mu (except single-threaded
// startup in Load).
func (c *Config) save(path string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("save mktemp: %w", err)
	}
	tmpName := tmp.Name()
	if err := toml.NewEncoder(tmp).Encode(c); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("save encode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("save close: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("save rename: %w", err)
	}
	return nil
}

func (c *DaemonConfig) SubtitleLangsArg() string {
	if len(c.SubtitleLangs) == 0 {
		return ""
	}
	return strings.Join(c.SubtitleLangs, ",")
}

func (c *DaemonConfig) SponsorBlockArg() string {
	if !c.SponsorBlock || len(c.SponsorBlockCats) == 0 {
		return ""
	}
	out := c.SponsorBlockCats[0]
	for _, cat := range c.SponsorBlockCats[1:] {
		out += "," + cat
	}
	return out
}

// SetBlacklistID back-fills the channel ID on an existing name-only blacklist
// entry. Locked so it can't race a concurrent save encoding the slice.
func (c *Config) SetBlacklistID(idx int, id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if idx >= 0 && idx < len(c.BlacklistedChannels) {
		c.BlacklistedChannels[idx].ID = id
	}
}

// AddBlacklistedChannel appends a channel to the blacklist if not already present.
// Deduplicates by ID first, then by name. Locked against concurrent saves.
func (c *Config) AddBlacklistedChannel(id, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, bl := range c.BlacklistedChannels {
		if id != "" && bl.ID == id {
			if bl.Name == "" {
				c.BlacklistedChannels[i].Name = name
			}
			return
		}
		if bl.ID == "" && strings.EqualFold(bl.Name, name) {
			c.BlacklistedChannels[i].ID = id
			return
		}
	}
	c.BlacklistedChannels = append(c.BlacklistedChannels, BlacklistedChannel{ID: id, Name: name})
}
