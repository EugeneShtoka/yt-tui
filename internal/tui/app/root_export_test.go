package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	ovpkg "github.com/EugeneShtoka/yt-tui/internal/tui/overlay"
)

// exportCaptureBackend records the export request and returns a canned bundle.
type exportCaptureBackend struct {
	apitest.NopBackend
	gotOpts portability.ExportOptions
	bundle  portability.Bundle
	err     error
}

func (b *exportCaptureBackend) Export(_ context.Context, opts portability.ExportOptions) (portability.Bundle, error) {
	b.gotOpts = opts
	return b.bundle, b.err
}

// TestHandleExport_OpensOverlay verifies that E opens the export-selection
// overlay and kicks off the bundle fetch (the write itself is driven by the
// overlay and covered in the overlay package's tests).
func TestHandleExport_OpensOverlay(t *testing.T) {
	be := &exportCaptureBackend{bundle: portability.Bundle{SchemaVersion: portability.SchemaVersion}}
	r := Root{backend: be, cfg: &config.Config{DataDir: t.TempDir()}}

	r2, cmd := r.handleExport()
	if len(r2.overlays) != 1 {
		t.Fatalf("want 1 overlay pushed, got %d", len(r2.overlays))
	}
	if _, ok := r2.overlays[0].(ovpkg.ExportSelect); !ok {
		t.Fatalf("want ExportSelect overlay, got %T", r2.overlays[0])
	}
	if cmd == nil {
		t.Fatal("want a non-nil fetch command")
	}
}

// TestHandleExport_ConfigProfileFlowsToBundle drives the overlay end-to-end
// (open → fetch → Enter to write) and asserts the client-marshaled config
// profile lands in the bundle with portable fields in and machine-local out.
func TestHandleExport_ConfigProfileFlowsToBundle(t *testing.T) {
	dir := t.TempDir()
	be := &exportCaptureBackend{bundle: portability.Bundle{SchemaVersion: portability.SchemaVersion}}
	cfg := &config.Config{DataDir: dir}
	cfg.Theme = "gruvbox"
	cfg.FeedMode = "mixed"
	cfg.Player = "vlc" // machine-local — must NOT appear in the bundle
	keys := keymap.Build(config.KeyBindings{Close: "esc", Quit: "q", Up: "k", Down: "j", DrillDown: "enter"})
	r := Root{backend: be, cfg: cfg, keys: keys}

	r2, cmd := r.handleExport()
	// Run the fetch, feed the result back, then Enter writes with defaults.
	loaded, _ := r2.overlays[0].Update(runCmd(cmd))
	_, writeCmd := loaded.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	runCmd(writeCmd)

	// The backend was asked for watch data (the overlay filters client-side).
	if !be.gotOpts.IncludeWatchData {
		t.Error("overlay should fetch with IncludeWatchData=true")
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("want 1 file, got %d", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var round portability.Bundle
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatal(err)
	}
	if len(round.Config) == 0 {
		t.Fatal("exported bundle carries no config profile")
	}
	var p configProfile
	if err := json.Unmarshal(round.Config, &p); err != nil {
		t.Fatalf("config profile is not decodable: %v", err)
	}
	if p.Theme != "gruvbox" || p.FeedMode != "mixed" {
		t.Errorf("config profile missing portable fields: %+v", p)
	}
	if strings.Contains(string(round.Config), "vlc") {
		t.Errorf("machine-local player leaked into exported config: %s", round.Config)
	}
}
