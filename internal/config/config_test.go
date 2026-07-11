package config

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestFillDefaultsZeroInput(t *testing.T) {
	var kb KeyBindings
	kb.fillDefaults()
	want := defaultKeyBindings()
	if kb != want {
		t.Errorf("fillDefaults on zero struct did not produce defaultKeyBindings()\ngot:  %+v\nwant: %+v", kb, want)
	}
}

func TestFillDefaultsPreservesExisting(t *testing.T) {
	kb := KeyBindings{
		Play:    "space",
		SortKeys: SortKeys{Date: "D"},
		TabKeys:  TabKeys{Recommended: "1"},
	}
	kb.fillDefaults()
	if kb.Play != "space" {
		t.Errorf("fillDefaults overwrote Play: got %q, want %q", kb.Play, "space")
	}
	if kb.SortKeys.Date != "D" {
		t.Errorf("fillDefaults overwrote SortKeys.Date: got %q, want %q", kb.SortKeys.Date, "D")
	}
	if kb.TabKeys.Recommended != "1" {
		t.Errorf("fillDefaults overwrote TabKeys.Recommended: got %q, want %q", kb.TabKeys.Recommended, "1")
	}
	// Other fields should be filled with defaults.
	d := defaultKeyBindings()
	if kb.Download != d.Download {
		t.Errorf("fillDefaults did not fill Download: got %q, want %q", kb.Download, d.Download)
	}
}

func TestFillDefaultsNoEmptyFields(t *testing.T) {
	// After fillDefaults, every string field at any nesting depth must be non-empty.
	var kb KeyBindings
	kb.fillDefaults()
	var check func(v reflect.Value, path string)
	check = func(v reflect.Value, path string) {
		tp := v.Type()
		for i := 0; i < tp.NumField(); i++ {
			fv := v.Field(i)
			ft := tp.Field(i)
			switch fv.Kind() {
			case reflect.String:
				if fv.String() == "" {
					t.Errorf("field %s.%s is empty after fillDefaults", path, ft.Name)
				}
			case reflect.Struct:
				check(fv, path+"."+ft.Name)
			}
		}
	}
	check(reflect.ValueOf(kb), "KeyBindings")
}

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
