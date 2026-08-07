package app

import (
	"errors"
	"testing"

	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// withClipboard swaps the clipboard write seam for a fake for the duration of a
// test, restoring the real one afterward.
func withClipboard(t *testing.T, fn func(string) error) {
	t.Helper()
	orig := clipboardWrite
	clipboardWrite = fn
	t.Cleanup(func() { clipboardWrite = orig })
}

func TestCopyCmdSuccess(t *testing.T) {
	var got string
	withClipboard(t, func(s string) error { got = s; return nil })

	msg := copyCmd("https://example.com", "Copied: https://example.com")()
	sm, ok := msg.(tuipkg.StatusMsg)
	if !ok || sm.IsErr {
		t.Fatalf("want a success StatusMsg, got %#v", msg)
	}
	if got != "https://example.com" {
		t.Errorf("clipboard got %q, want the URL", got)
	}
	if sm.Text != "Copied: https://example.com" {
		t.Errorf("status = %q", sm.Text)
	}
}

func TestCopyCmdError(t *testing.T) {
	withClipboard(t, func(string) error { return errors.New("no clipboard") })

	sm, ok := copyCmd("x", "done")().(tuipkg.StatusMsg)
	if !ok || !sm.IsErr {
		t.Fatalf("want an error StatusMsg, got %#v", sm)
	}
}

func TestCopyHandlersDelegateToClipboard(t *testing.T) {
	var got string
	withClipboard(t, func(s string) error { got = s; return nil })
	r := Root{}

	_, cmd := r.handleCopyText(tuipkg.CopyTextMsg{Text: "transcript body", Label: "transcript"})
	sm := cmd().(tuipkg.StatusMsg)
	if got != "transcript body" {
		t.Errorf("copy-text clipboard got %q", got)
	}
	if sm.Text != "Copied transcript to clipboard" {
		t.Errorf("status = %q", sm.Text)
	}
}
