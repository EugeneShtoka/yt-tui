package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type KeyBindings struct {
	Download      string `toml:"download"`        // default "d"
	DownloadAudio string `toml:"download_audio"`  // default "D"
	Delete        string `toml:"delete"`          // default "x"
	Play          string `toml:"play"`            // default "p"
	HideChannel   string `toml:"hide_channel"`    // default "R"
	CopyURL       string `toml:"copy_url"`        // default "c"
	WatchLater    string `toml:"watch_later"`     // default "w"
	AddToPlaylist string `toml:"add_to_playlist"` // default "a"
	NewPlaylist   string `toml:"new_playlist"`    // default "n"
	ToggleMode    string `toml:"toggle_mode"`     // default "m"
	Help          string `toml:"help"`            // default "?"
	Quit          string `toml:"quit"`            // default "q"
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
	Tabs                  []string             `toml:"tabs"`
	RecommendedMaxAgeDays int                  `toml:"recommended_max_age_days"`
	RecommendedFetchCount int                  `toml:"recommended_fetch_count"`
	RecommendedMaxPages   int                  `toml:"recommended_max_pages"`
	Keybindings           KeyBindings          `toml:"keybindings"`
	BlacklistedChannels   []BlacklistedChannel `toml:"blacklisted_channels"`
	DataDir               string               `toml:"-"`
}

var DefaultTabs = []string{
	"recommended", "subscriptions", "playlists",
	"search", "downloading", "local", "history",
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
		RecommendedMaxAgeDays: 7,
		RecommendedFetchCount: 150,
		RecommendedMaxPages:   3,
		Keybindings: KeyBindings{
			Download:      "d",
			DownloadAudio: "D",
			Delete:        "x",
			Play:          "p",
			HideChannel:   "R",
			CopyURL:       "c",
			WatchLater:    "w",
			AddToPlaylist: "a",
			NewPlaylist:   "n",
			ToggleMode:    "m",
			Help:          "?",
			Quit:          "q",
		},
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
	} else {
		if err := cfg.save(cfgFile); err != nil {
			return nil, err
		}
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
			// Already present; enrich name if missing.
			if bl.Name == "" {
				c.BlacklistedChannels[i].Name = name
			}
			return
		}
		if bl.ID == "" && strings.EqualFold(bl.Name, name) {
			// Name-only entry — add the ID now.
			c.BlacklistedChannels[i].ID = id
			return
		}
	}
	c.BlacklistedChannels = append(c.BlacklistedChannels, BlacklistedChannel{ID: id, Name: name})
}
