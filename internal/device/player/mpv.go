package player

import (
	"fmt"
	"time"
)

type mpvDriver struct {
	path    string
	cookies CookieSource
}

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
	if title != "" && !isRemote(source) {
		args = append(args, "--force-media-title="+title)
	}
	args = append(args, d.ytdlOptions(source)...)
	if startAt > 0 {
		args = append(args, fmt.Sprintf("--start=%.0f", startAt.Seconds()))
	}
	return append(args, source)
}

func (d *mpvDriver) AudioArgs(source, title string, startAt time.Duration) []string {
	return append([]string{"--no-video"}, d.Args(source, title, startAt)...)
}

// ytdlOptions are the yt-dlp settings mpv's ytdl_hook needs for a remote source.
// The hook shells out to yt-dlp, so without cookies it extracts anonymously and
// YouTube refuses with "Sign in to confirm you're not a bot" — every playback
// fails while the rest of the app, which passes cookies, works fine.
//
// Each setting goes in its own --ytdl-raw-options-append: the plain
// --ytdl-raw-options form is a comma-separated list, so a single flag holding a
// cookie-file path that contains a comma would be silently split into two broken
// options. -append sets one key at a time and never splits the value.
func (d *mpvDriver) ytdlOptions(source string) []string {
	if !isRemote(source) {
		return nil
	}
	var opts []string
	if cookies := d.cookies.ytdlpOption(); cookies != "" {
		opts = append(opts, "--ytdl-raw-options-append="+cookies)
	}
	return opts
}
