package overlay

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
)

func ipKeys() keymap.KeyMap {
	return keymap.Build(config.KeyBindings{Close: "esc", Quit: "q", Up: "k", Down: "j", DrillDown: "enter"})
}

// importFakeBackend records the preview/apply calls and returns canned results.
type importFakeBackend struct {
	apitest.NopBackend
	previewOpts []portability.ImportOptions
	plan        portability.ImportPlan
	previewErr  error

	applyOpts portability.ImportOptions
	applied   bool
	result    portability.ImportResult
	applyErr  error
}

func (b *importFakeBackend) ImportPreview(_ context.Context, _ portability.Bundle, opts portability.ImportOptions) (portability.ImportPlan, error) {
	b.previewOpts = append(b.previewOpts, opts)
	return b.plan, b.previewErr
}

func (b *importFakeBackend) ImportApply(_ context.Context, _ portability.Bundle, opts portability.ImportOptions) (portability.ImportResult, error) {
	b.applyOpts = opts
	b.applied = true
	return b.result, b.applyErr
}

func runMsg(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

// runBatch flattens one level of tea.Batch, returning every leaf message.
func runBatch(cmd tea.Cmd) []tea.Msg {
	msg := runMsg(cmd)
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	out := make([]tea.Msg, 0, len(batch))
	for _, c := range batch {
		out = append(out, runMsg(c))
	}
	return out
}

func writeBundle(t *testing.T, dir, name string, b portability.Bundle) {
	t.Helper()
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// step drives the overlay Update once and returns the concrete overlay + cmd.
func step(t *testing.T, ip ImportPreview, msg tea.Msg) (ImportPreview, tea.Cmd) {
	t.Helper()
	model, cmd := ip.Update(msg)
	next, ok := model.(ImportPreview)
	if !ok {
		t.Fatalf("Update returned %T, want ImportPreview", model)
	}
	return next, cmd
}

func keyPress(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	default:
		return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	}
}

func TestImportScanListsBundlesNewestFirst(t *testing.T) {
	dir := t.TempDir()
	writeBundle(t, dir, "yt-tui-export-20260101-000000.json", portability.Bundle{SchemaVersion: 1})
	writeBundle(t, dir, "yt-tui-export-20260728-120000.json", portability.Bundle{SchemaVersion: 1})
	// A non-bundle file must be ignored.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	ip, cmd := NewImportPreview(context.Background(), &importFakeBackend{}, ipKeys(), dir, false)
	ip, _ = step(t, ip, runMsg(cmd))

	if len(ip.files) != 2 {
		t.Fatalf("want 2 bundle files, got %v", ip.files)
	}
	if ip.files[0] != "yt-tui-export-20260728-120000.json" {
		t.Errorf("newest bundle should sort first, got %v", ip.files)
	}
}

func TestImportSelectFileLoadsPreview(t *testing.T) {
	dir := t.TempDir()
	writeBundle(t, dir, "yt-tui-export-20260728-120000.json", portability.Bundle{
		SchemaVersion: 1,
		Channels:      []portability.ChannelExport{{ChannelID: "c1"}},
	})
	be := &importFakeBackend{plan: portability.ImportPlan{SchemaVersion: 1, Compatible: true, NewChannels: 1}}

	ip, cmd := NewImportPreview(context.Background(), be, ipKeys(), dir, false)
	ip, _ = step(t, ip, runMsg(cmd)) // files loaded

	// Enter selects the (only) file and triggers load + preview.
	ip, cmd = step(t, ip, keyPress("enter"))
	ip, _ = step(t, ip, runMsg(cmd)) // importPreviewMsg

	if ip.stage != importStagePreview {
		t.Fatalf("stage = %v, want preview", ip.stage)
	}
	if ip.plan.NewChannels != 1 {
		t.Errorf("plan not stored: %+v", ip.plan)
	}
	if len(be.previewOpts) != 1 {
		t.Fatalf("ImportPreview called %d times, want 1", len(be.previewOpts))
	}
}

func TestImportToggleRePreviewsWithNewOpts(t *testing.T) {
	be := &importFakeBackend{plan: portability.ImportPlan{SchemaVersion: 1, Compatible: true}}
	ip := ImportPreview{
		backend: be,
		keys:    ipKeys(),
		stage:   importStagePreview,
		plan:    be.plan,
		bundle:  portability.Bundle{SchemaVersion: 1},
		rowSel:  ipRowWatch,
	}

	ip, cmd := step(t, ip, keyPress("enter")) // toggle IncludeWatchData
	if !ip.opts.IncludeWatchData {
		t.Fatal("IncludeWatchData should be on after toggle")
	}
	ip, _ = step(t, ip, runMsg(cmd)) // re-preview
	if len(be.previewOpts) != 1 || !be.previewOpts[0].IncludeWatchData {
		t.Fatalf("re-preview opts = %+v, want IncludeWatchData", be.previewOpts)
	}

	// Toggle ConvertYTToLocal too.
	ip.rowSel = ipRowConvert
	ip, cmd = step(t, ip, keyPress("enter"))
	if !ip.opts.ConvertYTToLocal {
		t.Fatal("ConvertYTToLocal should be on after toggle")
	}
	_, _ = step(t, ip, runMsg(cmd))
	if !be.previewOpts[1].ConvertYTToLocal || !be.previewOpts[1].IncludeWatchData {
		t.Fatalf("second re-preview opts = %+v, want both toggles on", be.previewOpts[1])
	}
}

func TestImportApplyRunsAndClosesOverlay(t *testing.T) {
	be := &importFakeBackend{
		plan:   portability.ImportPlan{SchemaVersion: 1, Compatible: true},
		result: portability.ImportResult{ChannelsUpserted: 3, PlaylistAdds: 5},
	}
	ip := ImportPreview{
		backend: be,
		keys:    ipKeys(),
		stage:   importStagePreview,
		plan:    be.plan,
		opts:    portability.ImportOptions{ConvertYTToLocal: true},
		bundle:  portability.Bundle{SchemaVersion: 1},
		rowSel:  ipRowApply,
	}

	ip, cmd := step(t, ip, keyPress("enter")) // apply
	if !ip.applying {
		t.Fatal("overlay should be in applying state")
	}
	ip, cmd = step(t, ip, runMsg(cmd)) // importAppliedMsg
	if !be.applied || !be.applyOpts.ConvertYTToLocal {
		t.Fatalf("ImportApply not called with opts: applied=%v opts=%+v", be.applied, be.applyOpts)
	}

	msgs := runBatch(cmd)
	var sawStatus, sawPop bool
	for _, m := range msgs {
		switch v := m.(type) {
		case tuipkg.StatusMsg:
			sawStatus = true
			if v.IsErr {
				t.Errorf("apply status unexpectedly error: %q", v.Text)
			}
		case PopOverlayMsg:
			sawPop = true
		}
	}
	if !sawStatus || !sawPop {
		t.Errorf("apply should emit StatusMsg + PopOverlayMsg, got status=%v pop=%v", sawStatus, sawPop)
	}
}

func TestImportConfigToggleFlipsWithoutRePreview(t *testing.T) {
	be := &importFakeBackend{plan: portability.ImportPlan{SchemaVersion: 1, Compatible: true}}
	ip := ImportPreview{
		backend: be,
		keys:    ipKeys(),
		stage:   importStagePreview,
		plan:    be.plan,
		bundle:  portability.Bundle{SchemaVersion: 1, Config: json.RawMessage(`{"theme":"x"}`)},
		rowSel:  ipRowConfig,
	}

	ip, cmd := step(t, ip, keyPress("enter")) // toggle ApplyConfig
	if !ip.opts.ApplyConfig {
		t.Fatal("ApplyConfig should be on after toggle")
	}
	if cmd != nil {
		t.Error("config toggle is client-side and must not trigger a re-preview")
	}
	if len(be.previewOpts) != 0 {
		t.Errorf("config toggle must not call ImportPreview, got %d calls", len(be.previewOpts))
	}
}

func TestImportApplyEmitsConfigProfileWhenOptedIn(t *testing.T) {
	rawCfg := json.RawMessage(`{"theme":"gruvbox"}`)
	be := &importFakeBackend{plan: portability.ImportPlan{SchemaVersion: 1, Compatible: true}}
	ip := ImportPreview{
		backend: be,
		keys:    ipKeys(),
		stage:   importStagePreview,
		plan:    be.plan,
		opts:    portability.ImportOptions{ApplyConfig: true},
		bundle:  portability.Bundle{SchemaVersion: 1, Config: rawCfg},
		rowSel:  ipRowApply,
	}

	_, cmd := step(t, ip, keyPress("enter")) // apply
	_, cmd = step(t, ip, runMsg(cmd))        // importAppliedMsg

	var got *ApplyConfigProfileMsg
	for _, m := range runBatch(cmd) {
		if v, ok := m.(ApplyConfigProfileMsg); ok {
			got = &v
		}
	}
	if got == nil {
		t.Fatal("apply with ApplyConfig on + config present should emit ApplyConfigProfileMsg")
	}
	if string(got.Config) != string(rawCfg) {
		t.Errorf("carried config = %s, want %s", got.Config, rawCfg)
	}
}

func TestImportApplyOmitsConfigProfileWhenNotOptedInOrAbsent(t *testing.T) {
	cases := []struct {
		name   string
		opts   portability.ImportOptions
		bundle portability.Bundle
	}{
		{"opted-out", portability.ImportOptions{ApplyConfig: false}, portability.Bundle{SchemaVersion: 1, Config: json.RawMessage(`{"theme":"x"}`)}},
		{"no-config-in-bundle", portability.ImportOptions{ApplyConfig: true}, portability.Bundle{SchemaVersion: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			be := &importFakeBackend{plan: portability.ImportPlan{SchemaVersion: 1, Compatible: true}}
			ip := ImportPreview{
				backend: be, keys: ipKeys(), stage: importStagePreview,
				plan: be.plan, opts: tc.opts, bundle: tc.bundle, rowSel: ipRowApply,
			}
			_, cmd := step(t, ip, keyPress("enter"))
			_, cmd = step(t, ip, runMsg(cmd))
			for _, m := range runBatch(cmd) {
				if _, ok := m.(ApplyConfigProfileMsg); ok {
					t.Fatalf("did not expect ApplyConfigProfileMsg for case %q", tc.name)
				}
			}
		})
	}
}

func TestImportApplyBlockedOnIncompatible(t *testing.T) {
	be := &importFakeBackend{}
	ip := ImportPreview{
		backend: be,
		keys:    ipKeys(),
		stage:   importStagePreview,
		plan:    portability.ImportPlan{SchemaVersion: 99, Compatible: false},
		rowSel:  ipRowApply,
	}
	_, cmd := step(t, ip, keyPress("enter"))
	if be.applied {
		t.Fatal("ImportApply must not run for an incompatible bundle")
	}
	status, ok := runMsg(cmd).(tuipkg.StatusMsg)
	if !ok || !status.IsErr {
		t.Fatalf("want error StatusMsg, got %#v", runMsg(cmd))
	}
}

func TestImportEscBackThenPop(t *testing.T) {
	ip := ImportPreview{keys: ipKeys(), stage: importStagePreview, err: errors.New("x")}
	// Esc in preview returns to the picker and clears the error.
	ip, cmd := step(t, ip, keyPress("esc"))
	if ip.stage != importStagePick {
		t.Fatalf("Esc in preview should return to pick, stage=%v", ip.stage)
	}
	if ip.err != nil {
		t.Errorf("Esc should clear the error, got %v", ip.err)
	}
	if cmd != nil {
		t.Errorf("Esc back to pick should not emit a command")
	}
	// Esc in the picker pops the overlay.
	_, cmd = step(t, ip, keyPress("esc"))
	if _, ok := runMsg(cmd).(PopOverlayMsg); !ok {
		t.Fatalf("Esc in pick should pop overlay, got %#v", runMsg(cmd))
	}
}

func TestImportRenderRectangular(t *testing.T) {
	const width, height = 100, 30
	ip := ImportPreview{
		keys:  ipKeys(),
		stage: importStagePreview,
		plan:  portability.ImportPlan{SchemaVersion: 1, Compatible: true, NewChannels: 3, PlaylistAdds: 5},
	}
	out := ip.Render(rectangularBehind(width, height), width, height)
	assertRectangular(t, out, width, height)
}

func TestSummarizeImport(t *testing.T) {
	if got := summarizeImport(portability.ImportResult{}); got != "Import: already up to date (nothing new)" {
		t.Errorf("empty result summary = %q", got)
	}
	got := summarizeImport(portability.ImportResult{ChannelsUpserted: 2, HistoryAdded: 4})
	if got != "Imported: 2 channels, 4 history" {
		t.Errorf("summary = %q", got)
	}
}
