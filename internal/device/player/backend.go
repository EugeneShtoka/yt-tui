package player

import "time"

// Backend manages launching a video player and tracking playback position.
type Backend interface {
	// Launch starts the player at startAt and begins tracking position.
	// title is shown in the player window/OSD; pass "" to let the player choose.
	Launch(source, title string, startAt time.Duration) error
	// LaunchAudio starts the player in audio-only mode.
	LaunchAudio(source, title string, startAt time.Duration) error
	// Position returns the last known playback position; ok=false if not active.
	Position() (time.Duration, bool)
	// Wait blocks until the player process exits. Returns immediately if not active.
	Wait() error
	// Close stops tracking. Safe to call multiple times or when not playing.
	Close()
}
