package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"

	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// clipboardWrite copies text to the system clipboard. It is a package var so
// the copy handlers can be tested without a real clipboard, and so Root no
// longer depends on the clipboard package directly (H-2 seam).
var clipboardWrite = clipboard.WriteAll

// copyCmd writes text to the clipboard off the event loop and reports the
// outcome as a StatusMsg (done on success, the error otherwise). Shared by the
// copy-URL and copy-text handlers.
func copyCmd(text, done string) tea.Cmd {
	return func() tea.Msg {
		if err := clipboardWrite(text); err != nil {
			return tuipkg.StatusMsg{Text: "clipboard: " + err.Error(), IsErr: true}
		}
		return tuipkg.StatusMsg{Text: done}
	}
}
