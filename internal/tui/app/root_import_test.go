package app

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	ovpkg "github.com/EugeneShtoka/yt-tui/internal/tui/overlay"
)

// TestHandleImport_OpensPreviewOverlay verifies the Import key opens the
// import-preview overlay pointed at the data dir. The overlay itself is
// exercised in the overlay package's tests; here we assert only the wiring.
func TestHandleImport_OpensPreviewOverlay(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "yt-tui-export-1.json"), []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	keys := keymap.Build(config.KeyBindings{Import: "I", Close: "esc"})
	r := Root{
		backend: apitest.NopBackend{},
		cfg:     &config.Config{DataDir: dir},
		keys:    keys,
		router:  tabRouter{tabs: []tuipkg.Tab{fakeTab{}}},
	}

	r2, cmd := r.handleKey(tea.KeyPressMsg{Code: 'I', Text: "I"})
	if len(r2.overlays) != 1 {
		t.Fatalf("Import key should open one overlay, got %d", len(r2.overlays))
	}
	if _, ok := r2.overlays[0].(ovpkg.ImportPreview); !ok {
		t.Fatalf("overlay is %T, want ImportPreview", r2.overlays[0])
	}
	if cmd == nil {
		t.Error("expected a background scan command from opening the import overlay")
	}
}
