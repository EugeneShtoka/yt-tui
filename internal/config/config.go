package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DownloadDir          string   `yaml:"download_dir"`
	Browser              string   `yaml:"browser"`
	Player               string   `yaml:"player"`
	MaxDownloads         int      `yaml:"max_concurrent_downloads"`
	SponsorBlock         bool     `yaml:"sponsorblock"`
	SponsorBlockCats     []string `yaml:"sponsorblock_categories"`
	AudioFormat          string   `yaml:"audio_format"`
	Tabs                 []string `yaml:"tabs"`
	RecommendedMaxAgeDays int     `yaml:"recommended_max_age_days"`
	DataDir              string   `yaml:"-"`
}

var DefaultTabs = []string{
	"recommended", "subscriptions", "playlists",
	"search", "downloading", "local", "history",
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

	cfg := &Config{
		DownloadDir:           filepath.Join(os.Getenv("HOME"), "Videos", "yt-tui"),
		Browser:               "vivaldi+gnomekeyring",
		Player:                "mpv",
		MaxDownloads:          3,
		SponsorBlock:          true,
		SponsorBlockCats:      []string{"sponsor", "selfpromo", "interaction"},
		AudioFormat:           "mp3",
		Tabs:                  DefaultTabs,
		RecommendedMaxAgeDays: 7,
		DataDir:               appDir,
	}

	cfgFile := filepath.Join(appDir, "config.yaml")
	data, err := os.ReadFile(cfgFile)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	} else {
		// Write default config
		out, _ := yaml.Marshal(cfg)
		_ = os.WriteFile(cfgFile, out, 0644)
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
