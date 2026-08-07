package main

import "testing"

// TestRunPositionTrackerArgValidation covers the tracker subcommand's argument
// handling — the paths that reject bad input before touching the config or DB.
func TestRunPositionTrackerArgValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"missing pid", []string{"vid1"}},
		{"too many args", []string{"vid1", "123", "extra"}},
		{"non-numeric pid", []string{"vid1", "notapid"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := runPositionTracker(tt.args); err == nil {
				t.Errorf("runPositionTracker(%q): got nil error, want a validation error", tt.args)
			}
		})
	}
}
