package overlay

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

// exportToggleKey is the Space binding that flips the focused section checkbox.
// It is overlay-internal UX (not a remappable global), so it lives here rather
// than in the config-driven KeyMap; Enter stays reserved for the export action.
var exportToggleKey = key.NewBinding(key.WithKeys(" ", "space"))

// ── private messages ──────────────────────────────────────────────────────────

// exportBundleLoadedMsg carries the full backend export bundle (fetched once
// with watch data included, so section toggles apply instantly client-side).
type exportBundleLoadedMsg struct {
	tuipkg.OverlayTarget
	bundle portability.Bundle
	err    error
}

// exportWrittenMsg carries the outcome of writing the (filtered) bundle to disk.
type exportWrittenMsg struct {
	tuipkg.OverlayTarget
	path string
	err  error
}

// ── ExportSelect ──────────────────────────────────────────────────────────────

// section-row indices (the interactive list of includable sections + action).
const (
	esRowChannels  = iota // toggle include Channels
	esRowBlocklist        // toggle include BlockedNames
	esRowPlaylists        // toggle include Playlists + YTPlaylists + Videos
	esRowWatch            // toggle include History + Positions
	esRowConfig           // toggle include the portable config profile
	esRowExport           // write the bundle
	esRowCount
)

// exportSel records which sections the user has opted to include.
type exportSel struct {
	channels, blocklist, playlists, watch, config bool
}

// ExportSelect is the export overlay: it fetches the full app-owned data bundle
// from the backend, lets the user pick which sections to include (Space toggles,
// Enter writes), and writes the filtered bundle to a timestamped JSON file in the
// data dir. Filtering is client-side — the backend returns everything (watch data
// included) and the overlay zeroes the unselected sections before writing, so the
// config profile (which only the client holds) rides along the same path.
type ExportSelect struct {
	identity
	ctx      context.Context
	backend  api.PortabilityBackend
	keys     keymap.KeyMap
	circular bool
	dir      string // data dir the bundle is written into

	// config is the marshaled portable client config profile, attached at write
	// time when the config section is selected. configErr, when non-nil, means the
	// profile couldn't be marshaled — the config section is then unavailable.
	config    json.RawMessage
	configErr error

	loaded    bool
	bundle    portability.Bundle
	sel       exportSel
	rowSel    int
	exporting bool
	err       error
}

// NewExportSelect creates the export overlay and kicks off the backend fetch of
// the full bundle. config is the marshaled portable config profile (nil/empty
// when unavailable); configErr surfaces a marshal failure in the config row.
func NewExportSelect(ctx context.Context, backend api.PortabilityBackend, keys keymap.KeyMap, dir string, config json.RawMessage, configErr error, circular bool) (ExportSelect, tea.Cmd) {
	es := ExportSelect{
		identity:  newIdentity(),
		ctx:       ctx,
		backend:   backend,
		keys:      keys,
		circular:  circular,
		dir:       dir,
		config:    config,
		configErr: configErr,
		// Sensible defaults: everything portable on, personal watch data off (it is
		// opt-in, matching the import preview). Config only when one is available.
		sel: exportSel{channels: true, blocklist: true, playlists: true, watch: false, config: len(config) > 0},
	}
	return es, es.fetchCmd()
}

// ── overlay.Overlay interface ─────────────────────────────────────────────────

func (es ExportSelect) InterceptsInput() bool { return false }
func (es ExportSelect) WidthReduction() int   { return 0 }
func (es ExportSelect) HasFocus() bool        { return true }

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (es ExportSelect) Init() tea.Cmd  { return nil }
func (es ExportSelect) View() tea.View { return tea.NewView("") } // rendering done via Render(behind,...)

func (es ExportSelect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case exportBundleLoadedMsg:
		if m.err != nil {
			// Stay unloaded so the error renders and Enter can't write a partial
			// bundle; the user backs out with Esc.
			es.err = m.err
			return es, nil
		}
		es.loaded = true
		es.bundle = m.bundle
		return es, nil

	case exportWrittenMsg:
		es.exporting = false
		if m.err != nil {
			es.err = m.err
			return es, func() tea.Msg { return tuipkg.StatusMsg{Text: "export: " + m.err.Error(), IsErr: true} }
		}
		return es, tea.Batch(
			func() tea.Msg { return tuipkg.StatusMsg{Text: "Exported to " + m.path} },
			func() tea.Msg { return PopOverlayMsg{} },
		)

	case tea.KeyPressMsg:
		return es.handleKey(m)
	}
	return es, nil
}

// ── key handling ──────────────────────────────────────────────────────────────

func (es ExportSelect) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if newSel, consumed := moveVertical(es.rowSel, esRowCount, msg, es.keys, es.circular, true); consumed {
		es.rowSel = newSel
		return es, nil
	}
	switch {
	case key.Matches(msg, es.keys.Escape), key.Matches(msg, es.keys.Quit):
		return es, func() tea.Msg { return PopOverlayMsg{} }
	case key.Matches(msg, exportToggleKey):
		es.toggleRow()
		return es, nil
	case key.Matches(msg, es.keys.DrillDown):
		// Enter confirms the export from any row (the Export row is just an explicit
		// affordance); Space is the selection key, so Enter never toggles.
		return es.exportRow()
	}
	return es, nil
}

// toggleRow flips the focused section checkbox. The Export row and an
// unavailable config section are no-ops.
func (es *ExportSelect) toggleRow() {
	switch es.rowSel {
	case esRowChannels:
		es.sel.channels = !es.sel.channels
	case esRowBlocklist:
		es.sel.blocklist = !es.sel.blocklist
	case esRowPlaylists:
		es.sel.playlists = !es.sel.playlists
	case esRowWatch:
		es.sel.watch = !es.sel.watch
	case esRowConfig:
		if es.hasConfig() {
			es.sel.config = !es.sel.config
		}
	}
}

// exportRow writes the filtered bundle, or reports if nothing is selected.
func (es ExportSelect) exportRow() (tea.Model, tea.Cmd) {
	if !es.loaded || es.exporting {
		return es, nil
	}
	if !es.anySelected() {
		return es, func() tea.Msg {
			return tuipkg.StatusMsg{Text: "export: nothing selected", IsErr: true}
		}
	}
	es.exporting = true
	return es, es.writeCmd()
}

func (es ExportSelect) anySelected() bool {
	s := es.sel
	return s.channels || s.blocklist || s.playlists || s.watch || (s.config && es.hasConfig())
}

// hasConfig reports whether a config profile is available to include.
func (es ExportSelect) hasConfig() bool { return es.configErr == nil && len(es.config) > 0 }

// ── commands ──────────────────────────────────────────────────────────────────

// fetchCmd pulls the full bundle from the backend, always requesting watch data
// so the watch-data toggle applies instantly without a re-fetch.
func (es ExportSelect) fetchCmd() tea.Cmd {
	backend := es.backend
	ctx := es.ctx
	target := tuipkg.OverlayTarget{ID: es.ID()}
	return func() tea.Msg {
		b, err := backend.Export(ctx, portability.ExportOptions{IncludeWatchData: true})
		return exportBundleLoadedMsg{OverlayTarget: target, bundle: b, err: err}
	}
}

// writeCmd builds the filtered bundle and writes it to a timestamped file.
func (es ExportSelect) writeCmd() tea.Cmd {
	dir := es.dir
	bundle := es.buildBundle()
	target := tuipkg.OverlayTarget{ID: es.ID()}
	return func() tea.Msg {
		data, err := json.MarshalIndent(bundle, "", "  ")
		if err != nil {
			return exportWrittenMsg{OverlayTarget: target, err: err}
		}
		stamp := time.Now().Format("20060102-150405")
		path := filepath.Join(dir, "yt-tui-export-"+stamp+".json")
		if werr := os.WriteFile(path, data, 0o600); werr != nil {
			return exportWrittenMsg{OverlayTarget: target, err: werr}
		}
		return exportWrittenMsg{OverlayTarget: target, path: path}
	}
}

// buildBundle returns a copy of the fetched bundle with unselected sections
// cleared and the config profile attached when selected. Videos ride with the
// playlist section: they are the dedup metadata that rehydrates playlist,
// watch-later, and YT-playlist references on import.
func (es ExportSelect) buildBundle() portability.Bundle {
	b := es.bundle
	if !es.sel.channels {
		b.Channels = nil
	}
	if !es.sel.blocklist {
		b.BlockedNames = nil
	}
	if !es.sel.playlists {
		b.Playlists = nil
		b.YTPlaylists = nil
		b.Videos = nil
	}
	if !es.sel.watch {
		b.History = nil
		b.Positions = nil
	}
	if es.sel.config && es.hasConfig() {
		b.Config = es.config
	} else {
		b.Config = nil
	}
	return b
}

// ── rendering ─────────────────────────────────────────────────────────────────

func (es ExportSelect) Render(behind string, width, _ int) string {
	return placeOverlayBox(behind, es.renderContent(), width, importBoxW)
}

func (es ExportSelect) renderContent() string {
	const innerW = importBoxW - 6
	lines := []string{styles.Bold.Render("Export data"), ""}

	if !es.loaded {
		if es.err != nil {
			lines = append(lines, styles.Error.Render(render.Truncate(es.err.Error(), innerW)))
		} else {
			lines = append(lines, styles.Help.Render("Loading…"))
		}
		lines = append(lines, "", importHint(innerW, "", es.keys.Escape.Help().Key+": cancel"))
		return strings.Join(lines, "\n")
	}

	lines = append(lines,
		importRow(es.rowSel == esRowChannels, importCheckbox(es.sel.channels, es.channelsLabel())),
		importRow(es.rowSel == esRowBlocklist, importCheckbox(es.sel.blocklist, es.blocklistLabel())),
		importRow(es.rowSel == esRowPlaylists, importCheckbox(es.sel.playlists, es.playlistsLabel())),
		importRow(es.rowSel == esRowWatch, importCheckbox(es.sel.watch, es.watchLabel())),
		importRow(es.rowSel == esRowConfig, importCheckbox(es.sel.config && es.hasConfig(), es.configLabel())),
		"",
		importRow(es.rowSel == esRowExport, "Export to file"),
		"",
		importHint(innerW, "j/k: move  space: toggle  enter: export", es.keys.Escape.Help().Key+": cancel"),
	)
	return strings.Join(lines, "\n")
}

func (es ExportSelect) channelsLabel() string {
	return fmt.Sprintf("Channels & annotations (%d)", len(es.bundle.Channels))
}

func (es ExportSelect) blocklistLabel() string {
	return fmt.Sprintf("Blocklist (%d names)", len(es.bundle.BlockedNames))
}

func (es ExportSelect) playlistsLabel() string {
	return fmt.Sprintf("Playlists (%d · %d YT)",
		len(es.bundle.Playlists), len(es.bundle.YTPlaylists))
}

func (es ExportSelect) watchLabel() string {
	if len(es.bundle.History) == 0 && len(es.bundle.Positions) == 0 {
		return "Watch data (none)"
	}
	return fmt.Sprintf("Watch data (%d history · %d positions)", len(es.bundle.History), len(es.bundle.Positions))
}

func (es ExportSelect) configLabel() string {
	if es.configErr != nil {
		return "Config profile (unavailable)"
	}
	if len(es.config) == 0 {
		return "Config profile (none)"
	}
	return "Config profile"
}
