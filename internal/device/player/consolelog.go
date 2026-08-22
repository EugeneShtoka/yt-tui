package player

import (
	"os"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/debug"
)

// consoleTailBytes caps how much of a player's console output is kept. The
// interesting part of a failed launch is always the last few lines (yt-dlp's
// error, then the player giving up), so a small tail is enough and a chatty
// player cannot grow memory without bound.
const consoleTailBytes = 4 << 10

// consoleLog captures a player process's console output so a launch that never
// plays anything can be explained. mpv resolves YouTube URLs through yt-dlp (its
// ytdl_hook) and reports yt-dlp's own errors — "HTTP Error 403", "Sign in to
// confirm you're not a bot" — on its terminal output, which used to go straight
// to /dev/null. That discarded text is the difference between "playback silently
// did nothing" and a cause we can name.
//
// Both streams land in one file: mpv prints its messages, ytdl_hook errors
// included, on stdout, while other players use stderr — and interleaving them
// preserves the order the player wrote them in.
//
// It is an unlinked temp file rather than a pipe on purpose. A pipe would block
// the player once 64 KiB accumulated with nobody reading, and both backends
// deliberately let the player outlive the TUI. A file never blocks a writer;
// unlinking it at creation means nothing is left on disk, the child keeps writing
// through its inherited fd, we keep reading through ours, and the inode is freed
// as soon as the last one closes.
type consoleLog struct{ f *os.File }

// newConsoleLog creates the capture file, returning nil when it cannot — output
// then simply goes to /dev/null as before, since losing the diagnostics must
// never cost the user their playback.
func newConsoleLog() *consoleLog {
	f, err := os.CreateTemp("", "yt-tui-player-*.log")
	if err != nil {
		debug.Log("player: console capture unavailable: %v", err)
		return nil
	}
	if err := os.Remove(f.Name()); err != nil {
		debug.Log("player: unlinking console capture: %v", err)
	}
	return &consoleLog{f: f}
}

// file is the writer to hand the child process, or nil when capture is off.
func (e *consoleLog) file() *os.File {
	if e == nil {
		return nil
	}
	return e.f
}

// tail returns the last consoleTailBytes the player wrote, trimmed. Safe on a nil
// consoleLog, an empty log, and a read that fails.
func (e *consoleLog) tail() string {
	if e == nil {
		return ""
	}
	info, err := e.f.Stat()
	if err != nil || info.Size() == 0 {
		return ""
	}
	size := min(info.Size(), int64(consoleTailBytes))
	buf := make([]byte, size)
	// ReadAt reports io.EOF alongside a short final read, so the byte count — not
	// the error — decides whether there is anything to show.
	n, _ := e.f.ReadAt(buf, info.Size()-size)
	if n == 0 {
		return ""
	}
	return strings.TrimSpace(string(buf[:n]))
}

func (e *consoleLog) close() {
	if e != nil {
		_ = e.f.Close()
	}
}
