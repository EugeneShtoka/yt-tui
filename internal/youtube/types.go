package youtube

import (
	"fmt"
	"strings"
)

func StripEmojis(s string) string {
	result := strings.Map(func(r rune) rune {
		if (r >= 0x1F000 && r <= 0x1FFFF) ||
			(r >= 0x2600 && r <= 0x27BF) ||
			(r >= 0x2B00 && r <= 0x2BFF) ||
			(r >= 0xFE00 && r <= 0xFE0F) ||
			r == 0x200D || r == 0x20E3 {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(result), " ")
}

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

func (v Video) DurationStr() string {
	s := v.Duration
	h := s / 3600
	m := (s % 3600) / 60
	sec := s % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
}

func (v Video) ViewsStr() string {
	n := v.ViewCount
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	case n > 0:
		return fmt.Sprintf("%d", n)
	}
	return ""
}

func (v Video) DateStr() string {
	if len(v.UploadDate) != 8 {
		return v.UploadDate
	}
	y := v.UploadDate[:4]
	m := v.UploadDate[4:6]
	d := v.UploadDate[6:8]
	return d + "/" + m + "/" + y
}

type Channel struct {
	ID          string
	Name        string
	Alias       string   // user-defined display name override
	Tags        []string // user-defined categories
	URL         string
	Subscribers int64
	IsLocal     bool // subscribed locally, not via YouTube API
}

func (ch Channel) DisplayName() string {
	if ch.Alias != "" {
		return ch.Alias
	}
	return ch.Name
}

type Chapter struct {
	Title     string
	StartTime float64
	EndTime   float64
}

type VideoDetails struct {
	Video
	Description  string
	ThumbnailURL string
	Subscribers  int64
	Chapters     []Chapter
}
