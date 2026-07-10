package player

import (
	"fmt"
	"time"
)

type mpvDriver struct{ path string }

func (d *mpvDriver) Path() string     { return d.path }
func (d *mpvDriver) DBusName() string { return "org.mpris.MediaPlayer2.mpv" }

func (d *mpvDriver) Args(source string, startAt time.Duration) []string {
	if startAt > 0 {
		return []string{fmt.Sprintf("--start=%.0f", startAt.Seconds()), source}
	}
	return []string{source}
}

func (d *mpvDriver) AudioArgs(source string, startAt time.Duration) []string {
	return append([]string{"--no-video"}, d.Args(source, startAt)...)
}
