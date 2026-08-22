package youtube

import (
	"testing"
	"time"
)

// TestParseVersion covers what yt-dlp itself prints and what package managers
// wrap around it: a distro epoch, a packaging release suffix, and nightly builds.
func TestParseVersion(t *testing.T) {
	tests := []struct {
		in     string
		wantOK bool
		want   string // yyyy-mm-dd of the parsed release date
	}{
		{"2026.08.19", true, "2026-08-19"},
		{"2026.08.19\n", true, "2026-08-19"},
		{"2026.08.19.123456", true, "2026-08-19"}, // nightly
		{"2026.8.9", true, "2026-08-09"},          // dnf prints unpadded
		{"2026.08.19-1", true, "2026-08-19"},      // pacman/deb packaging release
		{"1:2026.08.19-1.1", true, "2026-08-19"},  // epoch + release
		{"2026.08.19+really", true, "2026-08-19"},
		{"2026.08", false, ""},
		{"", false, ""},
		{"1999.06.09", false, ""}, // implausibly old year
		{"2026.13.09", false, ""}, // invalid month
		{"2026.06.40", false, ""}, // invalid day
		{"stable", false, ""},
		{"nightly", false, ""},
	}
	for _, tt := range tests {
		got, ok := ParseVersion(tt.in)
		if ok != tt.wantOK {
			t.Errorf("ParseVersion(%q): ok=%v, want %v", tt.in, ok, tt.wantOK)
			continue
		}
		if ok && got.Date.Format("2006-01-02") != tt.want {
			t.Errorf("ParseVersion(%q): date=%s, want %s", tt.in, got.Date.Format("2006-01-02"), tt.want)
		}
	}
}

// TestParseVersionKeepsRaw: the raw string is what gets shown to the user, so it
// survives parsing intact apart from surrounding whitespace.
func TestParseVersionKeepsRaw(t *testing.T) {
	got, ok := ParseVersion("  2026.08.19-1\n")
	if !ok {
		t.Fatal("ParseVersion failed on a valid packaged version")
	}
	if got.Raw != "2026.08.19-1" {
		t.Errorf("Raw = %q, want %q", got.Raw, "2026.08.19-1")
	}
}

func TestVersionComparisons(t *testing.T) {
	older, _ := ParseVersion("2026.05.01")
	newer, _ := ParseVersion("2026.08.19")

	if !older.Older(newer) {
		t.Error("2026.05.01 must be older than 2026.08.19")
	}
	if newer.Older(older) {
		t.Error("2026.08.19 must not be older than 2026.05.01")
	}
	if got, want := older.Behind(newer), 110*24*time.Hour; got != want {
		t.Errorf("Behind = %v, want %v", got, want)
	}
	if got := newer.Behind(older); got != 0 {
		t.Errorf("a newer version is 0 behind, got %v", got)
	}
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	if got, want := newer.Age(now), 24*time.Hour; got != want {
		t.Errorf("Age = %v, want %v", got, want)
	}
}
