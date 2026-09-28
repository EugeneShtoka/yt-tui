package player

import (
	"fmt"
	"time"
)

// vlcDriver takes no CookieSource: VLC resolves YouTube URLs with its own bundled
// Lua playlist script rather than yt-dlp, and that script has no way to be handed
// cookies. Authenticated YouTube playback therefore needs mpv.
type vlcDriver struct{ path string }

func (d *vlcDriver) Path() string     { return d.path }
func (d *vlcDriver) DBusName() string { return "org.mpris.MediaPlayer2.vlc" }

func (d *vlcDriver) Args(source, _ string, startAt time.Duration) []string {
	if startAt > 0 {
		return []string{fmt.Sprintf("--start-time=%.0f", startAt.Seconds()), source}
	}
	return []string{source}
}

func (d *vlcDriver) AudioArgs(source, title string, startAt time.Duration) []string {
	return append([]string{"--novideo"}, d.Args(source, title, startAt)...)
}
