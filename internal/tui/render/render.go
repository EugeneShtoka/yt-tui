package render

import (
	"fmt"
	"sync"

	runewidth "github.com/mattn/go-runewidth"
)

// Column widths shared across all video-list views.
// ColDuration and ColDate are vars because they change with the active format.
const (
	ColNum     = 4
	ColChannel = 30
	ColViews   = 8 // max content: "100.0K"
)

var (
	ColDuration    = 8  // recomputed by SetDurFmt
	ColDurationPos = 17 // pos/total: 2*ColDuration+1, recomputed by SetDurFmt
	ColDate        = 10 // recomputed by SetDateFmt; all built-in formats are 10 chars
)

// DurFmt controls how video durations are formatted in all table views.
// Uppercase component letters = zero-padded; lowercase = no padding.
// hh/HH = hours, mm/MM = component minutes, ss/SS = seconds, mmm/mMM/MMM = total minutes.
// A lowercase hh prefix also suppresses the hours field when it is zero.
type DurFmt string

const (
	DurFmtHHMMSS DurFmt = "HH:MM:SS" // 01:05:30  — always show hours, all padded
	DurFmthhmmss DurFmt = "hh:mm:ss" // 1:05:30, or 5:30 when under 1 h
	DurFmtHHMM   DurFmt = "HH:MM"    // 01:05     — always show hours, all padded
	DurFmthHmm   DurFmt = "hH:mm"    // 0:45, 1:05 — always show hours, unpadded
	DurFmthhmm   DurFmt = "hh:mm"    // 1:05, or just 5 when under 1 h
	DurFmtMMMSS  DurFmt = "MMM:SS"   // 065:05    — total min padded to 3 digits
	DurFmtmmmss  DurFmt = "mmm:ss"   // 65:5      — total min and sec, both unpadded
	DurFmtMMM    DurFmt = "MMM"      // 065       — total min padded to 3 digits
	DurFmtmMM    DurFmt = "mMM"      // 65        — total min padded to 2 digits
	DurFmtmmm    DurFmt = "mmm"      // 65        — total min, no padding
)

var activeDurFmt DurFmt = DurFmthhmmss

var durFmtOnce sync.Once

// SetDurFmt sets the active duration format and recomputes ColDuration and ColDurationPos.
// Call once at startup after loading config. Unrecognized values fall back to hh:mm.
// Panics if called more than once.
func SetDurFmt(f DurFmt) {
	called := false
	durFmtOnce.Do(func() {
		called = true
		switch f {
		case DurFmtHHMMSS, DurFmthhmmss, DurFmtHHMM, DurFmthHmm, DurFmthhmm,
			DurFmtMMMSS, DurFmtmmmss, DurFmtMMM, DurFmtmMM, DurFmtmmm:
			activeDurFmt = f
		default:
			activeDurFmt = DurFmthhmm
		}
		ColDuration = len(formatDuration(99*3600+59*60+59, activeDurFmt))
		ColDurationPos = 2*ColDuration + 1
	})
	if !called {
		panic("render.SetDurFmt called more than once")
	}
}

// DateFmt controls how dates are displayed in all table views.
type DateFmt string

const (
	DateFmtDMY     DateFmt = "dd/mm/yyyy" // 21/07/2026 — default
	DateFmtMDY     DateFmt = "mm/dd/yyyy" // 07/21/2026
	DateFmtYMD     DateFmt = "yyyy-mm-dd" // 2026-07-21
	DateFmtDMYDash DateFmt = "dd-mm-yyyy" // 21-07-2026
)

var activeDateFmt DateFmt = DateFmtDMY

var dateFmtOnce sync.Once

// SetDateFmt sets the active date format and recomputes ColDate.
// Call once at startup after loading config. Unrecognized values fall back to dd/mm/yyyy.
// Panics if called more than once.
func SetDateFmt(f DateFmt) {
	called := false
	dateFmtOnce.Do(func() {
		called = true
		switch f {
		case DateFmtDMY, DateFmtMDY, DateFmtYMD, DateFmtDMYDash:
			activeDateFmt = f
		default:
			activeDateFmt = DateFmtDMY
		}
		ColDate = len(formatDate("20260721", activeDateFmt))
	})
	if !called {
		panic("render.SetDateFmt called more than once")
	}
}

func Duration(secs int) string {
	if secs <= 0 {
		return ""
	}
	return formatDuration(secs, activeDurFmt)
}

func formatDuration(secs int, f DurFmt) string {
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	totalM := secs / 60
	switch f {
	case DurFmtHHMMSS:
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	case DurFmthhmmss:
		if h > 0 {
			return fmt.Sprintf("%d:%02d:%02d", h, m, s)
		}
		return fmt.Sprintf("%d:%02d", m, s)
	case DurFmtHHMM:
		return fmt.Sprintf("%02d:%02d", h, m)
	case DurFmthHmm:
		return fmt.Sprintf("%d:%02d", h, m)
	case DurFmthhmm:
		if h > 0 {
			return fmt.Sprintf("%d:%02d", h, m)
		}
		return fmt.Sprintf("%d", m)
	case DurFmtMMMSS:
		return fmt.Sprintf("%03d:%02d", totalM, s)
	case DurFmtmmmss:
		return fmt.Sprintf("%d:%d", totalM, s)
	case DurFmtMMM:
		return fmt.Sprintf("%03d", totalM)
	case DurFmtmMM:
		return fmt.Sprintf("%02d", totalM)
	case DurFmtmmm:
		return fmt.Sprintf("%d", totalM)
	default:
		if h > 0 {
			return fmt.Sprintf("%d:%02d", h, m)
		}
		return fmt.Sprintf("%d", m)
	}
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
	return formatDate(yyyymmdd, activeDateFmt)
}

func formatDate(yyyymmdd string, f DateFmt) string {
	y, m, d := yyyymmdd[:4], yyyymmdd[4:6], yyyymmdd[6:]
	switch f {
	case DateFmtMDY:
		return m + "/" + d + "/" + y
	case DateFmtYMD:
		return y + "-" + m + "-" + d
	case DateFmtDMYDash:
		return d + "-" + m + "-" + y
	default: // DateFmtDMY
		return d + "/" + m + "/" + y
	}
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
