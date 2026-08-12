package youtube

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

// waitErr must fold a non-zero yt-dlp exit into an error ONLY when the run
// produced no output (parsed == 0); a non-zero exit alongside real output is a
// partial success, and a pre-existing scan error always wins (H-2).
func TestWaitErr(t *testing.T) {
	scanBoom := errors.New("scan boom")
	tests := []struct {
		name    string
		bin     string // "true" exits 0, "false" exits non-zero
		parsed  int
		scanErr error
		wantErr bool
		wantIs  error // if set, result must errors.Is this
	}{
		{"success, no scan error", "true", 0, nil, false, nil},
		{"failed exit, no output -> error", "false", 0, nil, true, nil},
		{"failed exit, with output -> partial success", "false", 3, nil, false, nil},
		{"scan error beats clean exit", "true", 5, scanBoom, true, scanBoom},
		{"scan error beats failed exit", "false", 0, scanBoom, true, scanBoom},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.CommandContext(context.Background(), tc.bin)
			if err := cmd.Start(); err != nil {
				t.Fatalf("start %q: %v", tc.bin, err)
			}
			got := waitErr(cmd, tc.parsed, tc.scanErr)
			switch {
			case tc.wantIs != nil:
				if !errors.Is(got, tc.wantIs) {
					t.Fatalf("want errors.Is(%v), got %v", tc.wantIs, got)
				}
			case tc.wantErr:
				if got == nil {
					t.Fatal("want error, got nil")
				}
			default:
				if got != nil {
					t.Fatalf("want nil, got %v", got)
				}
			}
		})
	}
}
