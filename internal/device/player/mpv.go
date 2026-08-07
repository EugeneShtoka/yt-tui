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
	var args []string
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
