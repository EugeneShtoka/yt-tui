package domain

import "time"

// VideoStatus represents the playback state of a local video.
type VideoStatus string

const (
	StatusNew     VideoStatus = "new"
	StatusStarted VideoStatus = "started"
	StatusWatched VideoStatus = "watched"
)

// LocalVideo is a video that has been downloaded to disk.
type LocalVideo struct {
	ID             string
	Title          string
	Channel        string
	Duration       int
	ViewCount      int64
	UploadDate     string
	FilePath       string
	DownloadType   string // "video" or "audio"
	DownloadedAt   time.Time
	Status         VideoStatus
	LastPlayed     time.Time
	LastPositionMs int64
}

func (lv LocalVideo) GetBaseTitle() string { return lv.Title }
func (lv LocalVideo) GetTitle() string     { return lv.Title }
func (lv LocalVideo) IsAudio() bool        { return lv.DownloadType == "audio" }

// LocalVideo has no ChannelID; GetChannelID returns "" so alias lookup falls back to name.
func (lv LocalVideo) GetChannelID() string   { return "" }
func (lv LocalVideo) GetChannelName() string { return lv.Channel }

func (lv LocalVideo) GetCount() int64    { return lv.ViewCount }
func (lv LocalVideo) GetRawDate() string { return lv.UploadDate }

func (lv LocalVideo) GetDurationSecs() int     { return lv.Duration }
func (lv LocalVideo) GetLastPositionSecs() int { return int(lv.LastPositionMs / 1000) }

func (lv LocalVideo) GetIndicator() string {
	switch lv.Status {
	case StatusNew:
		return " ● "
	case StatusStarted, StatusWatched:
		return " ○ "
	}
	return "   "
}

