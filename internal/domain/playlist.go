package domain

import "time"

// WatchLaterYTID is YouTube's fixed playlist id for the Watch Later list. Used
// both to mutate it via the API and to address its local cache row.
const WatchLaterYTID = "WL"

// WatchLaterPlaylistName is the reserved name of the local playlist that backs
// Watch Later when the YouTube API is not initialized. When YT auth is present,
// Watch Later maps to YouTube's own "WL" playlist instead; offline it lives here
// as an ordinary local playlist so the queue keeps working without a connection.
const WatchLaterPlaylistName = "Watch Later"

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
