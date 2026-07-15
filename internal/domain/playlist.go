package domain

import "time"

// YTPlaylist is a YouTube playlist (ID + title).
type YTPlaylist struct {
	ID    string
	Title string
}

// Playlist is a user-created local playlist.
type Playlist struct {
	ID        int64
	Name      string
	CreatedAt time.Time
}

// WatchLaterEntry is a video queued for later viewing.
type WatchLaterEntry struct {
	VideoID string
	Title   string
	Channel string
	URL     string
	AddedAt time.Time
}
