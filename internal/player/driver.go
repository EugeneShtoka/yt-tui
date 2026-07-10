package player

import "time"

// Driver abstracts all player-specific CLI arguments.
type Driver interface {
	Path() string
	Args(source string, startAt time.Duration) []string
	AudioArgs(source string, startAt time.Duration) []string
	DBusName() string
}
