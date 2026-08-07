package downloader

import (
	"strings"
	"testing"
)

func TestSanitizeFilenameInvalidChars(t *testing.T) {
	tests := map[string]bool{
		"/": true, "\\": true, ":": true, "*": true, "?": true,
		"\"": true, "<": true, ">": true, "|": true,
	}
	for char := range tests {
		name := "file" + char + "name"
		result := sanitizeFilename(name)
		if strings.Contains(result, char) {
			t.Errorf("sanitize %q: invalid char '%s' not replaced", name, char)
		}
	}
}

func TestSanitizeFilenameEmpty(t *testing.T) {
	result := sanitizeFilename("")
	if result != "download" {
		t.Errorf("empty filename: got %q, want 'download'", result)
	}
}

func TestSanitizeFilenameWhitespace(t *testing.T) {
	result := sanitizeFilename("   ")
	if result != "download" {
		t.Errorf("whitespace-only filename: got %q, want 'download'", result)
	}
}

func TestSanitizeFilenameValidChars(t *testing.T) {
	name := "my-video_2025 (1).mp4"
	result := sanitizeFilename(name)
	if result != name {
		t.Errorf("valid filename changed: got %q, want %q", result, name)
	}
}

func TestSanitizeFilenameNullByte(t *testing.T) {
	name := "file\x00name"
	result := sanitizeFilename(name)
	if strings.Contains(result, "\x00") {
		t.Errorf("null byte not sanitized in %q", result)
	}
}
