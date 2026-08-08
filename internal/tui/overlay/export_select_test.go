package overlay

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

var errUnavailable = errors.New("backend unavailable")

// exportFakeBackend returns a canned full bundle from Export.
type exportFakeBackend struct {
	apitest.NopBackend
	bundle portability.Bundle
	err    error
}

func (b *exportFakeBackend) Export(_ context.Context, _ portability.ExportOptions) (portability.Bundle, error) {
	return b.bundle, b.err
}

// fullBundle populates every section so filtering is observable.
func fullBundle() portability.Bundle {
	return portability.Bundle{
		SchemaVersion: 1,
		Channels:      []portability.ChannelExport{{}, {}},
		BlockedNames:  []string{"spam"},
		Playlists:     []portability.PlaylistExport{{}},
		YTPlaylists:   []portability.YTPlaylistRef{{}},
		Videos:        []portability.VideoExport{{}},
		History:       []portability.HistoryExport{{}, {}, {}},
		Positions:     []portability.PositionExport{{}},
	}
}

func stepES(t *testing.T, es ExportSelect, msg tea.Msg) (ExportSelect, tea.Cmd) {
	t.Helper()
	model, cmd := es.Update(msg)
	next, ok := model.(ExportSelect)
	if !ok {
		t.Fatalf("Update returned %T, want ExportSelect", model)
	}
	return next, cmd
}

func spaceKey() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeySpace} }

// newLoadedExport builds the overlay and drives the initial fetch so the
// returned overlay is in its loaded state. config controls the config section.
func newLoadedExport(t *testing.T, dir string, be *exportFakeBackend, config json.RawMessage) ExportSelect {
	t.Helper()
	es, cmd := NewExportSelect(context.Background(), be, ipKeys(), dir, config, nil, false)
	es, _ = stepES(t, es, runMsg(cmd)) // exportBundleLoadedMsg
	if !es.loaded {
		t.Fatal("overlay did not reach loaded state")
	}
	return es
}

// readOnlyExport returns the single written bundle file's decoded contents.
func readOnlyExport(t *testing.T, dir string) portability.Bundle {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "yt-tui-export-") && strings.HasSuffix(e.Name(), ".json") {
			if found != "" {
				t.Fatalf("expected exactly one export file, found %q and %q", found, e.Name())
			}
			found = e.Name()
		}
	}
	if found == "" {
		t.Fatal("no export file written")
	}
	data, err := os.ReadFile(filepath.Join(dir, found)) //nolint:gosec // test-controlled dir
	if err != nil {
		t.Fatal(err)
	}
	var b portability.Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestExportDefaultsWriteEverythingButWatchData(t *testing.T) {
	dir := t.TempDir()
	es := newLoadedExport(t, dir, &exportFakeBackend{bundle: fullBundle()}, json.RawMessage(`{"a":1}`))

	// Navigate to the Export row and confirm with Enter (defaults untouched).
	for es.rowSel < esRowExport {
		es, _ = stepES(t, es, keyPress("j"))
	}
	_, cmd := stepES(t, es, keyPress("enter"))
	runMsg(cmd) // executes the write

	got := readOnlyExport(t, dir)
	if len(got.Channels) != 2 || len(got.BlockedNames) != 1 || len(got.Playlists) != 1 {
		t.Errorf("portable sections should be included: %+v", got)
	}
	if len(got.History) != 0 || len(got.Positions) != 0 {
		t.Errorf("watch data should be excluded by default, got %d history / %d positions", len(got.History), len(got.Positions))
	}
	if len(got.Config) == 0 {
		t.Error("config profile should be included by default when present")
	}
}

func TestExportSpaceTogglesSections(t *testing.T) {
	dir := t.TempDir()
	es := newLoadedExport(t, dir, &exportFakeBackend{bundle: fullBundle()}, json.RawMessage(`{"a":1}`))

	// Space on the Channels row (rowSel 0) turns it off.
	es, _ = stepES(t, es, spaceKey())
	if es.sel.channels {
		t.Error("space should have toggled channels off")
	}
	// Move to the Watch row and turn it on.
	for es.rowSel < esRowWatch {
		es, _ = stepES(t, es, keyPress("j"))
	}
	es, _ = stepES(t, es, spaceKey())
	if !es.sel.watch {
		t.Error("space should have toggled watch data on")
	}

	// Export and verify the filtered result matches the toggles.
	for es.rowSel < esRowExport {
		es, _ = stepES(t, es, keyPress("j"))
	}
	_, cmd := stepES(t, es, keyPress("enter"))
	runMsg(cmd) // executes the write

	got := readOnlyExport(t, dir)
	if len(got.Channels) != 0 {
		t.Errorf("channels toggled off should be excluded, got %d", len(got.Channels))
	}
	if len(got.History) != 3 || len(got.Positions) != 1 {
		t.Errorf("watch data toggled on should be included, got %d history / %d positions", len(got.History), len(got.Positions))
	}
}

func TestExportEnterConfirmsWithoutTogglingFocusedRow(t *testing.T) {
	dir := t.TempDir()
	es := newLoadedExport(t, dir, &exportFakeBackend{bundle: fullBundle()}, nil)

	// Enter on the Channels row must export (not toggle it off) — Space is the
	// selection key, Enter confirms. Channels stays included.
	if es.rowSel != esRowChannels {
		t.Fatalf("expected initial focus on channels row, got %d", es.rowSel)
	}
	_, cmd := stepES(t, es, keyPress("enter"))
	runMsg(cmd) // executes the write

	got := readOnlyExport(t, dir)
	if len(got.Channels) != 2 {
		t.Errorf("enter should confirm without toggling channels off, got %d", len(got.Channels))
	}
}

func TestExportFetchErrorBlocksWrite(t *testing.T) {
	dir := t.TempDir()
	es, cmd := NewExportSelect(context.Background(), &exportFakeBackend{err: errUnavailable}, ipKeys(), dir, nil, nil, false)
	es, _ = stepES(t, es, runMsg(cmd)) // exportBundleLoadedMsg with err

	if es.loaded {
		t.Error("overlay must not be loaded after a fetch error")
	}
	// Enter must not write a partial bundle while unloaded.
	_, wcmd := stepES(t, es, keyPress("enter"))
	runMsg(wcmd)
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("no file should be written after a fetch error, found %d entries", len(entries))
	}
	// The error is surfaced in the rendered box.
	view := es.Render("", 80, 24)
	if !strings.Contains(view, "unavailable") {
		t.Errorf("render should show the fetch error, got:\n%s", view)
	}
}

func TestExportNothingSelectedReportsAndSkipsWrite(t *testing.T) {
	dir := t.TempDir()
	es := newLoadedExport(t, dir, &exportFakeBackend{bundle: fullBundle()}, nil)

	// Turn every section off, then try to export.
	es.sel = exportSel{}
	for es.rowSel < esRowExport {
		es, _ = stepES(t, es, keyPress("j"))
	}
	_, cmd := stepES(t, es, keyPress("enter"))

	msg := runMsg(cmd)
	status, ok := msg.(tuipkg.StatusMsg)
	if !ok || !status.IsErr {
		t.Fatalf("expected an error StatusMsg, got %#v", msg)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("no file should be written when nothing is selected, found %d entries", len(entries))
	}
}
