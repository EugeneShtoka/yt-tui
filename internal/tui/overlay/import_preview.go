package overlay

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

// ── private messages ──────────────────────────────────────────────────────────

// importFilesLoadedMsg carries the bundle files discovered in the data dir.
type importFilesLoadedMsg struct {
	tuipkg.OverlayTarget
	files []string
	err   error
}

// importPreviewMsg carries the dry-run plan for a loaded bundle. It is produced
// both by the initial file load and by a re-preview after a toggle changes.
type importPreviewMsg struct {
	tuipkg.OverlayTarget
	path   string
	bundle portability.Bundle
	plan   portability.ImportPlan
	opts   portability.ImportOptions
	err    error
}

// importAppliedMsg carries the outcome of ImportApply.
type importAppliedMsg struct {
	tuipkg.OverlayTarget
	result portability.ImportResult
	err    error
}

// ── ImportPreview ─────────────────────────────────────────────────────────────

type importStage int

const (
	importStagePick    importStage = iota // choosing a bundle file
	importStagePreview                    // reviewing the plan + toggles
)

// preview-stage row indices (the interactive list under the counts).
const (
	ipRowConvert = iota // toggle ConvertYTToLocal
	ipRowWatch          // toggle IncludeWatchData
	ipRowConfig         // toggle ApplyConfig (overwrite local portable config)
	ipRowApply          // run ImportApply
	ipRowCount
)

// ImportPreview is the import overlay: it lists exported bundle files, shows a
// non-mutating dry-run diff (ImportPlan) of the selected one with the two
// opt-in toggles, and applies it. It reuses moveVertical + placeOverlayBox like
// the other overlay selectors and never touches the DB itself — the backend
// (inproc DB read / remote RPC) computes both the preview and the apply.
type ImportPreview struct {
	identity
	ctx      context.Context
	backend  api.PortabilityBackend
	keys     keymap.KeyMap
	circular bool
	dir      string // data dir scanned for *.json bundle files

	stage importStage

	// pick stage
	files   []string
	fileSel int

	// preview stage
	bundlePath string
	bundle     portability.Bundle
	plan       portability.ImportPlan
	opts       portability.ImportOptions
	rowSel     int
	applying   bool
	err        error
}

// NewImportPreview creates the import overlay and kicks off a background scan of
// dir for exported bundle files.
func NewImportPreview(ctx context.Context, backend api.PortabilityBackend, keys keymap.KeyMap, dir string, circular bool) (ImportPreview, tea.Cmd) {
	ip := ImportPreview{
		identity: newIdentity(),
		ctx:      ctx,
		backend:  backend,
		keys:     keys,
		circular: circular,
		dir:      dir,
		stage:    importStagePick,
	}
	return ip, ip.scanCmd()
}

// ── overlay.Overlay interface ─────────────────────────────────────────────────

func (ip ImportPreview) InterceptsInput() bool { return false }
func (ip ImportPreview) WidthReduction() int   { return 0 }
func (ip ImportPreview) HasFocus() bool        { return true }

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (ip ImportPreview) Init() tea.Cmd  { return nil }
func (ip ImportPreview) View() tea.View { return tea.NewView("") } // rendering done via Render(behind,...)

func (ip ImportPreview) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case importFilesLoadedMsg:
		ip.files = m.files
		if ip.fileSel >= len(ip.files) {
			ip.fileSel = 0
		}
		if m.err != nil {
			ip.err = m.err
		}
		return ip, nil

	case importPreviewMsg:
		ip.stage = importStagePreview
		ip.bundlePath = m.path
		ip.bundle = m.bundle
		ip.plan = m.plan
		ip.opts = m.opts
		ip.err = m.err
		return ip, nil

	case importAppliedMsg:
		return ip.handleApplied(m)

	case tea.KeyPressMsg:
		return ip.handleKey(m)
	}
	return ip, nil
}

func (ip ImportPreview) handleApplied(m importAppliedMsg) (tea.Model, tea.Cmd) {
	ip.applying = false
	if m.err != nil {
		ip.err = m.err
		return ip, func() tea.Msg {
			return tuipkg.StatusMsg{Text: "import: " + m.err.Error(), IsErr: true}
		}
	}
	summary := summarizeImport(m.result)
	cmds := []tea.Cmd{
		func() tea.Msg { return tuipkg.StatusMsg{Text: summary} },
	}
	// Config lives on the client, so Root (which owns it) applies it on the main
	// loop after the data import succeeds. Only when the user opted in and the
	// bundle actually carries a profile.
	if ip.opts.ApplyConfig && ip.hasConfig() {
		cfg := ip.bundle.Config
		cmds = append(cmds, func() tea.Msg { return ApplyConfigProfileMsg{Config: cfg} })
	}
	cmds = append(cmds, func() tea.Msg { return PopOverlayMsg{} })
	return ip, tea.Batch(cmds...)
}

// hasConfig reports whether the loaded bundle carries a portable config profile.
func (ip ImportPreview) hasConfig() bool { return len(ip.bundle.Config) > 0 }

// ── key handling ──────────────────────────────────────────────────────────────

func (ip ImportPreview) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if ip.stage == importStagePick {
		return ip.handlePickKey(msg)
	}
	return ip.handlePreviewKey(msg)
}

func (ip ImportPreview) handlePickKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if newSel, consumed := moveVertical(ip.fileSel, len(ip.files), msg, ip.keys, ip.circular, true); consumed {
		ip.fileSel = newSel
		return ip, nil
	}
	switch {
	case key.Matches(msg, ip.keys.Escape), key.Matches(msg, ip.keys.Quit):
		return ip, func() tea.Msg { return PopOverlayMsg{} }
	case key.Matches(msg, ip.keys.DrillDown):
		if len(ip.files) == 0 {
			return ip, nil
		}
		path := filepath.Join(ip.dir, ip.files[ip.fileSel])
		return ip, ip.loadAndPreviewCmd(path, ip.opts)
	}
	return ip, nil
}

func (ip ImportPreview) handlePreviewKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if newSel, consumed := moveVertical(ip.rowSel, ipRowCount, msg, ip.keys, ip.circular, true); consumed {
		ip.rowSel = newSel
		return ip, nil
	}
	switch {
	case key.Matches(msg, ip.keys.Escape):
		// Back to the file picker (Esc from the picker pops the overlay).
		ip.stage = importStagePick
		ip.err = nil
		return ip, nil
	case key.Matches(msg, ip.keys.Quit):
		return ip, func() tea.Msg { return PopOverlayMsg{} }
	case key.Matches(msg, ip.keys.DrillDown):
		return ip.activateRow()
	}
	return ip, nil
}

// activateRow toggles a checkbox (re-previewing to refresh the counts) or runs
// the import when the Apply row is selected.
func (ip ImportPreview) activateRow() (tea.Model, tea.Cmd) {
	switch ip.rowSel {
	case ipRowConvert:
		ip.opts.ConvertYTToLocal = !ip.opts.ConvertYTToLocal
		return ip, ip.previewCmd(ip.opts)
	case ipRowWatch:
		ip.opts.IncludeWatchData = !ip.opts.IncludeWatchData
		return ip, ip.previewCmd(ip.opts)
	case ipRowConfig:
		// Config is applied client-side and doesn't affect the backend's data
		// diff, so flip locally without a re-preview.
		ip.opts.ApplyConfig = !ip.opts.ApplyConfig
		return ip, nil
	case ipRowApply:
		if !ip.plan.Compatible {
			return ip, func() tea.Msg {
				return tuipkg.StatusMsg{Text: "import: incompatible bundle schema", IsErr: true}
			}
		}
		if ip.applying {
			return ip, nil
		}
		ip.applying = true
		return ip, ip.applyCmd()
	}
	return ip, nil
}

// ── commands ──────────────────────────────────────────────────────────────────

func (ip ImportPreview) scanCmd() tea.Cmd {
	dir := ip.dir
	target := tuipkg.OverlayTarget{ID: ip.ID()}
	return func() tea.Msg {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return importFilesLoadedMsg{OverlayTarget: target, err: err}
		}
		var files []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				files = append(files, e.Name())
			}
		}
		// Timestamped export names sort lexicographically, so reverse order puts
		// the most recent bundle first.
		sort.Sort(sort.Reverse(sort.StringSlice(files)))
		return importFilesLoadedMsg{OverlayTarget: target, files: files}
	}
}

// loadAndPreviewCmd reads + decodes the bundle at path, then computes its plan.
func (ip ImportPreview) loadAndPreviewCmd(path string, opts portability.ImportOptions) tea.Cmd {
	backend, ctx := ip.backend, ip.ctx
	target := tuipkg.OverlayTarget{ID: ip.ID()}
	return func() tea.Msg {
		data, err := os.ReadFile(path) //nolint:gosec // path is a user-selected file in the data dir
		if err != nil {
			return importPreviewMsg{OverlayTarget: target, path: path, opts: opts, err: err}
		}
		var b portability.Bundle
		if uerr := json.Unmarshal(data, &b); uerr != nil {
			return importPreviewMsg{OverlayTarget: target, path: path, opts: opts, err: fmt.Errorf("decode bundle: %w", uerr)}
		}
		plan, err := backend.ImportPreview(ctx, b, opts)
		return importPreviewMsg{OverlayTarget: target, path: path, bundle: b, plan: plan, opts: opts, err: err}
	}
}

// previewCmd re-computes the plan for the already-loaded bundle with new opts.
func (ip ImportPreview) previewCmd(opts portability.ImportOptions) tea.Cmd {
	backend, b, path, ctx := ip.backend, ip.bundle, ip.bundlePath, ip.ctx
	target := tuipkg.OverlayTarget{ID: ip.ID()}
	return func() tea.Msg {
		plan, err := backend.ImportPreview(ctx, b, opts)
		return importPreviewMsg{OverlayTarget: target, path: path, bundle: b, plan: plan, opts: opts, err: err}
	}
}

func (ip ImportPreview) applyCmd() tea.Cmd {
	backend, b, opts, ctx := ip.backend, ip.bundle, ip.opts, ip.ctx
	target := tuipkg.OverlayTarget{ID: ip.ID()}
	return func() tea.Msg {
		res, err := backend.ImportApply(ctx, b, opts)
		return importAppliedMsg{OverlayTarget: target, result: res, err: err}
	}
}

// ── rendering ─────────────────────────────────────────────────────────────────

const importBoxW = 56

func (ip ImportPreview) Render(behind string, width, _ int) string {
	var content string
	if ip.stage == importStagePick {
		content = ip.renderPick()
	} else {
		content = ip.renderPreview()
	}
	return placeOverlayBox(behind, content, width, importBoxW)
}

func (ip ImportPreview) renderPick() string {
	const innerW = importBoxW - 6
	lines := []string{styles.Bold.Render("Import bundle"), ""}
	if len(ip.files) == 0 {
		lines = append(lines, styles.Help.Render("No export bundles found in"), styles.Help.Render(render.Truncate(ip.dir, innerW)))
	} else {
		for i, f := range ip.files {
			lines = append(lines, importRow(i == ip.fileSel, render.Truncate(f, innerW-2)))
		}
	}
	lines = append(lines, "", importHint(innerW, "j/k: move  enter: open", ip.keys.Escape.Help().Key+": cancel"))
	return strings.Join(lines, "\n")
}

func (ip ImportPreview) renderPreview() string {
	const innerW = importBoxW - 6
	title := fmt.Sprintf("Import preview  (schema v%d)", ip.plan.SchemaVersion)
	lines := []string{styles.Bold.Render(title), ""}

	if ip.err != nil {
		lines = append(lines,
			styles.Error.Render(render.Truncate(ip.err.Error(), innerW)),
			"",
			importHint(innerW, "", ip.keys.Escape.Help().Key+": back"),
		)
		return strings.Join(lines, "\n")
	}
	if !ip.plan.Compatible {
		lines = append(lines, styles.Warning.Render("Incompatible bundle schema — cannot import."), "")
	}

	lines = append(lines, ip.planLines()...)
	lines = append(lines,
		"",
		importRow(ip.rowSel == ipRowConvert, importCheckbox(ip.opts.ConvertYTToLocal, "Convert YouTube subs → local")),
		importRow(ip.rowSel == ipRowWatch, importCheckbox(ip.opts.IncludeWatchData, ip.watchToggleLabel())),
		importRow(ip.rowSel == ipRowConfig, importCheckbox(ip.opts.ApplyConfig, ip.configToggleLabel())),
		"",
		importRow(ip.rowSel == ipRowApply, "Apply import"),
		"",
		importHint(innerW, "j/k: move  enter: toggle/apply", ip.keys.Escape.Help().Key+": back"),
	)
	return strings.Join(lines, "\n")
}

// planLines renders the non-mutating diff counts. Zero-valued sections are still
// shown so the user sees the full shape of the import.
func (ip ImportPreview) planLines() []string {
	p := ip.plan
	configState := "none"
	if ip.hasConfig() {
		configState = "present"
	}
	return []string{
		styles.Help.Render(fmt.Sprintf("Channels     %d new · %d updated · %d blocked", p.NewChannels, p.UpdatedChannels, p.BlockedChannels)),
		styles.Help.Render(fmt.Sprintf("Blocked names %d new", p.NewBlockedNames)),
		styles.Help.Render(fmt.Sprintf("Playlists    %d new · %d merged · %d adds", p.NewPlaylists, p.MergedPlaylists, p.PlaylistAdds)),
		styles.Help.Render(fmt.Sprintf("Videos       %d · watch-later %d · YT lists %d", p.Videos, p.NewWatchLater, p.NewYTPlaylists)),
		styles.Help.Render(fmt.Sprintf("Watch data   %d history · %d positions", p.NewHistory, p.NewPositions)),
		styles.Help.Render(fmt.Sprintf("Config       %s", configState)),
	}
}

// watchToggleLabel annotates the watch-data toggle so the user knows whether the
// bundle even carries watch data to merge.
func (ip ImportPreview) watchToggleLabel() string {
	if len(ip.bundle.History) == 0 && len(ip.bundle.Positions) == 0 {
		return "Include watch data (none in bundle)"
	}
	return "Include watch data"
}

// configToggleLabel annotates the apply-config toggle so the user knows whether
// the bundle even carries a config profile to apply. Applying overwrites the
// local portable config wholesale (a restart is needed to take full effect).
func (ip ImportPreview) configToggleLabel() string {
	if !ip.hasConfig() {
		return "Apply config profile (none in bundle)"
	}
	return "Apply config profile (overwrites local)"
}

// ── small render helpers ──────────────────────────────────────────────────────

func importRow(selected bool, text string) string {
	if selected {
		return styles.Selected.Render("▶ " + text)
	}
	return "  " + text
}

func importCheckbox(on bool, label string) string {
	mark := " "
	if on {
		mark = "x"
	}
	return "[" + mark + "] " + label
}

func importHint(innerW int, left, right string) string {
	return styles.Help.Render(render.JustifyEnds(left, right, innerW))
}

// summarizeImport turns an ImportResult into a one-line status message, listing
// only the non-zero sections. An all-zero result (re-import of the same bundle)
// reports that nothing changed.
func summarizeImport(r portability.ImportResult) string {
	parts := make([]string, 0, 9)
	add := func(n int, unit string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, unit))
		}
	}
	add(r.ChannelsUpserted, "channels")
	add(r.BlockedNames, "blocked names")
	add(r.PlaylistsTouched, "playlists")
	add(r.PlaylistAdds, "playlist adds")
	add(r.VideosUpserted, "videos")
	add(r.WatchLaterAdded, "watch-later")
	add(r.YTPlaylists, "YT playlists")
	add(r.HistoryAdded, "history")
	add(r.PositionsSet, "positions")
	if len(parts) == 0 {
		return "Import: already up to date (nothing new)"
	}
	return "Imported: " + strings.Join(parts, ", ")
}
