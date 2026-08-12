package sys

import "testing"

func TestEditorCommandResolution(t *testing.T) {
	tests := []struct {
		name       string
		visual     string
		editor     string
		wantEditor string
	}{
		{"visual wins", "code", "nano", "code"},
		{"editor when no visual", "", "nano", "nano"},
		{"vi fallback", "", "", "vi"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("VISUAL", tc.visual)
			t.Setenv("EDITOR", tc.editor)
			cmd := EditorCommand("/tmp/config.toml")
			if cmd.Args[0] != tc.wantEditor {
				t.Errorf("editor = %q, want %q", cmd.Args[0], tc.wantEditor)
			}
			if got := cmd.Args[len(cmd.Args)-1]; got != "/tmp/config.toml" {
				t.Errorf("path arg = %q, want /tmp/config.toml", got)
			}
		})
	}
}
