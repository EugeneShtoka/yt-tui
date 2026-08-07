package domain

// VideoRef is the minimal (id, url) pair the enrichment passes work with: the
// id keys caches/thumbnails, the url is handed to yt-dlp for a full fetch. It
// lives in domain so the db layer and the backend enricher can share it without
// the backend importing db.
type VideoRef struct {
	ID  string
	URL string
}

// Video is a YouTube video entry.
type Video struct {
	ID         string
	Title      string
	Channel    string
	ChannelID  string
	Duration   int // seconds
	ViewCount  int64
	UploadDate string // YYYYMMDD
	URL        string
}

// RawChapter is a chapter as returned by yt-dlp (unprocessed timecodes).
type RawChapter struct {
	Title     string
	StartTime float64
	EndTime   float64
}

// VideoDetails is a Video with additional metadata from a full yt-dlp fetch.
type VideoDetails struct {
	Video
	Description  string
	ThumbnailURL string
	Subscribers  int64
	Chapters     []RawChapter
	Language     string      // original audio language (BCP-47-ish code, e.g. "en", "ru"); "" when yt-dlp omits it
	SBSegments   []SBSegment // SponsorBlock cut ranges (original timeline); populated only when a fetch marks them
}
