package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	ovpkg "github.com/EugeneShtoka/yt-tui/internal/tui/overlay"
)

func TestHandleApplyConfigProfile_OverwritesPortableKeepsLocalAndSaves(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.toml")
	cfg := &config.Config{ConfigFile: cfgFile}
	cfg.Player = "mpv"           // machine-local — must survive
	cfg.DownloadDir = "/keep/me" // machine-local — must survive
	r := Root{cfg: cfg}

	raw, err := json.Marshal(configProfile{Theme: "gruvbox", HintMode: "minimal", FeedMode: "mixed"})
	if err != nil {
		t.Fatal(err)
	}

	_, cmd := r.handleApplyConfigProfile(ovpkg.ApplyConfigProfileMsg{Config: raw})
	status, ok := runCmd(cmd).(tuipkg.StatusMsg)
	if !ok || status.IsErr {
		t.Fatalf("want non-error StatusMsg, got %#v", runCmd(cmd))
	}
	if !strings.Contains(status.Text, "restart") {
		t.Errorf("status should mention restart, got %q", status.Text)
	}

	if cfg.Theme != "gruvbox" || cfg.HintMode != "minimal" || cfg.FeedMode != "mixed" {
		t.Errorf("portable fields not applied: theme=%q hint=%q feed=%q", cfg.Theme, cfg.HintMode, cfg.FeedMode)
	}
	if cfg.Player != "mpv" || cfg.DownloadDir != "/keep/me" {
		t.Errorf("machine-local fields overwritten: player=%q dir=%q", cfg.Player, cfg.DownloadDir)
	}
	// Normalize ran (empty panels fall back to defaults), and the config was saved.
	if len(cfg.Panels) == 0 {
		t.Error("Normalize should have backfilled default panels")
	}
	if _, err := os.Stat(cfgFile); err != nil {
		t.Errorf("config file was not saved: %v", err)
	}
}

func TestHandleApplyConfigProfile_BadJSONSurfacesError(t *testing.T) {
	r := Root{cfg: &config.Config{ConfigFile: filepath.Join(t.TempDir(), "config.toml")}}
	_, cmd := r.handleApplyConfigProfile(ovpkg.ApplyConfigProfileMsg{Config: []byte("{not json")})
	status, ok := runCmd(cmd).(tuipkg.StatusMsg)
	if !ok || !status.IsErr {
		t.Fatalf("want error StatusMsg, got %#v", runCmd(cmd))
	}
}
