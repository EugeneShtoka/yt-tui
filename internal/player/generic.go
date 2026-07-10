package player

import "time"

type genericDriver struct{ path string }

func (d *genericDriver) Path() string     { return d.path }
func (d *genericDriver) DBusName() string { return "org.mpris.MediaPlayer2." + baseName(d.path) }

func (d *genericDriver) Args(source string, _ time.Duration) []string {
	return []string{source}
}

func (d *genericDriver) AudioArgs(source string, startAt time.Duration) []string {
	return d.Args(source, startAt)
}
