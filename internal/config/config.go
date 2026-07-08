package config

import (
	"os"
	"path/filepath"
	"strings"

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
	AddToPlaylist string `toml:"add_to_playlist"`
	NewPlaylist   string `toml:"new_playlist"`
	ToggleMode    string `toml:"toggle_mode"`
	Subscribe      string `toml:"subscribe"`
	Unsubscribe    string `toml:"unsubscribe"`
	RenameChannel  string `toml:"rename_channel"`
	TagChannel     string `toml:"tag_channel"`
	Help           string `toml:"help"`
	Quit          string `toml:"quit"`

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
		AddToPlaylist: "a",
		NewPlaylist:   "n",
		ToggleMode:    "m",
		Subscribe:     "S",
		Unsubscribe:   "u",
		RenameChannel: "A",
		TagChannel:    "T",
		Help:          "?",
		Quit:          "q",

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
		},
	}
}

// fillDefaults ensures no keybinding is empty (happens when config was generated
// before a new binding was added — TOML zeroes nested struct fields not in the file).
func (kb *KeyBindings) fillDefaults() {
	d := defaultKeyBindings()
	if kb.Download == ""      { kb.Download = d.Download }
	if kb.DownloadAudio == "" { kb.DownloadAudio = d.DownloadAudio }
	if kb.Delete == ""        { kb.Delete = d.Delete }
	if kb.Play == ""          { kb.Play = d.Play }
	if kb.PlayAudio == ""     { kb.PlayAudio = d.PlayAudio }
	if kb.HideVideo == ""     { kb.HideVideo = d.HideVideo }
	if kb.HideChannel == ""   { kb.HideChannel = d.HideChannel }
	if kb.CopyURL == ""       { kb.CopyURL = d.CopyURL }
	if kb.OpenLinks == ""     { kb.OpenLinks = d.OpenLinks }
	if kb.AddToPlaylist == "" { kb.AddToPlaylist = d.AddToPlaylist }
	if kb.NewPlaylist == ""   { kb.NewPlaylist = d.NewPlaylist }
	if kb.ToggleMode == ""    { kb.ToggleMode = d.ToggleMode }
	if kb.Subscribe == ""      { kb.Subscribe = d.Subscribe }
	if kb.Unsubscribe == ""    { kb.Unsubscribe = d.Unsubscribe }
	if kb.RenameChannel == ""  { kb.RenameChannel = d.RenameChannel }
	if kb.TagChannel == ""     { kb.TagChannel = d.TagChannel }
	if kb.Help == ""           { kb.Help = d.Help }
	if kb.Quit == ""          { kb.Quit = d.Quit }

	if kb.Refresh == ""      { kb.Refresh = d.Refresh }
	if kb.ForceRefresh == "" { kb.ForceRefresh = d.ForceRefresh }
	if kb.VideoInfo == ""    { kb.VideoInfo = d.VideoInfo }

	if kb.Up == ""         { kb.Up = d.Up }
	if kb.Down == ""       { kb.Down = d.Down }
	if kb.Right == ""      { kb.Right = d.Right }
	if kb.PageUp == ""     { kb.PageUp = d.PageUp }
	if kb.PageDown == ""   { kb.PageDown = d.PageDown }
	if kb.DrillDown == ""  { kb.DrillDown = d.DrillDown }
	if kb.Back == ""       { kb.Back = d.Back }
	if kb.Filter == ""     { kb.Filter = d.Filter }
	if kb.TabChord == ""   { kb.TabChord = d.TabChord }
	if kb.SortChord == ""  { kb.SortChord = d.SortChord }
	if kb.GotoPrefix == "" { kb.GotoPrefix = d.GotoPrefix }
	if kb.GotoBottom == "" { kb.GotoBottom = d.GotoBottom }
	if kb.GotoLine   == "" { kb.GotoLine   = d.GotoLine }

	if kb.SubscribeKeys.Remote == "" { kb.SubscribeKeys.Remote = d.SubscribeKeys.Remote }
	if kb.SubscribeKeys.Local == ""  { kb.SubscribeKeys.Local = d.SubscribeKeys.Local }
	if kb.PlaylistKeys.Remote == ""  { kb.PlaylistKeys.Remote = d.PlaylistKeys.Remote }
	if kb.PlaylistKeys.Local == ""   { kb.PlaylistKeys.Local = d.PlaylistKeys.Local }

	if kb.SortKeys.Date == ""        { kb.SortKeys.Date = d.SortKeys.Date }
	if kb.SortKeys.Views == ""       { kb.SortKeys.Views = d.SortKeys.Views }
	if kb.SortKeys.Name == ""        { kb.SortKeys.Name = d.SortKeys.Name }
	if kb.SortKeys.Channel == ""     { kb.SortKeys.Channel = d.SortKeys.Channel }
	if kb.SortKeys.Duration == ""    { kb.SortKeys.Duration = d.SortKeys.Duration }
	if kb.SortKeys.Subscribers == "" { kb.SortKeys.Subscribers = d.SortKeys.Subscribers }
	if kb.SortKeys.Tags == ""        { kb.SortKeys.Tags = d.SortKeys.Tags }

	if kb.TabKeys.Recommended == ""   { kb.TabKeys.Recommended = d.TabKeys.Recommended }
	if kb.TabKeys.Subscriptions == "" { kb.TabKeys.Subscriptions = d.TabKeys.Subscriptions }
	if kb.TabKeys.Channels == ""      { kb.TabKeys.Channels = d.TabKeys.Channels }
	if kb.TabKeys.Playlists == ""     { kb.TabKeys.Playlists = d.TabKeys.Playlists }
	if kb.TabKeys.Search == ""        { kb.TabKeys.Search = d.TabKeys.Search }
	if kb.TabKeys.Downloading == ""   { kb.TabKeys.Downloading = d.TabKeys.Downloading }
	if kb.TabKeys.Local == ""         { kb.TabKeys.Local = d.TabKeys.Local }
	if kb.TabKeys.History == ""       { kb.TabKeys.History = d.TabKeys.History }
}

type BlacklistedChannel struct {
	ID   string `toml:"id,omitempty"` // YouTube channel ID — stable primary match key
	Name string `toml:"name"`         // human-readable label; fallback match when ID absent
}

type Config struct {
	DownloadDir           string               `toml:"download_dir"`
	Browser               string               `toml:"browser"`
	Player                string               `toml:"player"`
	PlayerBackend         string               `toml:"player_backend"`
	MaxDownloads          int                  `toml:"max_concurrent_downloads"`
	SponsorBlock          bool                 `toml:"sponsorblock"`
	SponsorBlockCats      []string             `toml:"sponsorblock_categories"`
	AudioFormat           string               `toml:"audio_format"`
	Theme                 string               `toml:"theme,omitempty"`
	Tabs                  []string             `toml:"tabs"`
	HintMode              string               `toml:"hint_mode"` // "full" | "minimal" | "none"
	RecommendedMaxAgeDays int                  `toml:"recommended_max_age_days"`
	RecommendedFetchCount int                  `toml:"recommended_fetch_count"`
	RecommendedMaxPages   int                  `toml:"recommended_max_pages"`
	ChannelLatestCount    int                  `toml:"channel_latest_count"`
	ChannelStrikes        int                  `toml:"channel_strikes"`
	StripEmojis           bool                 `toml:"strip_emojis"`
	CloseOnLinkOpen       bool                 `toml:"close_on_link_open"`
	CircularNav           bool                 `toml:"circular_nav"`
	Subtitles             bool                 `toml:"subtitles"`
	SubtitleLangs         []string             `toml:"subtitle_langs"`
	Keybindings           KeyBindings          `toml:"keybindings"`
	BlacklistedChannels   []BlacklistedChannel `toml:"blacklisted_channels"`
	DataDir               string               `toml:"-"`
}

var DefaultTabs = []string{
	"recommended", "subscriptions", "channels", "playlists",
	"search", "downloading", "local", "history", "activity",
}

func defaultConfig() *Config {
	return &Config{
		DownloadDir:           filepath.Join(os.Getenv("HOME"), "Videos", "yt-tui"),
		Browser:               "vivaldi+gnomekeyring",
		Player:                "mpv",
		PlayerBackend:         "mpris",
		MaxDownloads:          3,
		SponsorBlock:          true,
		SponsorBlockCats:      []string{"sponsor", "selfpromo", "interaction"},
		AudioFormat:           "mp3",
		Tabs:                  DefaultTabs,
		HintMode:              "full",
		RecommendedMaxAgeDays: 7,
		RecommendedFetchCount: 150,
		RecommendedMaxPages:   3,
		ChannelLatestCount:    3,
		ChannelStrikes:        2,
		StripEmojis:           true,
		CloseOnLinkOpen:       true,
		Subtitles:             true,
		SubtitleLangs:         []string{"en.*"},
		Keybindings:           defaultKeyBindings(),
	}
}

func Load() (*Config, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	appDir := filepath.Join(configDir, "yt-tui")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return nil, err
	}

	cfg := defaultConfig()

	cfgFile := filepath.Join(appDir, "config.toml")
	data, err := os.ReadFile(cfgFile)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		if err := toml.Unmarshal(data, cfg); err != nil {
			return nil, err
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

	if len(cfg.DownloadDir) > 1 && cfg.DownloadDir[:2] == "~/" {
		cfg.DownloadDir = filepath.Join(os.Getenv("HOME"), cfg.DownloadDir[2:])
	}

	if err := os.MkdirAll(cfg.DownloadDir, 0755); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Save() error {
	configDir, _ := os.UserConfigDir()
	cfgFile := filepath.Join(configDir, "yt-tui", "config.toml")
	return c.save(cfgFile)
}

func (c *Config) save(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}

func (c *Config) SubtitleLangsArg() string {
	if len(c.SubtitleLangs) == 0 {
		return ""
	}
	return strings.Join(c.SubtitleLangs, ",")
}

func (c *Config) SponsorBlockArg() string {
	if !c.SponsorBlock || len(c.SponsorBlockCats) == 0 {
		return ""
	}
	out := c.SponsorBlockCats[0]
	for _, cat := range c.SponsorBlockCats[1:] {
		out += "," + cat
	}
	return out
}

// AddBlacklistedChannel appends a channel to the blacklist if not already present,
// then saves config. Deduplicates by ID first, then by name.
func (c *Config) AddBlacklistedChannel(id, name string) {
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
