package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// envConfigPath names the environment variable that overrides the config-file
// location, mirroring the --config flag. The flag takes precedence when both
// are set; either one lets a single host run several independent configs.
const envConfigPath = "YT_TUI_CONFIG"

// DaemonConfig holds settings owned by the headless yt-tuid daemon: download
// location, browser cookie source, yt-dlp fetch parameters, and feed filters.
// These fields are irrelevant when the TUI connects to a remote daemon.
type DaemonConfig struct {
	Token       string `toml:"token,omitempty"`
	TLSCert     string `toml:"tls_cert,omitempty"`
	TLSKey      string `toml:"tls_key,omitempty"`
	DownloadDir string `toml:"download_dir"`
	Browser     string `toml:"browser"`
	CookiesFile string `toml:"cookies_file,omitempty"`
	// YouTubeEnabled makes "YouTube access is allowed" explicit. Default true.
	// When false, the startup availability probe is skipped (no yt-dlp/cookie
	// warnings) — set it off on hosts that intentionally run without YouTube.
	YouTubeEnabled bool `toml:"youtube_enabled"`
	// YtdlpUpdateCheck lets the startup probe learn the newest yt-dlp release by
	// asking GitHub once a day, in the background, and caching the answer under
	// StateDir. It is the only network call the probe's version check involves —
	// the check itself only ever reads that cache — so turning this off leaves the
	// probe comparing against what the host's package manager offers and nothing
	// more. Default true.
	YtdlpUpdateCheck           bool     `toml:"ytdlp_update_check"`
	MaxDownloads               int      `toml:"max_concurrent_downloads"`
	SponsorBlock               bool     `toml:"sponsorblock"`
	SponsorBlockCats           []string `toml:"sponsorblock_categories"`
	AudioFormat                string   `toml:"audio_format"`
	RecommendedMaxAgeDays      int      `toml:"recommended_max_age_days"`
	RecommendedMinDurationSecs int      `toml:"recommended_min_duration_secs"`
	RecommendedMinViews        int      `toml:"recommended_min_views"`
	RecommendedFetchCount      int      `toml:"recommended_fetch_count"`
	RecommendedMaxPages        int      `toml:"recommended_max_pages"`
	ChannelLatestCount         int      `toml:"channel_latest_count"`
	RefreshMinutes             int      `toml:"refresh_minutes"` // skip auto-refresh of a channel's videos or the YouTube playlist list if refreshed within this many minutes; manual refresh always fetches
	// WatchLaterAutoRemovePercent auto-removes a video from Watch Later once it has
	// been watched at least this % of its duration (0 disables). Clamped to 0..100.
	WatchLaterAutoRemovePercent int `toml:"watch_later_auto_remove_percent"`
	ChannelStrikes              int `toml:"channel_strikes"`
	// EnrichmentDelaySeconds paces the background pass that fetches full details
	// (exact upload date, description, chapters) for subscribed-channel videos
	// via yt-dlp, correcting the approximate dates the flat listing yields. It is
	// the sleep between successive yt-dlp calls. 0 disables the details pass.
	EnrichmentDelaySeconds int `toml:"enrichment_delay_seconds"`
	// ThumbnailsPerChannel is how many of the newest videos per subscribed
	// channel have their thumbnail image cached to disk (served back over the
	// GetThumbnail RPC so the client renders instantly). 0 disables thumbnail
	// caching.
	ThumbnailsPerChannel int      `toml:"thumbnails_per_channel"`
	StripEmojis          bool     `toml:"strip_emojis"`
	Subtitles            bool     `toml:"subtitles"`
	SubtitleLangs        []string `toml:"subtitle_langs"`
	// SaveTranscript, when true, has the background enrichment pass build a
	// canonical markdown transcript note (.md) for every eligible video. Uses
	// SubtitleLangs to choose the caption language(s).
	SaveTranscript bool `toml:"save_transcript"`
	// DataDirOverride relocates the durable data directory — the SQLite database
	// plus every DataDir-derived store (thumbnails, transcripts, profiles) — away
	// from the XDG default (xdg.DataHome/yt-tui). Empty keeps the default. A
	// leading ~/ is expanded and the path is made absolute. Combined with a
	// per-config --config file, this gives each config its own independent DB.
	// Read the resolved value via cfg.DataDir, never this field directly.
	DataDirOverride string `toml:"data_dir,omitempty"`
	// ThumbnailsDir, TranscriptsDir and TranscriptMarkdownDir override where the
	// three on-disk artifact stores live. Empty means the OS-appropriate default
	// under DataDir (xdg.DataHome/yt-tui): thumbnails/, transcripts/ (raw .srt
	// archive) and transcript-md/ (the canonical Obsidian-facing .md, a sibling of
	// the transcripts dir). Resolve via the ThumbnailsPath/TranscriptsPath/
	// TranscriptMarkdownPath helpers rather than reading these directly.
	ThumbnailsDir         string `toml:"thumbnails_dir,omitempty"`
	TranscriptsDir        string `toml:"transcripts_dir,omitempty"`
	TranscriptMarkdownDir string `toml:"transcript_md_dir,omitempty"`
}

// Panel is one entry in the data-driven tab bar. The ordered Panels list
// replaces the former fixed tab set: each panel names a tab Type to construct,
// an optional source Mode (feed/channels/tags only; empty inherits the tab's
// global default — FeedMode/ChannelsView/TagsMode), and an optional default
// Sort (list tabs only; empty keeps the tab's built-in default). Name is the
// stable identity referenced by KeyBindings.TabKeys and the `:tab` command.
type Panel struct {
	Name string `toml:"name" json:"name"`
	Type string `toml:"type" json:"type"`
	Mode string `toml:"mode,omitempty" json:"mode,omitempty"`
	Sort string `toml:"sort,omitempty" json:"sort,omitempty"`
}

// ClientConfig holds settings used only by the yt-tui TUI client: local player
// binary, visual theme, tab layout, UI preferences, and key bindings.
// These fields are irrelevant on a headless daemon host.
type ClientConfig struct {
	DaemonAddr  string `toml:"daemon_addr,omitempty"`
	DaemonToken string `toml:"daemon_token,omitempty"`
	TLSCACert   string `toml:"tls_ca_cert,omitempty"`
	// LoadProfile names a daemon-stored config profile to fetch and apply over
	// the local config on connect (remote mode only). Empty means keep the local
	// config as-is. Machine-local (which profile *this* client wants), so it is
	// never carried in an exported profile. See internal/tui/app.LoadProfileOnConnect.
	LoadProfile    string  `toml:"load_profile,omitempty"`
	Player         string  `toml:"player"`
	PlayerBackend  string  `toml:"player_backend"`
	Theme          string  `toml:"theme,omitempty"`
	Panels         []Panel `toml:"panels"`          // ordered, data-driven tab bar; empty falls back to DefaultPanels
	HintMode       string  `toml:"hint_mode"`       // "full" | "minimal" | "none"
	DurationFormat string  `toml:"duration_format"` // see render.DurFmt constants
	DateFormat     string  `toml:"date_format"`     // see render.DateFmt constants
	// TranscriptWidth sizes the transcript popup: an absolute column count ("80")
	// or a percentage of the terminal width ("50%"). Defaults to "50%".
	TranscriptWidth string `toml:"transcript_width"`
	// LocalThumbnails opts a *remote* client into keeping its own thumbnail cache
	// (every thumbnail it views, in a local directory), instead of asking the
	// daemon on each open. Off by default. Single-binary mode always caches
	// locally — it has no remote server — so this knob is a no-op there.
	LocalThumbnails bool `toml:"local_thumbnails"`
	// LocalTranscripts opts a *remote* client into keeping its own cache of the
	// transcript text it views, mirroring LocalThumbnails. Off by default;
	// single-binary always caches locally (no remote server), so it is a no-op there.
	LocalTranscripts bool `toml:"local_transcripts"`
	// LocalThumbnailsMax / LocalTranscriptsMax bound the client-local caches by
	// newest-N (evicted by mtime at startup). <=0 keeps everything.
	LocalThumbnailsMax  int `toml:"local_thumbnails_max"`
	LocalTranscriptsMax int `toml:"local_transcripts_max"`
	// LocalPrewarm, when "eager", proactively warms the local thumbnail cache with
	// the daemon's eligible set on connect (remote mode only), so thumbnails render
	// instantly and offline instead of on first open. Empty/"off" disables it.
	// Only meaningful with local_thumbnails on.
	LocalPrewarm string `toml:"local_prewarm"`
	// LocalPrewarmConcurrency bounds the eager pre-warm worker pool. Thumbnails are
	// CDN/daemon reads, safe to parallelize; <=0 uses the built-in default (~16).
	LocalPrewarmConcurrency int    `toml:"local_prewarm_concurrency"`
	CloseOnLinkOpen         bool   `toml:"close_on_link_open"`
	CircularNav             bool   `toml:"circular_nav"`
	FeedMode                string `toml:"feed_mode"`     // Feed tab default mode: "recommended" | "subscribed" | "mixed" | "stale"
	ChannelsView            string `toml:"channels_view"` // Channels tab default view: "recommended" | "subscribed" | "mixed" | "blocked" | "stale"
	TagsMode                string `toml:"tags_mode"`     // Tags tab default mode: "recommended" | "subscribed" | "mixed" | "stale"
	// HideStaleTaggedChannels hides tagged, unsubscribed channels with no activity
	// within StaleTaggedChannelDays from the Channels/Tags panels' non-stale modes;
	// the "stale" mode surfaces exactly that set. Off by default (nothing hidden
	// until opted in). See internal/domain/channels.IsStale.
	HideStaleTaggedChannels bool `toml:"hide_stale_tagged_channels"`
	StaleTaggedChannelDays  int  `toml:"stale_tagged_channel_days"` // stale threshold in days (default 30)
	// Columns maps a panel name (see Panels) to the ordered list of column keys
	// to show in that panel's primary list. An absent or empty entry shows all
	// of the panel's columns in their natural order — the default that preserves
	// today's look. A present list both filters (only listed keys survive) and
	// reorders (list order wins). Keyed by panel name — like TabKeys — so it
	// survives panel reordering; unknown/unavailable keys are validated at
	// startup (see app.ValidateColumns) and ignored. Applied via
	// videotable.SelectColumns in each tab constructor.
	Columns     map[string][]string `toml:"columns,omitempty" json:"columns,omitempty"`
	Keybindings KeyBindings         `toml:"keybindings"`
}

// Config is the unified configuration used in single-binary (InProc) mode.
// It embeds DaemonConfig and ClientConfig so the existing flat config.toml
// layout is preserved — no migration required for existing installations.
// When the TUI runs with --connect, only ClientConfig fields are relevant;
// when yt-tuid runs headlessly, only DaemonConfig fields are relevant.
type Config struct {
	DaemonConfig
	ClientConfig
	// ConfigDir holds config.toml + theme.toml (xdg.ConfigHome/yt-tui);
	// DataDir holds the durable database (xdg.DataHome/yt-tui);
	// StateDir holds the debug log (xdg.StateHome/yt-tui).
	ConfigDir  string `toml:"-"`
	DataDir    string `toml:"-"`
	StateDir   string `toml:"-"`
	ConfigFile string `toml:"-"`

	// Issues holds the non-fatal problems found while loading this config
	// (invalid enums reset to defaults, dropped/unreachable panels, pruned tab
	// hotkeys). Populated by Load; the client composition root appends runtime
	// issues (theme/TLS/player/YouTube) and shows them in the startup overlay.
	// Not persisted.
	Issues []ConfigIssue `toml:"-"`

	// mu guards mutable config fields and serializes file writes. It is an
	// RWMutex so background readers (enrichment loop, downloader) can take a
	// consistent snapshot (RLock) concurrently while a mid-session profile
	// import takes the write lock (see Mutate / DaemonSnapshot).
	// Unexported fields are ignored by the TOML encoder.
	mu sync.RWMutex
	// saveReq coalesces async save requests into a single background write.
	// Guarded by mu so Close can nil-and-close it without racing SaveAsync.
	saveReq chan struct{}
	// closeOnce guards a single shutdown of the save worker.
	closeOnce sync.Once
}

// DefaultPanels reproduces the historical fixed tab bar: one panel per tab
// type, in the original order, named after its type. Mode/Sort are left empty
// so each panel inherits its tab's global default — nothing changes until the
// user customizes the list.
var DefaultPanels = []Panel{
	{Name: "feed", Type: "feed"},
	{Name: "channels", Type: "channels"},
	{Name: "tags", Type: "tags"},
	{Name: "playlists", Type: "playlists"},
	{Name: "search", Type: "search"},
	{Name: "downloading", Type: "downloading"},
	{Name: "local", Type: "local"},
	{Name: "history", Type: "history"},
	{Name: "activity", Type: "activity"},
}

func defaultConfig() *Config {
	return &Config{
		DaemonConfig: DaemonConfig{
			DownloadDir:                 filepath.Join(os.Getenv("HOME"), "Videos", "yt-tui"),
			Browser:                     "vivaldi+gnomekeyring",
			YouTubeEnabled:              true,
			YtdlpUpdateCheck:            true,
			MaxDownloads:                3,
			SponsorBlock:                true,
			SponsorBlockCats:            []string{"sponsor", "selfpromo", "interaction"},
			AudioFormat:                 "mp3",
			RecommendedMaxAgeDays:       7,
			RecommendedFetchCount:       150,
			RecommendedMaxPages:         3,
			ChannelLatestCount:          3,
			RefreshMinutes:              60,
			WatchLaterAutoRemovePercent: 90,
			ChannelStrikes:              2,
			EnrichmentDelaySeconds:      5,
			ThumbnailsPerChannel:        30,
			StripEmojis:                 true,
			Subtitles:                   true,
			SubtitleLangs:               []string{"en"},
		},
		ClientConfig: ClientConfig{
			Player:        "mpv",
			PlayerBackend: "mpris",
			// Copy so a config file's [[panels]] decoding (which reuses the slice's
			// backing array) can't mutate the shared DefaultPanels package var.
			Panels:                 append([]Panel(nil), DefaultPanels...),
			HintMode:               "full",
			DurationFormat:         "hh:mm:ss",
			DateFormat:             "dd/mm/yyyy",
			TranscriptWidth:        "50%",
			CloseOnLinkOpen:        true,
			FeedMode:               "recommended",
			ChannelsView:           "subscribed",
			TagsMode:               "subscribed",
			StaleTaggedChannelDays: 30,
			Keybindings:            defaultKeyBindings(),
		},
	}
}

// Load reads the config from its default location, honoring the YT_TUI_CONFIG
// environment variable if set. It is shorthand for LoadFrom("").
func Load() (*Config, error) { return LoadFrom("") }

// LoadFrom reads the config, allowing an explicit config-file path override
// (the --config flag). Precedence for locating config.toml is: the override
// argument, then $YT_TUI_CONFIG, then the XDG default
// (xdg.ConfigHome/yt-tui/config.toml). When an override is used, its parent
// becomes the config dir (theme.toml lives beside it) and the legacy DB/log
// migration is skipped — that only makes sense for the default install layout.
//
// The data directory (SQLite DB, thumbnails, transcripts, profiles) defaults to
// xdg.DataHome/yt-tui but is relocated when the loaded config sets data_dir, so
// each config can point at its own independent database.
func LoadFrom(override string) (*Config, error) {
	dirs, err := resolveAppDirs()
	if err != nil {
		return nil, err
	}

	if override == "" {
		override = strings.TrimSpace(os.Getenv(envConfigPath))
	}

	cfgFile := filepath.Join(dirs.Config, "config.toml")
	configDir := dirs.Config
	if override != "" {
		resolved, aerr := absPath(override)
		if aerr != nil {
			return nil, fmt.Errorf("Load config path %q: %w", override, aerr)
		}
		cfgFile = resolved
		// Accept either a config file or a directory: when the override points at
		// an existing directory, config.toml is resolved inside it.
		if info, statErr := os.Stat(cfgFile); statErr == nil && info.IsDir() {
			cfgFile = filepath.Join(cfgFile, "config.toml")
		}
		configDir = filepath.Dir(cfgFile)
		if mkErr := os.MkdirAll(configDir, 0750); mkErr != nil {
			return nil, fmt.Errorf("Load mkdir config %q: %w", configDir, mkErr)
		}
	} else {
		// Best-effort: pull the DB + log out of the old (config) dir on first run
		// after the XDG split, so upgrades don't silently start on an empty DB.
		migrateLegacyFiles(dirs)
	}

	cfg := defaultConfig()
	if err := loadConfigFile(cfg, cfgFile); err != nil {
		return nil, err
	}

	// A data_dir override in the config relocates the DB and every
	// DataDir-derived store; otherwise fall back to the XDG data home.
	dataDir, derr := resolveDataDir(cfg.DataDirOverride, dirs.Data)
	if derr != nil {
		return nil, derr
	}

	// Always re-save so any missing/new keybindings appear in the file.
	if err := cfg.save(cfgFile); err != nil {
		return nil, err
	}

	cfg.ConfigDir = configDir
	cfg.DataDir = dataDir
	cfg.StateDir = dirs.State
	cfg.ConfigFile = cfgFile

	// Start the background save worker now that ConfigFile is known. All saves
	// after startup go through this single goroutine (via SaveAsync) or Save,
	// both serialized by cfg.mu.
	cfg.saveReq = make(chan struct{}, 1)
	go cfg.saveWorker(cfg.saveReq)

	if err := prepareDownloadDir(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// loadConfigFile reads cfgFile into cfg (if it exists) and applies defaults for
// any missing or invalid fields. A missing file is not an error.
func loadConfigFile(cfg *Config, cfgFile string) error {
	data, err := os.ReadFile(cfgFile)
	if os.IsNotExist(err) {
		// No file yet — still normalize the pure defaults (e.g. backfill each
		// panel's Mode/Sort) so the first-run save writes a complete config.
		// Nothing to complain about, so no issue log.
		cfg.Keybindings.fillDefaults()
		applyDerivedDefaults(cfg, nil)
		return nil
	}
	if err != nil {
		return fmt.Errorf("Load read: %w", err)
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("Load unmarshal: %w", err)
	}
	cfg.Keybindings.fillDefaults()
	var log issueLog
	applyDerivedDefaults(cfg, &log)
	cfg.Issues = log.items
	return nil
}
