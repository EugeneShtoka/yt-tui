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
