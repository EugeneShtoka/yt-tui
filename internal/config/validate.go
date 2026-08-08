package config

import (
	"strconv"
	"strings"
)

// panelTypes is the set of Type values the TUI's panel factory can build.
// Kept here so post-load validation can drop panels the factory can't honor.
var panelTypes = map[string]bool{
	"feed": true, "channels": true, "tags": true, "playlists": true,
	"search": true, "downloading": true, "local": true, "history": true, "activity": true,
}

// IsPanelType reports whether t names a panel type the TUI can build. Exported
// so the TUI's panel factory filters against the same vocabulary this package
// validates panels with.
func IsPanelType(t string) bool { return panelTypes[t] }

// panelModeTypes lists the panel types that accept a source Mode; any other
// type must carry an empty Mode (validation clears a stray one).
var panelModeTypes = map[string]bool{"feed": true, "channels": true, "tags": true}

// validSortNames mirrors the sort-key vocabulary (see SortKeys); a panel's
// Sort must be one of these or empty. Kept local so the config package needs
// no dependency on the feed domain package (which owns the Sort* modes).
var validSortNames = map[string]bool{
	"views": true, "date": true, "name": true, "none": true, "channel": true,
	"duration": true, "subscribers": true, "tags": true, "size": true,
}

// Normalize fills in any missing keybindings and re-applies the derived
// defaults + panel validation. Call it after mutating config fields wholesale
// (e.g. applying an imported config profile) so the result is as complete and
// valid as a freshly loaded config before it is saved.
func (c *Config) Normalize() {
	c.Keybindings.fillDefaults()
	// nil log: a mid-session profile import has its own preview UX, so its
	// normalizations must not leak into the startup issue overlay.
	applyDerivedDefaults(c, nil)
}

// validFeedModes is the set of source modes a feed/tags panel (and the global
// feed_mode/tags_mode) accepts. Channels additionally allows "blocked"/"all".
var validFeedModes = map[string]bool{
	"recommended": true, "subscribed": true, "mixed": true, "stale": true,
}

// validChannelsViews is the set of modes the channels panel (and the global
// channels_view) accepts. "all" is a legacy alias for "mixed".
var validChannelsViews = map[string]bool{
	"recommended": true, "subscribed": true, "mixed": true, "blocked": true, "stale": true, "all": true,
}

// applyDerivedDefaults backfills scalar fields left empty/zero by an older or
// partial config file. Recoverable problems in user-supplied enum values are
// reset to their default and recorded on log (nil skips recording).
func applyDerivedDefaults(cfg *Config, log *issueLog) {
	if cfg.HintMode == "" {
		cfg.HintMode = "full"
	}
	if cfg.DurationFormat == "" {
		cfg.DurationFormat = "hh:mm:ss"
	}
	if cfg.DateFormat == "" {
		cfg.DateFormat = "dd/mm/yyyy"
	}
	if cfg.ChannelLatestCount <= 0 {
		cfg.ChannelLatestCount = 3
	}
	if cfg.ChannelRefreshMinutes <= 0 {
		cfg.ChannelRefreshMinutes = 60
	}
	// 0 is a valid "disabled"; only an out-of-range value falls back to the default.
	if cfg.WatchLaterAutoRemovePercent < 0 || cfg.WatchLaterAutoRemovePercent > 100 {
		cfg.WatchLaterAutoRemovePercent = 90
	}
	if cfg.ChannelStrikes <= 0 {
		cfg.ChannelStrikes = 2
	}
	cfg.FeedMode = normalizeEnum("feed_mode", cfg.FeedMode, "recommended", validFeedModes, log)
	cfg.ChannelsView = normalizeEnum("channels_view", cfg.ChannelsView, "subscribed", validChannelsViews, log)
	cfg.TagsMode = normalizeEnum("tags_mode", cfg.TagsMode, "subscribed", validFeedModes, log)
	if cfg.StaleTaggedChannelDays <= 0 {
		cfg.StaleTaggedChannelDays = 30
	}
	if len(cfg.SubtitleLangs) == 0 {
		cfg.SubtitleLangs = []string{"en"}
	}
	if cfg.TranscriptWidth != "" && !ValidWidthSpec(cfg.TranscriptWidth) {
		log.warnf("transcript_width %q is not a valid width spec; using %q", cfg.TranscriptWidth, "50%")
		cfg.TranscriptWidth = "50%"
	} else if cfg.TranscriptWidth == "" {
		cfg.TranscriptWidth = "50%"
	}
	validatePanels(cfg, log)
}

// normalizeEnum returns val unchanged when it is a recognized value, backfills
// an empty val to def silently (an absent key is not a user error), and for any
// other value records a warning on log and falls back to def.
func normalizeEnum(name, val, def string, valid map[string]bool, log *issueLog) string {
	switch {
	case val == "":
		return def
	case valid[val]:
		return val
	default:
		log.warnf("%s %q is not a valid value; using %q", name, val, def)
		return def
	}
}

// ValidWidthSpec reports whether s is a usable width specification: either a
// positive absolute column count ("80") or a percentage in (0,100] ("50%").
func ValidWidthSpec(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if pct, ok := strings.CutSuffix(s, "%"); ok {
		n, err := strconv.Atoi(strings.TrimSpace(pct))
		return err == nil && n > 0 && n <= 100
	}
	n, err := strconv.Atoi(s)
	return err == nil && n > 0
}

// validatePanels sanitizes and completes the data-driven tab layout after load.
// It drops panels whose Type the factory can't build (falling back to
// DefaultPanels when nothing valid remains), normalizes each survivor via
// normalizePanel, prunes TabKeys hotkeys that point at no surviving panel, and
// records every recoverable problem on log (Phase 19 issue reporting).
func validatePanels(cfg *Config, log *issueLog) {
	kept := make([]Panel, 0, len(cfg.Panels))
	for _, p := range cfg.Panels {
		if np, ok := normalizePanel(p, cfg, log); ok {
			kept = append(kept, np)
		}
	}
	if len(kept) == 0 {
		if len(cfg.Panels) > 0 {
			log.warnf("no configured panel had a usable type; falling back to the default layout")
		}
		for _, p := range DefaultPanels {
			np, _ := normalizePanel(p, cfg, nil) // defaults are always valid
			kept = append(kept, np)
		}
	}
	cfg.Panels = kept
	pruneTabKeys(cfg, log)
	warnUnreachablePanels(cfg, log)
}

// normalizePanel validates one panel and fills in its effective Mode/Sort so
// the written config always surfaces them (rather than leaving blanks that
// silently inherit a default). ok is false for a type the factory can't build.
// A Mode/Sort the type doesn't accept is cleared, then an empty Mode/Sort is
// backfilled from the type's default — the panel becomes the source of truth,
// seeded once from the global FeedMode/ChannelsView/TagsMode.
func normalizePanel(p Panel, cfg *Config, log *issueLog) (Panel, bool) {
	if !panelTypes[p.Type] {
		log.warnf("panel %q has unknown type %q; dropping it", panelLabel(p), p.Type)
		return Panel{}, false
	}
	if p.Name == "" {
		p.Name = p.Type // keep the panel reachable by a sensible name
	}
	if p.Mode != "" && (!panelModeTypes[p.Type] || !validPanelMode(p.Type, p.Mode)) {
		log.warnf("panel %q: mode %q is not valid for type %q; using its default", p.Name, p.Mode, p.Type)
		p.Mode = ""
	}
	if p.Mode == "" {
		p.Mode = defaultModeForType(p.Type, cfg)
	}
	if p.Sort != "" && !validSortNames[p.Sort] {
		log.warnf("panel %q: sort %q is not a valid sort name; using its default", p.Name, p.Sort)
		p.Sort = ""
	}
	if p.Sort == "" {
		p.Sort = defaultSortForType(p.Type)
	}
	return p, true
}

// panelLabel names a panel for a diagnostic message, falling back to its type
// when the panel has no name (an unnamed, unknown-type panel).
func panelLabel(p Panel) string {
	if p.Name != "" {
		return p.Name
	}
	return p.Type
}

// defaultModeForType returns the effective default source mode for a panel type
// (empty for types that take no mode), seeded from the global mode config so an
// existing FeedMode/ChannelsView/TagsMode is preserved into the panel.
func defaultModeForType(typ string, cfg *Config) string {
	switch typ {
	case "feed":
		return cfg.FeedMode
	case "channels":
		return cfg.ChannelsView
	case "tags":
		return cfg.TagsMode
	}
	return ""
}

// defaultSortForType returns the built-in default sort name for a panel type
// (empty for types without a sort chord), matching each list tab's historical
// default so surfacing it in config changes no behavior.
func defaultSortForType(typ string) string {
	switch typ {
	case "feed", "channels":
		return "date"
	case "local", "history", "playlists":
		return "views"
	}
	return ""
}

// validPanelMode reports whether mode is a legal source mode for a panel of the
// given type. Callers guarantee mode != "" and panelModeTypes[typ]. It shares
// the same valid-value sets as the global feed_mode/channels_view/tags_mode
// enums so the two surfaces never drift.
func validPanelMode(typ, mode string) bool {
	switch typ {
	case "feed", "tags":
		return validFeedModes[mode]
	case "channels":
		return validChannelsViews[mode]
	}
	return false
}

// pruneTabKeys removes tab-chord hotkeys that reference a panel name not present
// in the (already validated) panel list, so a rename/removal can't leave a
// dangling binding, and records each pruned hotkey on log.
func pruneTabKeys(cfg *Config, log *issueLog) {
	names := make(map[string]bool, len(cfg.Panels))
	for _, p := range cfg.Panels {
		names[p.Name] = true
	}
	for hotkey, name := range cfg.Keybindings.TabKeys {
		if !names[name] {
			log.warnf("tab hotkey %q points at unknown panel %q; ignoring it", hotkey, name)
			delete(cfg.Keybindings.TabKeys, hotkey)
		}
	}
}

// warnUnreachablePanels records a warning for each panel that no tab hotkey
// activates: the positional TabChord+1..9 fallback covers only the first nine
// panels, so a 10th+ panel without a named hotkey is reachable only by
// Tab/Shift-Tab cycling. Purely informational — nothing is changed.
func warnUnreachablePanels(cfg *Config, log *issueLog) {
	if log == nil {
		return
	}
	keyed := make(map[string]bool, len(cfg.Keybindings.TabKeys))
	for _, name := range cfg.Keybindings.TabKeys {
		keyed[name] = true
	}
	for i, p := range cfg.Panels {
		if i < 9 || keyed[p.Name] {
			continue // reachable by a positional digit or a named hotkey
		}
		log.warnf("panel %q is past the 9th position and has no tab hotkey; it is reachable only by Tab/Shift-Tab cycling", p.Name)
	}
}
