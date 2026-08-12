package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
)

const (
	// appName is the per-application subdirectory under each XDG base dir.
	appName = "yt-tui"
	// dbFileName mirrors the database filename db.New manages inside DataDir;
	// config only needs it to migrate a legacy copy out of the old config dir.
	dbFileName = "yt-tui.db"
	// logFileName is the debug log written under StateDir (see (*Config).LogPath).
	logFileName = "debug.log"
)

// appDirs holds yt-tui's per-purpose base directories, following the XDG
// base-directory spec: config (config.toml, theme.toml), data (the durable
// database) and state (the debug log). Keeping them distinct means the DB and
// log no longer pollute the config dir.
type appDirs struct {
	Config string
	Data   string
	State  string
}

// resolveAppDirs resolves the three XDG base directories for yt-tui and ensures
// each exists. The base homes come from github.com/adrg/xdg, which honors
// $XDG_CONFIG_HOME / $XDG_DATA_HOME / $XDG_STATE_HOME with spec-compliant
// fallbacks (~/.config, ~/.local/share, ~/.local/state on Linux).
func resolveAppDirs() (appDirs, error) {
	dirs := appDirs{
		Config: filepath.Join(xdg.ConfigHome, appName),
		Data:   filepath.Join(xdg.DataHome, appName),
		State:  filepath.Join(xdg.StateHome, appName),
	}
	for _, d := range []string{dirs.Config, dirs.Data, dirs.State} {
		if err := os.MkdirAll(d, 0750); err != nil {
			return appDirs{}, fmt.Errorf("resolveAppDirs mkdir %q: %w", d, err)
		}
	}
	return dirs, nil
}

// resolveDataDir returns the durable data directory: the config's data_dir
// override (expanded to an absolute path and created if missing) when set,
// otherwise the XDG-default fallback.
func resolveDataDir(override, fallback string) (string, error) {
	if override == "" {
		return fallback, nil
	}
	dir, err := absPath(override)
	if err != nil {
		return "", fmt.Errorf("Load data_dir %q: %w", override, err)
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", fmt.Errorf("Load mkdir data_dir %q: %w", dir, err)
	}
	return dir, nil
}

// absPath expands a leading ~/ to the user's home directory and returns the
// cleaned absolute form of p. Used for the --config path and the data_dir
// override so both accept relative and home-relative paths.
func absPath(p string) (string, error) {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand ~: %w", err)
		}
		p = filepath.Join(home, p[2:])
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("absPath %q: %w", p, err)
	}
	return abs, nil
}

// migrateLegacyFiles moves the database and debug log out of the config dir
// (where every file used to live before the XDG split) into their data/state
// homes. Best-effort: a destination that already exists, or a source that is
// absent, is skipped, and a failed move is logged rather than fatal — the app
// simply recreates the file in its new home. The DB's WAL/SHM sidecars are
// moved alongside it so SQLite doesn't reopen against a stale journal.
func migrateLegacyFiles(dirs appDirs) {
	moves := []struct{ name, dstDir string }{
		{dbFileName, dirs.Data},
		{dbFileName + "-wal", dirs.Data},
		{dbFileName + "-shm", dirs.Data},
		{logFileName, dirs.State},
	}
	for _, m := range moves {
		src := filepath.Join(dirs.Config, m.name)
		dst := filepath.Join(m.dstDir, m.name)
		if err := moveIfAbsent(src, dst); err != nil {
			fmt.Fprintf(os.Stderr, "config: could not migrate %s -> %s: %v\n", src, dst, err)
		}
	}
}

// moveIfAbsent renames src to dst when dst does not yet exist and src does.
// A missing source or an already-present destination is a no-op (nil error),
// which makes the whole migration idempotent across restarts.
func moveIfAbsent(src, dst string) error {
	if fileExists(dst) {
		return nil // already migrated
	}
	if !fileExists(src) {
		return nil // nothing to move (absent or unreadable) — leave it
	}
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("moveIfAbsent %s -> %s: %w", src, dst, err)
	}
	return nil
}

// fileExists reports whether path names an existing, stat-able file.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// LogPath returns the absolute path of the debug log inside StateDir.
func (c *Config) LogPath() string {
	return filepath.Join(c.StateDir, logFileName)
}

// ThumbnailsPath returns the resolved thumbnail cache directory: the configured
// override, or DataDir/thumbnails by default.
func (c *Config) ThumbnailsPath() string {
	if c.ThumbnailsDir != "" {
		return c.ThumbnailsDir
	}
	return filepath.Join(c.DataDir, "thumbnails")
}

// LocalThumbnailsPath returns the directory for the client-local thumbnail cache
// (the reactive "cache everything you view" store). It is deliberately distinct
// from ThumbnailsPath: the daemon/enrichment store there is pruned to the
// eligible newest-N set by Retain, whereas this cache must keep whatever the
// user viewed, so the two never share a directory.
func (c *Config) LocalThumbnailsPath() string {
	return filepath.Join(c.DataDir, "thumbnails-cache")
}

// LocalTranscriptsPath returns the directory for the client-local transcript-text
// cache — the transcript counterpart to LocalThumbnailsPath, kept distinct from
// the daemon's TranscriptsPath for the same reason.
func (c *Config) LocalTranscriptsPath() string {
	return filepath.Join(c.DataDir, "transcripts-cache")
}

// TranscriptsPath returns the resolved raw-transcript (.srt archive) directory:
// the configured override, or DataDir/transcripts by default.
func (c *Config) TranscriptsPath() string {
	if c.TranscriptsDir != "" {
		return c.TranscriptsDir
	}
	return filepath.Join(c.DataDir, "transcripts")
}

// TranscriptMarkdownPath returns the resolved markdown-export directory: the
// configured override, or a sibling of the transcripts dir named
// transcript-md by default (DataDir/transcript-md when the transcripts dir is
// itself the default).
func (c *Config) TranscriptMarkdownPath() string {
	if c.TranscriptMarkdownDir != "" {
		return c.TranscriptMarkdownDir
	}
	return filepath.Join(filepath.Dir(c.TranscriptsPath()), "transcript-md")
}

// ProfilesPath returns the directory holding daemon-stored named config
// profiles (DataDir/profiles). It lives under DataDir because profiles are
// durable app data, served by the daemon (or the in-process backend locally).
func (c *Config) ProfilesPath() string {
	return filepath.Join(c.DataDir, "profiles")
}

// prepareDownloadDir expands a leading ~/ in DownloadDir and ensures it exists.
func prepareDownloadDir(cfg *Config) error {
	if len(cfg.DownloadDir) > 1 && cfg.DownloadDir[:2] == "~/" {
		cfg.DownloadDir = filepath.Join(os.Getenv("HOME"), cfg.DownloadDir[2:])
	}
	if err := os.MkdirAll(cfg.DownloadDir, 0750); err != nil {
		return fmt.Errorf("Load mkdir download: %w", err)
	}
	return nil
}
