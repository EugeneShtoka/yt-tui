package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestJustifyEnds(t *testing.T) {
	cases := []struct {
		name       string
		left       string
		right      string
		width      int
		wantWidth  int    // total display width of the result
		wantPrefix string // result must start with left
		wantSuffix string // result must end with right
	}{
		{"basic", "ab", "cd", 10, 10, "ab", "cd"},
		{"empty left", "", "close", 10, 10, "", "close"},
		{"empty right", "scroll", "", 10, 10, "scroll", ""},
		{"exact fit keeps one space", "abc", "def", 6, 7, "abc", "def"},
		{"overflow floors to one space", "abcdef", "ghijkl", 4, 13, "abcdef", "ghijkl"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := JustifyEnds(tc.left, tc.right, tc.width)
			if w := ansi.StringWidth(got); w != tc.wantWidth {
				t.Errorf("width = %d, want %d (%q)", w, tc.wantWidth, got)
			}
			if !strings.HasPrefix(got, tc.wantPrefix) {
				t.Errorf("%q does not start with %q", got, tc.wantPrefix)
			}
			if !strings.HasSuffix(got, tc.wantSuffix) {
				t.Errorf("%q does not end with %q", got, tc.wantSuffix)
			}
		})
	}
}

// JustifyEnds must measure display width, not byte length: a styled or wide-rune
// left/right must not push the right end past the width (the import_preview len()
// bug this helper replaces).
func TestJustifyEndsMeasuresDisplayWidth(t *testing.T) {
	styled := "\x1b[31mred\x1b[0m" // 3 visible cells, many bytes
	got := JustifyEnds(styled, "x", 10)
	if w := ansi.StringWidth(got); w != 10 {
		t.Fatalf("styled width = %d, want 10 (%q)", w, got)
	}
	// A double-width rune counts as 2 cells.
	got = JustifyEnds("世界", "z", 10) // 4 cells + z
	if w := ansi.StringWidth(got); w != 10 {
		t.Fatalf("CJK width = %d, want 10 (%q)", w, got)
	}
}

func TestModalBox(t *testing.T) {
	cases := []struct {
		name        string
		width       int
		ratioTenths int
		max         int
		wantBox     int
	}{
		{"ratio applies", 100, 6, 72, 60},     // 100*6/10 = 60, under max
		{"clamped to max", 200, 6, 72, 72},    // 120 -> 72
		{"clamped to width-4", 40, 9, 72, 36}, // 36 -> min(36, 36)
		{"floors at 32", 30, 6, 72, 32},       // 18 -> 32
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			boxW, innerW := ModalBox(tc.width, tc.ratioTenths, tc.max)
			if boxW != tc.wantBox {
				t.Errorf("boxW = %d, want %d", boxW, tc.wantBox)
			}
			if innerW != boxW-BorderPad {
				t.Errorf("innerW = %d, want boxW-%d = %d", innerW, BorderPad, boxW-BorderPad)
			}
		})
	}
}
