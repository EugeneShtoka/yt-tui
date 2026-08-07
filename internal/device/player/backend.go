package player

import "time"

// Backend manages launching a video player and tracking playback position.
type Backend interface {
	// Launch starts the player at startAt and returns a Session for lifecycle
	// and position tracking. videoID identifies the video being played so the
	// backend can report it via Active (used by the post-quit position tracker).
	Launch(videoID, source, title string, startAt time.Duration) (*Session, error)
	// LaunchAudio starts the player in audio-only mode.
	LaunchAudio(videoID, source, title string, startAt time.Duration) (*Session, error)
	// Active reports the currently-playing video and its OS process, when the
	// backend supports position tracking and a player is running. It lets the
	// app hand off to a background tracker that keeps saving resume position
	// after the TUI exits (ok=false for backends without position tracking, or
	// when nothing is playing).
	Active() (ActivePlayback, bool)
	// Close stops any active playback. Safe to call when idle.
	Close()
}

// ActivePlayback identifies a running playback for hand-off to the background
// position tracker.
type ActivePlayback struct {
	VideoID string
	PID     int
}
