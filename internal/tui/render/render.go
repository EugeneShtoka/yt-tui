package render

import (
	"fmt"

	runewidth "github.com/mattn/go-runewidth"
)

// Column widths shared across all video-list views.
const (
	ColNum      = 4
	ColChannel  = 22
	ColDuration = 13
	ColViews    = 8
	ColDate     = 11
)

func Duration(secs int) string {
	if secs <= 0 {
		return ""
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d", h, m)
	}
	return fmt.Sprintf("%d", m)
}

func DurationWithPos(posMs int64, totalSecs int) string {
	return Duration(int(posMs/1000)) + "/" + Duration(totalSecs)
}

func Views(n int64) string {
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

func Date(yyyymmdd string) string {
	if len(yyyymmdd) != 8 {
		return yyyymmdd
	}
	return yyyymmdd[6:] + "/" + yyyymmdd[4:6] + "/" + yyyymmdd[:4]
}

func FormatEvent(s string) string {
	switch s {
	case "streamVideo":
		return "Stream video"
	case "streamAudio":
		return "Stream audio"
	case "playVideo":
		return "Play video"
	case "playAudio":
		return "Play audio"
	case "download video":
		return "Download video"
	case "download audio":
		return "Download audio"
	}
	return s
}

func Truncate(s string, n int) string {
	if runewidth.StringWidth(s) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	runes := []rune(s)
	var w, i int
	for i < len(runes) {
		if w+runewidth.RuneWidth(runes[i]) > n-1 {
			break
		}
		w += runewidth.RuneWidth(runes[i])
		i++
	}
	return string(runes[:i]) + "…"
}
