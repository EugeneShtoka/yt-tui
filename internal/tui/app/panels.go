package app

import (
	"context"
	"strconv"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/tab"
)

// effectivePanels drops panels whose type the factory can't build so the tab
// list, the chord map, and the name index all index the same slice. Falls back
// to the built-in DefaultPanels when nothing valid remains. Config validation
// normally guarantees this already; the filter keeps New robust when a Config
// is hand-built in tests without going through Load.
func effectivePanels(panels []config.Panel) []config.Panel {
	kept := make([]config.Panel, 0, len(panels))
	for _, p := range panels {
		if config.IsPanelType(p.Type) {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return config.DefaultPanels
	}
	return kept
}

// buildPanels constructs the ordered tab set, one tab per panel via buildPanel.
// Callers pass an already-filtered list (see effectivePanels), so every panel
// builds and the result is 1:1 with panels.
func buildPanels(ctx context.Context, panels []config.Panel, backend api.Backend, keys keymap.KeyMap, cfg *config.Config) []tuipkg.Tab {
	tabs := make([]tuipkg.Tab, 0, len(panels))
	for _, p := range panels {
		if t, ok := buildPanel(ctx, p, backend, keys, cfg); ok {
			tabs = append(tabs, t)
		}
	}
	return tabs
}

// buildPanel constructs a single tab for a panel definition, threading the
// panel's Mode (empty inherits the tab's global default — FeedMode/
// ChannelsView/TagsMode) and Sort (empty keeps the tab's built-in default).
// Only feed/channels/tags accept a mode; only the list tabs accept a sort.
// ok is false for an unknown type.
func buildPanel(ctx context.Context, p config.Panel, backend api.Backend, keys keymap.KeyMap, cfg *config.Config) (tuipkg.Tab, bool) {
	// cols is this panel's configured column selection (nil = show all, its
	// natural order). Keyed by panel name so it survives reordering; spread into
	// each constructor's trailing wantCols variadic. Invalid keys are validated
	// and warned about at startup (see ValidateColumns), then ignored.
	cols := cfg.Columns[p.Name]
	switch p.Type {
	case "feed":
		return tab.NewFeed(ctx, backend, keys, cfg.CircularNav, tab.FeedOpts{
			Mode: modeOr(p.Mode, cfg.FeedMode), Sort: p.Sort, StaleDays: cfg.StaleTaggedChannelDays,
			RecMaxAgeDays: cfg.RecommendedMaxAgeDays, RecMinDurationSecs: cfg.RecommendedMinDurationSecs,
			RecMinViews: cfg.RecommendedMinViews,
		}, cols...), true
	case "channels":
		return tab.NewChannels(ctx, backend, keys, cfg.CircularNav, tab.ChannelsOpts{
			LatestCount: cfg.ChannelLatestCount, RefreshMinutes: cfg.RefreshMinutes,
			View: modeOr(p.Mode, cfg.ChannelsView), Sort: p.Sort,
			HideStale: cfg.HideStaleTaggedChannels, StaleDays: cfg.StaleTaggedChannelDays,
		}, cols...), true
	case "tags":
		return tab.NewTags(ctx, backend, keys, cfg.CircularNav, tab.TagsOpts{
			Mode: modeOr(p.Mode, cfg.TagsMode), HideStale: cfg.HideStaleTaggedChannels, StaleDays: cfg.StaleTaggedChannelDays,
		}, cols...), true
	case "playlists":
		return tab.NewPlaylists(ctx, backend, keys, cfg.CircularNav, p.Sort, cols...), true
	case "search":
		return tab.NewSearch(ctx, backend, keys, cfg.CircularNav, cols...), true
	case "downloading":
		return tab.NewDownloading(ctx, backend, keys, cfg.CircularNav, cols...), true
	case "local":
		return tab.NewLocal(ctx, backend, keys, cfg.CircularNav, p.Sort, cols...), true
	case "history":
		return tab.NewHistory(ctx, backend, keys, cfg.CircularNav, p.Sort, cols...), true
	case "activity":
		return tab.NewActivity(ctx, backend, keys, cfg.CircularNav, cols...), true
	}
	return nil, false
}

// modeOr returns the panel-specific mode when set, else the tab's global default.
func modeOr(panelMode, global string) string {
	if panelMode != "" {
		return panelMode
	}
	return global
}

// panelIndexByName maps each panel's name to its position in the tab bar, for
// name-based navigation (the `:tab` command). First occurrence wins so a
// duplicate name can't shadow the earlier panel.
func panelIndexByName(panels []config.Panel) map[string]int {
	m := make(map[string]int, len(panels))
	for i, p := range panels {
		if _, ok := m[p.Name]; !ok {
			m[p.Name] = i
		}
	}
	return m
}

// buildTabChordKeys resolves the tab-chord second-key map (hotkey → panel
// index). It first installs the positional fallback (TabChord+1..9 → Nth
// panel) so keyless panels stay reachable, then overlays the configured named
// hotkeys (hotkey → panel name → index); a named binding wins over a positional
// digit on collision.
func buildTabChordKeys(tabKeys map[string]string, panels []config.Panel) map[string]int {
	idxByName := panelIndexByName(panels)
	m := make(map[string]int, len(panels)+len(tabKeys))
	for i := range panels {
		if i >= 9 {
			break
		}
		m[strconv.Itoa(i+1)] = i
	}
	for hotkey, name := range tabKeys {
		if idx, ok := idxByName[name]; ok {
			m[hotkey] = idx
		}
	}
	return m
}

// panelNames returns the panel names in order, for command completion.
func panelNames(panels []config.Panel) []string {
	names := make([]string, len(panels))
	for i, p := range panels {
		names[i] = p.Name
	}
	return names
}
