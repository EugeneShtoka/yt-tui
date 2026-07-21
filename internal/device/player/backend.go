package player

import "time"

// Backend manages launching a video player and tracking playback position.
type Backend interface {
	// Launch starts the player at startAt and returns a Session for lifecycle
	// and position tracking.
	Launch(source, title string, startAt time.Duration) (*Session, error)
	// LaunchAudio starts the player in audio-only mode.
	LaunchAudio(source, title string, startAt time.Duration) (*Session, error)
	// Close stops any active playback. Safe to call when idle.
	Close()
}
