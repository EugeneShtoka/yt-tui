package app

import (
	"strings"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/config"
)

func hasIssueContaining(issues []config.ConfigIssue, substr string) bool {
	for _, is := range issues {
		if strings.Contains(is.Message, substr) {
			return true
		}
	}
	return false
}

var validatePanels = []config.Panel{
	{Name: "feed", Type: "feed"},
	{Name: "mylocal", Type: "local"},
}

// A valid selection produces no issues.
func TestValidateColumnsAcceptsValidKeys(t *testing.T) {
	cols := map[string][]string{
		"feed":    {"num", "title", "date"},
		"mylocal": {"title", "size"},
	}
	if got := ValidateColumns(validatePanels, cols); len(got) != 0 {
		t.Errorf("valid columns produced issues: %v", got)
	}
}

// A nil/empty map is clean.
func TestValidateColumnsEmptyIsClean(t *testing.T) {
	if got := ValidateColumns(validatePanels, nil); got != nil {
		t.Errorf("nil columns should be clean, got %v", got)
	}
}

// A key not offered by the panel's type warns (e.g. size on the feed video list).
func TestValidateColumnsWarnsUnavailableKey(t *testing.T) {
	cols := map[string][]string{"feed": {"title", "size"}}
	got := ValidateColumns(validatePanels, cols)
	if !hasIssueContaining(got, "size") {
		t.Errorf("expected a warning about the unavailable 'size' key, got %v", got)
	}
}

// A wholly unknown key warns too.
func TestValidateColumnsWarnsUnknownKey(t *testing.T) {
	cols := map[string][]string{"feed": {"ghost"}}
	if got := ValidateColumns(validatePanels, cols); !hasIssueContaining(got, "ghost") {
		t.Errorf("expected a warning about the unknown 'ghost' key, got %v", got)
	}
}

// A columns entry for a panel name that doesn't exist warns.
func TestValidateColumnsWarnsUnknownPanel(t *testing.T) {
	cols := map[string][]string{"nosuchpanel": {"title"}}
	if got := ValidateColumns(validatePanels, cols); !hasIssueContaining(got, "nosuchpanel") {
		t.Errorf("expected a warning about the unknown panel, got %v", got)
	}
}

// Every issue is a warning (non-fatal — the loader falls back to show-all).
func TestValidateColumnsSeverityIsWarning(t *testing.T) {
	cols := map[string][]string{"feed": {"ghost"}, "nope": {"title"}}
	for _, is := range ValidateColumns(validatePanels, cols) {
		if is.Severity != config.SeverityWarning {
			t.Errorf("issue %q severity = %v, want warning", is.Message, is.Severity)
		}
	}
}
