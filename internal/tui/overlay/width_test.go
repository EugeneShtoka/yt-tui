package overlay

import (
	"testing"
)

func TestParseWidthSpec(t *testing.T) {
	cases := []struct {
		spec string
		term int
		want int
	}{
		{"50%", 200, 100},
		{"25%", 200, 50},
		{"80", 200, 80},
		{"", 200, 100},    // empty → half
		{"999", 200, 196}, // clamped to term-4
		{"bogus", 200, 100},
		{"150%", 200, 100}, // >100% falls back to 50%
		{"10", 200, 24},    // below the readability floor
	}
	for _, c := range cases {
		if got := parseWidthSpec(c.spec, c.term); got != c.want {
			t.Errorf("parseWidthSpec(%q, %d) = %d, want %d", c.spec, c.term, got, c.want)
		}
	}
}

// TestTranscriptWidthDerivation locks the box↔text width relationship the modal
// renderer relies on (text wraps inside the box content area, not past it).
func TestTranscriptWidthDerivation(t *testing.T) {
	const term = 200
	box := transcriptBoxWidth("50%", term)
	text := transcriptTextWidth("50%", term)
	if text != box-6 {
		t.Errorf("text width %d, want box(%d)-6", text, box)
	}
	// A wider spec must produce a wider text column.
	if transcriptTextWidth("80%", term) <= transcriptTextWidth("40%", term) {
		t.Error("wider spec did not widen the text column")
	}
}
