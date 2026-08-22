package player

import (
	"fmt"
	"strings"
	"time"
)

type mpvDriver struct{ path string }

func (d *mpvDriver) Path() string     { return d.path }
func (d *mpvDriver) DBusName() string { return "org.mpris.MediaPlayer2.mpv" }

func (d *mpvDriver) Args(source, title string, startAt time.Duration) []string {
	// mpv repeats a status line for every frame of playback, and that output is
	// captured (see consoleLog) so a failed launch can be explained. Blanking the
	// status message keeps the capture to the messages that matter — the errors —
	// instead of a megabyte an hour of progress updates.
	args := []string{"--term-status-msg="}
	// For local files force the title; for URLs yt-dlp will set it, avoiding a second
	// MPRIS metadata update that triggers a duplicate desktop notification.
	if title != "" && !strings.HasPrefix(source, "http") {
		args = append(args, "--force-media-title="+title)
	}
	if startAt > 0 {
		args = append(args, fmt.Sprintf("--start=%.0f", startAt.Seconds()))
	}
	return append(args, source)
}

func (d *mpvDriver) AudioArgs(source, title string, startAt time.Duration) []string {
	return append([]string{"--no-video"}, d.Args(source, title, startAt)...)
}
