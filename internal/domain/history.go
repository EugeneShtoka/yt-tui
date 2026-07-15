package domain

import "time"

// HistoryEntry is a single event from the history or activity log.
type HistoryEntry struct {
	ID         int64
	VideoID    string
	Title      string
	Channel    string
	ChannelID  string
	Duration   int
	ViewCount  int64
	UploadDate string
	EventType  string
	Details    string
	Timestamp  time.Time
}

// ActivityEntry records a user action such as subscribe or playlist modification.
type ActivityEntry struct {
	ID              int64
	Type            string // "subscribe", "create_playlist", "add_to_playlist"
	IsLocal         bool
	ChannelID       string
	ChannelName     string
	PlaylistID      string // YT playlist ID
	PlaylistLocalID int64  // local playlist DB ID
	PlaylistName    string
	VideoID         string
	VideoTitle      string
	Timestamp       time.Time
}
