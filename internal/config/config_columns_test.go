package config

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/BurntSushi/toml"
)

// A per-panel [columns] table parses into ClientConfig.Columns keyed by panel
// name, preserving both membership and order.
func TestColumnsParseFromTOML(t *testing.T) {
	const src = `
[columns]
feed = ["num", "title", "date"]
local = ["title", "size"]
`
	var cfg Config
	if err := toml.Unmarshal([]byte(src), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := cfg.Columns["feed"]; !reflect.DeepEqual(got, []string{"num", "title", "date"}) {
		t.Errorf("feed columns = %v", got)
	}
	if got := cfg.Columns["local"]; !reflect.DeepEqual(got, []string{"title", "size"}) {
		t.Errorf("local columns = %v", got)
	}
}

// The Columns map survives a save → decode round trip verbatim.
func TestColumnsRoundTripTOML(t *testing.T) {
	cfg := defaultConfig()
	cfg.Columns = map[string][]string{
		"feed":  {"num", "title", "views"},
		"local": {"title"},
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := cfg.save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	var got Config
	if _, err := toml.DecodeFile(path, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(got.Columns, cfg.Columns) {
		t.Errorf("columns did not round-trip: got %v, want %v", got.Columns, cfg.Columns)
	}
}

// An absent [columns] table leaves the map nil (nil = show all columns
// everywhere, the default that preserves today's look).
func TestColumnsAbsentIsNil(t *testing.T) {
	var cfg Config
	if err := toml.Unmarshal([]byte("player = \"mpv\"\n"), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg.Columns != nil {
		t.Errorf("absent [columns] should leave Columns nil, got %v", cfg.Columns)
	}
}
