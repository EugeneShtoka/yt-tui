package player

import "time"

// Backend manages launching a video player and tracking playback position.
type Backend interface {
	// Launch starts the player at startAt and begins tracking position.
	Launch(filePath string, startAt time.Duration) error
	// Position returns the last known playback position; ok=false if not active.
	Position() (time.Duration, bool)
	// Close stops tracking. Safe to call multiple times or when not playing.
	Close()
}
