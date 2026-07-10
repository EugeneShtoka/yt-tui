package config

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/BurntSushi/toml"
)

// TestConcurrentBlacklistSaves is the acceptance test for the config
// serialization work (REFACTOR_PLAN P0.1b): many goroutines mutating the
// blacklist and saving concurrently must not race and must leave a valid file.
// Run with: go test -race ./internal/config/...
func TestConcurrentBlacklistSaves(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultConfig()
	cfg.ConfigFile = filepath.Join(dir, "config.toml")

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cfg.AddBlacklistedChannel(fmt.Sprintf("id-%d", i), fmt.Sprintf("name-%d", i))
			if err := cfg.Save(); err != nil {
				t.Errorf("save: %v", err)
			}
		}(i)
	}
	wg.Wait()

	// The final file must be valid TOML and hold all entries (atomic writes
	// mean no save can truncate another mid-flight).
	var got Config
	if _, err := toml.DecodeFile(cfg.ConfigFile, &got); err != nil {
		t.Fatalf("final config is not valid TOML: %v", err)
	}
	if len(got.BlacklistedChannels) != n {
		t.Fatalf("want %d blacklisted channels, got %d", n, len(got.BlacklistedChannels))
	}
}

// TestAtomicSaveLeavesValidFile checks a single save produces a parseable file
// (regression guard for the temp-file+rename path).
func TestAtomicSaveLeavesValidFile(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultConfig()
	cfg.ConfigFile = filepath.Join(dir, "config.toml")
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	var got Config
	if _, err := toml.DecodeFile(cfg.ConfigFile, &got); err != nil {
		t.Fatalf("saved config is not valid TOML: %v", err)
	}
}
