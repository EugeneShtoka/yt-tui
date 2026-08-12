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

// SaveAsync requests a background save without blocking the caller. Multiple
// requests arriving before the worker runs are coalesced into a single write.
// Falls back to a synchronous save if the worker was never started or was
// stopped by Close. The non-blocking send is done under mu so it can't race
// Close closing the channel.
func (c *Config) SaveAsync() {
	c.mu.Lock()
	if c.saveReq == nil {
		c.mu.Unlock()
		_ = c.Save() // worker gone (never started / stopped by Close): synchronous fallback
		return
	}
	select {
	case c.saveReq <- struct{}{}:
	default: // a save is already pending — coalesce
	}
	c.mu.Unlock()
}

// saveWorker drains coalesced save requests one at a time. The channel is
// passed in (not read from c.saveReq) so Close can nil the field without
// racing this loop.
func (c *Config) saveWorker(reqs <-chan struct{}) {
	for range reqs {
		_ = c.Save()
	}
}

// Close stops the background save worker after a final synchronous flush, so a
// coalesced save that was still pending on shutdown isn't lost. Idempotent;
// SaveAsync calls after Close fall back to synchronous saves.
func (c *Config) Close() {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		ch := c.saveReq
		c.saveReq = nil
		c.mu.Unlock()
		if ch != nil {
			close(ch)
		}
		_ = c.Save()
	})
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
	// Flush to disk before the rename so a crash immediately after can't leave a
	// renamed-but-empty file — the rename must publish durable bytes.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("save sync: %w", err)
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
