package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/BurntSushi/toml"
)

// loadFromTOML writes body to a temp config.toml, runs it through the same
// unmarshal + fillDefaults + applyDerivedDefaults path Load uses, and returns
// the resulting Config for assertions.
func loadFromTOML(t *testing.T, body string) *Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg := defaultConfig()
	if err := loadConfigFile(cfg, path); err != nil {
		t.Fatalf("loadConfigFile: %v", err)
	}
	return cfg
}

func TestDefaultConfigPanelsAndTabKeys(t *testing.T) {
	cfg := defaultConfig()
	if !reflect.DeepEqual(cfg.Panels, DefaultPanels) {
		t.Errorf("defaultConfig Panels = %v, want DefaultPanels", cfg.Panels)
	}
	if !reflect.DeepEqual(cfg.Keybindings.TabKeys, defaultTabKeys()) {
		t.Errorf("defaultConfig TabKeys = %v, want defaultTabKeys()", cfg.Keybindings.TabKeys)
	}
	// Every default tab-key value must name a default panel (no dangling refs).
	names := map[string]bool{}
	for _, p := range DefaultPanels {
		names[p.Name] = true
	}
	for hotkey, name := range defaultTabKeys() {
		if !names[name] {
			t.Errorf("default tab key %q -> %q names no default panel", hotkey, name)
		}
	}
}

func TestFillDefaultsSeedsTabKeysWhenAbsent(t *testing.T) {
	// A config that predates the panel migration has no [keybindings.tab_keys]
	// table; fillDefaults must seed the built-in hotkeys rather than leave nil.
	cfg := loadFromTOML(t, "player = \"mpv\"\n")
	if !reflect.DeepEqual(cfg.Keybindings.TabKeys, defaultTabKeys()) {
		t.Errorf("absent tab_keys not seeded: got %v", cfg.Keybindings.TabKeys)
	}
	if len(cfg.Panels) == 0 {
		t.Error("absent panels not defaulted")
	}
}

func TestValidatePanelsDropsUnknownType(t *testing.T) {
	cfg := loadFromTOML(t, `
[[panels]]
name = "feed"
type = "feed"

[[panels]]
name = "bogus"
type = "does-not-exist"

[[panels]]
name = "local"
type = "local"
`)
	if len(cfg.Panels) != 2 {
		t.Fatalf("want 2 panels after dropping unknown, got %d: %v", len(cfg.Panels), cfg.Panels)
	}
	for _, p := range cfg.Panels {
		if p.Type == "does-not-exist" {
			t.Errorf("unknown-type panel survived validation: %v", p)
		}
	}
}

func TestValidatePanelsEmptyFallsBackToDefaults(t *testing.T) {
	cfg := loadFromTOML(t, `
[[panels]]
name = "x"
type = "nope"
`)
	// Falls back to the default layout (names/types), with mode/sort backfilled.
	if len(cfg.Panels) != len(DefaultPanels) {
		t.Fatalf("all-invalid panel list did not fall back to DefaultPanels: got %d", len(cfg.Panels))
	}
	for i, p := range DefaultPanels {
		if cfg.Panels[i].Name != p.Name || cfg.Panels[i].Type != p.Type {
			t.Errorf("panel[%d] = %+v, want name/type %s/%s", i, cfg.Panels[i], p.Name, p.Type)
		}
	}
}

func TestValidatePanelsClearsBadModeAndSort(t *testing.T) {
	cfg := loadFromTOML(t, `
[[panels]]
name = "feed"
type = "feed"
mode = "blocked"
sort = "nonsense"

[[panels]]
name = "local"
type = "local"
mode = "mixed"
sort = "size"
`)
	// An invalid mode/sort is cleared, then backfilled to the type's default.
	feed := cfg.Panels[0]
	if feed.Mode != "recommended" {
		t.Errorf("feed invalid mode (blocked) not reset to default: got %q, want recommended", feed.Mode)
	}
	if feed.Sort != "date" {
		t.Errorf("feed invalid sort not reset to default: got %q, want date", feed.Sort)
	}
	local := cfg.Panels[1]
	if local.Mode != "" {
		t.Errorf("local panel kept mode %q (local takes no mode)", local.Mode)
	}
	if local.Sort != "size" {
		t.Errorf("local panel dropped valid sort: got %q, want size", local.Sort)
	}
}

func TestValidatePanelsBackfillsDefaults(t *testing.T) {
	cfg := loadFromTOML(t, `
[[panels]]
name = "feed"
type = "feed"

[[panels]]
name = "local"
type = "local"

[[panels]]
name = "tags"
type = "tags"
`)
	feed, local, tags := cfg.Panels[0], cfg.Panels[1], cfg.Panels[2]
	if feed.Mode != "recommended" || feed.Sort != "date" {
		t.Errorf("feed defaults not backfilled: %+v", feed)
	}
	if local.Mode != "" || local.Sort != "views" {
		t.Errorf("local defaults not backfilled: %+v (want mode='' sort=views)", local)
	}
	if tags.Mode != "subscribed" || tags.Sort != "" {
		t.Errorf("tags defaults not backfilled: %+v (want mode=subscribed sort='')", tags)
	}
}

func TestValidatePanelsKeepsValidModeAndSort(t *testing.T) {
	cfg := loadFromTOML(t, `
[[panels]]
name = "feed"
type = "feed"
mode = "mixed"
sort = "date"

[[panels]]
name = "channels"
type = "channels"
mode = "blocked"
`)
	if cfg.Panels[0].Mode != "mixed" || cfg.Panels[0].Sort != "date" {
		t.Errorf("valid feed mode/sort altered: %+v", cfg.Panels[0])
	}
	if cfg.Panels[1].Mode != "blocked" {
		t.Errorf("valid channels blocked mode altered: %+v", cfg.Panels[1])
	}
}

func TestPruneTabKeysDropsDanglingRefs(t *testing.T) {
	cfg := loadFromTOML(t, `
[[panels]]
name = "myfeed"
type = "feed"

[keybindings.tab_keys]
g = "myfeed"
z = "ghost"
`)
	if cfg.Keybindings.TabKeys["g"] != "myfeed" {
		t.Errorf("valid tab key g -> myfeed was pruned: %v", cfg.Keybindings.TabKeys)
	}
	if _, ok := cfg.Keybindings.TabKeys["z"]; ok {
		t.Errorf("dangling tab key z -> ghost survived: %v", cfg.Keybindings.TabKeys)
	}
}

func TestPanelsRoundTripTOML(t *testing.T) {
	cfg := defaultConfig()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := cfg.save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	var got Config
	if _, err := toml.DecodeFile(path, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(got.Panels, DefaultPanels) {
		t.Errorf("panels did not round-trip: got %v", got.Panels)
	}
	if got.Keybindings.TabKeys["f"] != "feed" {
		t.Errorf("tab_keys did not round-trip: got %v", got.Keybindings.TabKeys)
	}
}
