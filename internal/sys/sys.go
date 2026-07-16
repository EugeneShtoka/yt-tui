// Package sys wraps the OS shell-outs the app makes (opening an editor, opening
// a URL) behind a small, testable boundary so they don't live inline in the UI
// dispatch layer.
package sys

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// EditorCommand builds the command to open path in the user's editor, resolving
// $VISUAL, then $EDITOR, then falling back to "vi". It returns the *exec.Cmd
// (rather than running it) so the caller can hand it to tea.ExecProcess, which
// suspends the TUI while the editor runs in the foreground.
func EditorCommand(path string) *exec.Cmd {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	return exec.CommandContext(context.Background(), editor, path) //nolint:gosec // $EDITOR is user-controlled by design
}

// OpenURL launches the URL in the desktop's default handler via xdg-open,
// without waiting for it to exit.
func OpenURL(url string) error {
	if err := exec.CommandContext(context.Background(), "xdg-open", url).Start(); err != nil {
		return fmt.Errorf("OpenURL: %w", err)
	}
	return nil
}
