package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/adrg/xdg"
)

// Save writes the config to disk, serialized against concurrent saves and
// mutations via mu. Safe to call from multiple goroutines.
func (c *Config) Save() error {
	path := c.ConfigFile
	if path == "" {
		path = filepath.Join(xdg.ConfigHome, appName, "config.toml")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.save(path)
}

// Close writes the config a final time on shutdown. Idempotent.
func (c *Config) Close() {
	c.closeOnce.Do(func() { _ = c.Save() })
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
		_ = tmp.Close()        // the encode error is the report
		_ = os.Remove(tmpName) //nolint:gosec // G703: the temp file just created beside the user's config
		return fmt.Errorf("save encode: %w", err)
	}
	// Flush to disk before the rename so a crash immediately after can't leave a
	// renamed-but-empty file — the rename must publish durable bytes.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()        // the sync error is the report
		_ = os.Remove(tmpName) //nolint:gosec // G703: the temp file just created beside the user's config
		return fmt.Errorf("save sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName) //nolint:gosec // G703: the temp file just created beside the user's config
		return fmt.Errorf("save close: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil { //nolint:gosec // G703: the temp file onto the user's own config path
		return fmt.Errorf("save rename: %w", err)
	}
	return nil
}
