package config

import "fmt"

// Severity ranks a configuration issue for display. Every issue Phase 19
// records is non-fatal — the loader falls back to a safe default and keeps
// starting — so severity only drives the startup overlay's color, not whether
// the app runs. Hard parse/unmarshal errors stay fatal and are returned from
// Load as an error rather than collected here.
type Severity int

const (
	// SeverityWarning marks a recoverable normalization: an invalid enum reset
	// to its default, a dangling hotkey pruned, a panel unreachable by any key.
	// The app behaves sensibly; the user is simply told what was ignored.
	SeverityWarning Severity = iota
	// SeverityError marks a configured capability that will not work as asked —
	// a theme that failed to load, no video player on PATH, YouTube allowed but
	// unusable. The app still starts, with that feature degraded.
	SeverityError
)

// String returns the lowercase label used in the issue overlay and tests.
func (s Severity) String() string {
	if s == SeverityError {
		return "error"
	}
	return "warning"
}

// ConfigIssue is one non-fatal problem found while loading configuration or
// probing the environment. Issues are collected during Load (panel/enum
// normalization) and, in the client's composition root, from runtime init
// (theme load, TLS CA, player resolution, YouTube availability) and surfaced
// together in the startup issue overlay instead of being swallowed to stderr.
type ConfigIssue struct {
	Severity Severity
	Message  string
}

// issueLog accumulates ConfigIssues during validation. A nil *issueLog is a
// valid no-op sink, so paths that don't surface diagnostics (the pure-defaults
// first run, and the profile-import Normalize path) simply pass nil.
type issueLog struct{ items []ConfigIssue }

// warnf records a SeverityWarning issue. It is the only severity the config
// loader emits; error-severity issues (theme/player/TLS/YouTube) are built by
// the client composition root, which owns those runtime concerns.
func (l *issueLog) warnf(format string, a ...any) {
	if l == nil {
		return
	}
	l.items = append(l.items, ConfigIssue{Severity: SeverityWarning, Message: fmt.Sprintf(format, a...)})
}
