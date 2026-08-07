package app

import (
	"fmt"
	"sort"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/tui/tab"
)

// ValidateColumns checks a per-panel column selection (config.Columns) against
// the panels that will actually be built and the columns each panel type offers
// (tab.PanelColumnKeys). It returns one warning per problem — a selection for an
// unknown panel name, or a key a panel's type does not offer — folding column
// validation into the startup issue overlay (Phase 19 reporting) rather than
// erroring. Every problem is recoverable: SelectColumns simply ignores an
// unknown key and falls back to "show all" when a selection resolves to nothing,
// so the app always starts. Column knowledge lives in the TUI layer, so this
// validation belongs in the composition root, not in the config loader.
func ValidateColumns(panels []config.Panel, columns map[string][]string) []config.ConfigIssue {
	if len(columns) == 0 {
		return nil
	}
	typeByName := make(map[string]string, len(panels))
	for _, p := range panels {
		if _, ok := typeByName[p.Name]; !ok {
			typeByName[p.Name] = p.Type
		}
	}
	// Deterministic order so the overlay (and tests) see stable output.
	names := make([]string, 0, len(columns))
	for name := range columns {
		names = append(names, name)
	}
	sort.Strings(names)

	var issues []config.ConfigIssue
	for _, name := range names {
		typ, ok := typeByName[name]
		if !ok {
			issues = append(issues, config.ConfigIssue{
				Severity: config.SeverityWarning,
				Message:  fmt.Sprintf("columns configured for unknown panel %q; ignoring them", name),
			})
			continue
		}
		offered := make(map[string]bool)
		for _, k := range tab.PanelColumnKeys(typ) {
			offered[k] = true
		}
		for _, k := range columns[name] {
			if !offered[k] {
				issues = append(issues, config.ConfigIssue{
					Severity: config.SeverityWarning,
					Message:  fmt.Sprintf("panel %q: column %q is not offered by a %q panel; ignoring it", name, k, typ),
				})
			}
		}
	}
	return issues
}
