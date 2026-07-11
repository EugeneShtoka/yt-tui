package media

import (
	"strings"
	"testing"
)

func TestExtractLinksNone(t *testing.T) {
	result := ExtractLinks("no links here")
	if len(result) != 0 {
		t.Errorf("no links: got len=%d, want 0", len(result))
	}
}

func TestExtractLinksBasic(t *testing.T) {
	desc := "check out https://example.com"
	result := ExtractLinks(desc)
	if len(result) != 1 {
		t.Errorf("basic link: got len=%d, want 1", len(result))
	}
	if len(result) > 0 && !strings.Contains(result[0].URL, "example.com") {
		t.Errorf("basic link: URL=%s, want example.com", result[0].URL)
	}
}

func TestExtractLinksDedupe(t *testing.T) {
	desc := "https://example.com and https://example.com again"
	result := ExtractLinks(desc)
	if len(result) != 1 {
		t.Errorf("dedupe links: got len=%d, want 1", len(result))
	}
}

func TestExtractLinksTrimPunctuation(t *testing.T) {
	desc := "visit https://example.com."
	result := ExtractLinks(desc)
	if len(result) > 0 && strings.HasSuffix(result[0].URL, ".") {
		t.Errorf("trim punctuation: URL ends with '.': %s", result[0].URL)
	}
}

func TestExtractLinksLabel(t *testing.T) {
	// leading text before the URL becomes the label, with trailing separator
	// decoration (":,;-…" etc.) stripped from the right.
	desc := "Website: https://example.com"
	result := ExtractLinks(desc)
	if len(result) != 1 {
		t.Fatalf("labelled link: got len=%d, want 1", len(result))
	}
	if result[0].Label != "Website" {
		t.Errorf("label = %q, want %q", result[0].Label, "Website")
	}
}
