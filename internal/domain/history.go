package domain

import (
	"fmt"
	"strings"
	"time"
)

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

func (e HistoryEntry) GetBaseTitle() string { return e.Title }
func (e HistoryEntry) GetTitle() string     { return e.Title }
func (e HistoryEntry) IsAudio() bool        { return isAudioEvent(e.EventType) }

func (e HistoryEntry) GetChannelID() string   { return e.ChannelID }
func (e HistoryEntry) GetChannelName() string { return e.Channel }

func (e HistoryEntry) GetCount() int64    { return e.ViewCount }
func (e HistoryEntry) GetRawDate() string { return e.UploadDate }

// GetDurationSecs returns the video duration. GetLastPositionSecs is 0 by default;
// use HistoryRow (in the tab layer) to pre-enrich with a live position.
func (e HistoryEntry) GetDurationSecs() int     { return e.Duration }
func (e HistoryEntry) GetLastPositionSecs() int { return 0 }

func (e HistoryEntry) GetIndicator() string {
	if isAudioEvent(e.EventType) || strings.HasPrefix(e.EventType, "download") {
		return " ● "
	}
	return " ○ "
}

// GetLabel returns a simplified event category for the Type column.
func (e HistoryEntry) GetLabel() string {
	switch {
	case strings.HasPrefix(e.EventType, "stream"):
		return "Streamed"
	case strings.HasPrefix(e.EventType, "download"):
		return "Downloaded"
	case strings.HasPrefix(e.EventType, "play"):
		return "Played"
	}
	return e.EventType
}

// GetTimestampRawDate returns the event timestamp as YYYYMMDD for the detail table's date column.
func (e HistoryEntry) GetTimestampRawDate() string {
	return e.Timestamp.Format("20060102")
}

func isAudioEvent(eventType string) bool {
	return eventType == "streamAudio" || eventType == "download audio" || eventType == "playAudio"
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

func (e ActivityEntry) GetLabel() string { return e.Type }

func (e ActivityEntry) GetActivityDetail() string {
	locality := "remote"
	if e.IsLocal {
		locality = "local"
	}
	switch e.Type {
	case "subscribe":
		return fmt.Sprintf("%s (%s)", e.ChannelName, locality)
	case "create_playlist":
		return fmt.Sprintf("%s (%s)", e.PlaylistName, locality)
	case "add_to_playlist":
		return fmt.Sprintf("%s → %s (%s)", e.VideoTitle, e.PlaylistName, locality)
	}
	return e.Type
}
