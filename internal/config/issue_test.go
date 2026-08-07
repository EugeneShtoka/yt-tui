package config

import (
	"strings"
	"testing"
)

// hasIssue reports whether any recorded issue has the given severity and a
// message containing every one of substrs (case-sensitive).
func hasIssue(issues []ConfigIssue, sev Severity, substrs ...string) bool {
	for _, iss := range issues {
		if iss.Severity != sev {
			continue
		}
		ok := true
		for _, s := range substrs {
			if !strings.Contains(iss.Message, s) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// TestSeverityString pins the display strings the overlay and tests rely on.
func TestSeverityString(t *testing.T) {
	if got := SeverityWarning.String(); got != "warning" {
		t.Errorf("SeverityWarning.String() = %q, want warning", got)
	}
	if got := SeverityError.String(); got != "error" {
		t.Errorf("SeverityError.String() = %q, want error", got)
	}
}

// TestCleanConfigRecordsNoIssues proves a valid config produces an empty issue
// list — the overlay only appears when something is actually wrong.
func TestCleanConfigRecordsNoIssues(t *testing.T) {
	// A config that overrides nothing structural keeps the aligned default
	// panels + tab keys, so no panel is dropped and no hotkey dangles.
	cfg := loadFromTOML(t, "player = \"mpv\"\nfeed_mode = \"mixed\"\n")
	if len(cfg.Issues) != 0 {
		t.Errorf("clean config recorded issues: %+v", cfg.Issues)
	}
}

// TestUnknownPanelTypeRecordsIssue: a dropped panel must be reported, not
// silently discarded (the whole point of Phase 19).
func TestUnknownPanelTypeRecordsIssue(t *testing.T) {
	cfg := loadFromTOML(t, `
[[panels]]
name = "feed"
type = "feed"

[[panels]]
name = "bogus"
type = "does-not-exist"
`)
	if !hasIssue(cfg.Issues, SeverityWarning, "bogus", "does-not-exist") {
		t.Errorf("dropping unknown panel type recorded no issue: %+v", cfg.Issues)
	}
}

// TestInvalidPanelModeRecordsIssue: a mode the type rejects is reset to the
// default and reported.
func TestInvalidPanelModeRecordsIssue(t *testing.T) {
	cfg := loadFromTOML(t, `
[[panels]]
name = "feed"
type = "feed"
mode = "blocked"
`)
	if !hasIssue(cfg.Issues, SeverityWarning, "feed", "blocked") {
		t.Errorf("invalid panel mode recorded no issue: %+v", cfg.Issues)
	}
}

// TestInvalidPanelSortRecordsIssue: an unknown sort name is reset and reported.
func TestInvalidPanelSortRecordsIssue(t *testing.T) {
	cfg := loadFromTOML(t, `
[[panels]]
name = "feed"
type = "feed"
sort = "nonsense"
`)
	if !hasIssue(cfg.Issues, SeverityWarning, "feed", "nonsense") {
		t.Errorf("invalid panel sort recorded no issue: %+v", cfg.Issues)
	}
}

// TestDanglingTabKeyRecordsIssue: a hotkey naming a non-existent panel is
// pruned and reported.
func TestDanglingTabKeyRecordsIssue(t *testing.T) {
	cfg := loadFromTOML(t, `
[[panels]]
name = "myfeed"
type = "feed"

[keybindings.tab_keys]
g = "myfeed"
z = "ghost"
`)
	if !hasIssue(cfg.Issues, SeverityWarning, "z", "ghost") {
		t.Errorf("dangling tab key recorded no issue: %+v", cfg.Issues)
	}
}

// TestInvalidGlobalModeRecordsIssue: an invalid top-level feed_mode enum is
// reset to the default and reported.
func TestInvalidGlobalModeRecordsIssue(t *testing.T) {
	cfg := loadFromTOML(t, "feed_mode = \"bananas\"\n")
	if cfg.FeedMode != "recommended" {
		t.Errorf("invalid feed_mode not reset: %q", cfg.FeedMode)
	}
	if !hasIssue(cfg.Issues, SeverityWarning, "feed_mode", "bananas") {
		t.Errorf("invalid feed_mode recorded no issue: %+v", cfg.Issues)
	}
}

// TestUnreachablePanelRecordsIssue: a 10th+ panel with no tab hotkey is only
// reachable by cycling, which Phase 19 surfaces as a warning.
func TestUnreachablePanelRecordsIssue(t *testing.T) {
	cfg := loadFromTOML(t, `
[[panels]]
name = "p1"
type = "feed"
[[panels]]
name = "p2"
type = "channels"
[[panels]]
name = "p3"
type = "tags"
[[panels]]
name = "p4"
type = "playlists"
[[panels]]
name = "p5"
type = "search"
[[panels]]
name = "p6"
type = "downloading"
[[panels]]
name = "p7"
type = "local"
[[panels]]
name = "p8"
type = "history"
[[panels]]
name = "p9"
type = "activity"
[[panels]]
name = "p10"
type = "feed"
`)
	if !hasIssue(cfg.Issues, SeverityWarning, "p10") {
		t.Errorf("unreachable 10th panel recorded no issue: %+v", cfg.Issues)
	}
}

// TestNormalizeDoesNotSurfaceIssues: applying an imported profile via Normalize
// must not populate the startup Issues list (import has its own preview UX).
func TestNormalizeDoesNotSurfaceIssues(t *testing.T) {
	cfg := defaultConfig()
	cfg.FeedMode = "bananas" // invalid, would warn under Load
	cfg.Issues = nil
	cfg.Normalize()
	if len(cfg.Issues) != 0 {
		t.Errorf("Normalize populated startup Issues: %+v", cfg.Issues)
	}
	if cfg.FeedMode != "recommended" {
		t.Errorf("Normalize did not still fix the invalid value: %q", cfg.FeedMode)
	}
}
