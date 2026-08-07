package config

import (
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
	// KeyBindings holds a map (TabKeys) so it is no longer == comparable.
	if !reflect.DeepEqual(kb, want) {
		t.Errorf("fillDefaults on zero struct did not produce defaultKeyBindings()\ngot:  %+v\nwant: %+v", kb, want)
	}
}

func TestFillDefaultsPreservesExisting(t *testing.T) {
	kb := KeyBindings{
		Play:     "space",
		SortKeys: SortKeys{Date: "D"},
		TabKeys:  map[string]string{"1": "feed"},
	}
	kb.fillDefaults()
	if kb.Play != "space" {
		t.Errorf("fillDefaults overwrote Play: got %q, want %q", kb.Play, "space")
	}
	if kb.SortKeys.Date != "D" {
		t.Errorf("fillDefaults overwrote SortKeys.Date: got %q, want %q", kb.SortKeys.Date, "D")
	}
	// A non-empty TabKeys map is preserved verbatim (not merged with defaults).
	if kb.TabKeys["1"] != "feed" || len(kb.TabKeys) != 1 {
		t.Errorf("fillDefaults did not preserve TabKeys: got %v", kb.TabKeys)
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

// TestConcurrentSaves is the acceptance test for the config serialization work
// (REFACTOR_PLAN P0.1b): many goroutines saving concurrently must not race and
// must leave a valid, complete file — the atomic temp-file+rename means no save
// can truncate another mid-flight. Run with: go test -race ./internal/config/...
func TestConcurrentSaves(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultConfig()
	cfg.ConfigFile = filepath.Join(dir, "config.toml")
	cfg.Player = "mpv-concurrent-marker"

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := cfg.Save(); err != nil {
				t.Errorf("save: %v", err)
			}
		}()
	}
	wg.Wait()

	// The final file must be valid TOML with the marker field intact.
	var got Config
	if _, err := toml.DecodeFile(cfg.ConfigFile, &got); err != nil {
		t.Fatalf("final config is not valid TOML: %v", err)
	}
	if got.Player != "mpv-concurrent-marker" {
		t.Fatalf("Player = %q, want marker (corrupt/truncated concurrent save?)", got.Player)
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

// TestConfigCloseFlushesAndIsIdempotent verifies Close performs a final flush
// (so a coalesced SaveAsync isn't lost on shutdown), tolerates a second call,
// and leaves SaveAsync working via the synchronous fallback (no send on a
// closed channel).
func TestConfigCloseFlushesAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultConfig()
	cfg.ConfigFile = filepath.Join(dir, "config.toml")
	cfg.saveReq = make(chan struct{}, 1)
	go cfg.saveWorker(cfg.saveReq)

	cfg.DownloadDir = filepath.Join(dir, "sentinel")
	cfg.SaveAsync()

	cfg.Close()

	var got Config
	if _, err := toml.DecodeFile(cfg.ConfigFile, &got); err != nil {
		t.Fatalf("config not valid TOML after Close: %v", err)
	}
	if got.DownloadDir != cfg.DownloadDir {
		t.Fatalf("Close did not flush latest state: got %q want %q", got.DownloadDir, cfg.DownloadDir)
	}
	if cfg.saveReq != nil {
		t.Fatal("Close did not nil saveReq")
	}

	cfg.Close()     // second call must not panic (closeOnce)
	cfg.SaveAsync() // after Close must not panic (falls back to sync save)
}
