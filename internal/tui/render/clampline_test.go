package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestClampLineStripsCarriageReturn(t *testing.T) {
	// A title with an embedded CR: width-0 to every calculator, but the terminal
	// snaps the cursor to column 0 on \r and corrupts the row.
	in := "Тбилиси\rпоехал!"
	out := ClampLine(in, 40)
	if strings.ContainsAny(out, "\r\n\t") {
		t.Fatalf("control char survived: %q", out)
	}
	if w := ansi.StringWidth(out); w != 40 {
		t.Fatalf("width = %d, want 40", w)
	}
}

func TestClampLinePreservesANSI(t *testing.T) {
	styled := "\x1b[31mred\x1b[0m"
	out := ClampLine(styled, 10)
	if !strings.Contains(out, "\x1b[31m") {
		t.Fatalf("ANSI SGR was stripped: %q", out)
	}
	if w := ansi.StringWidth(out); w != 10 {
		t.Fatalf("width = %d, want 10", w)
	}
}
