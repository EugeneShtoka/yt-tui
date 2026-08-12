package player

import "time"

// Driver abstracts all player-specific CLI arguments.
type Driver interface {
	Path() string
	Args(source, title string, startAt time.Duration) []string
	AudioArgs(source, title string, startAt time.Duration) []string
	DBusName() string
}
