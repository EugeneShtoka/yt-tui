package render

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	runewidth "github.com/mattn/go-runewidth"
)

// Column widths shared across all video-list views.
// ColDuration and ColDate are vars because they change with the active format.
const (
	ColNum     = 5 // line number; fits up to 99999 rows
	ColChannel = 30
	ColViews   = 8 // max content: "100.0K"
	ColSize    = 8 // max content: "1023.9M" / "9.9G"
)

var (
	ColDuration = 8  // recomputed by SetDurFmt
	ColDate     = 10 // recomputed by SetDateFmt; all built-in formats are 10 chars
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

// ActiveDurFmt reports the current duration format, letting callers save and
// restore the process-wide setting (e.g. tests that exercise a specific format).
func ActiveDurFmt() DurFmt { return activeDurFmt }

// SetDurFmt sets the active duration format and recomputes ColDuration.
// Typically called once at startup after loading config, but it is idempotent —
// safe to re-apply (e.g. across test cases). Unrecognized values fall back to hh:mm.
func SetDurFmt(f DurFmt) {
	switch f {
	case DurFmtHHMMSS, DurFmthhmmss, DurFmtHHMM, DurFmthHmm, DurFmthhmm,
		DurFmtMMMSS, DurFmtmmmss, DurFmtMMM, DurFmtmMM, DurFmtmmm:
		activeDurFmt = f
	default:
		activeDurFmt = DurFmthhmm
	}
	ColDuration = len(formatDuration(99*3600+59*60+59, activeDurFmt))
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

// SetDateFmt sets the active date format and recomputes ColDate. Typically called
// once at startup after loading config, but it is idempotent — safe to re-apply
// (e.g. across test cases). Unrecognized values fall back to dd/mm/yyyy.
func SetDateFmt(f DateFmt) {
	switch f {
	case DateFmtDMY, DateFmtMDY, DateFmtYMD, DateFmtDMYDash:
		activeDateFmt = f
	default:
		activeDateFmt = DateFmtDMY
	}
	ColDate = len(formatDate("20260721", activeDateFmt))
}

// Duration formats a second count using the active DurFmt (see SetDurFmt).
// Non-positive values render blank so unknown durations leave the column empty.
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

// Views renders a view/subscriber count in compact form (K/M/B). Zero renders
// blank so unknown counts leave the column empty rather than "0".
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

// Size renders a byte count in binary units (KiB/MiB/GiB, base 1024) with the
// short suffixes K/M/G, matching Views' compact style. Zero renders blank so
// unknown sizes (pre-backfill rows) leave the column empty rather than "0".
func Size(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(bytes)/(1<<10))
	case bytes > 0:
		return fmt.Sprintf("%dB", bytes)
	}
	return ""
}

// Date formats a "YYYYMMDD" string using the active DateFmt (see SetDateFmt).
// Inputs that aren't 8 characters pass through unchanged.
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

// ClampLine forces s onto a single physical line of exactly w display columns,
// measured with charmbracelet/x/ansi — the width authority lipgloss and the
// terminal renderer use. It strips stray control characters (a carriage return
// or backspace in a video title snaps the terminal cursor and corrupts the row),
// collapses embedded newlines, truncates overflow, and pads short lines, so
// composed layout blocks can never word-wrap onto an extra line or misalign
// against their neighbors (which pushes borders off their column and shifts
// subsequent rows).
func ClampLine(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = Sanitize(s)
	s = ansi.Truncate(s, w, "")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// OverlayCenter composites the already-styled box centered — horizontally and
// vertically — over behind, overwriting the underlying character cells, and
// returns the combined frame. width is the available content width, used both
// to center the box and to keep the box within bounds. Because it draws on top
// of behind (rather than appending), a popup stays visible even when the
// content beneath it fills the whole area. It is the shared compositor behind
// the modal overlays and the in-tab mode picker.
func OverlayCenter(behind, box string, width int) string {
	boxLines := strings.Split(box, "\n")
	boxW := 0
	for _, l := range boxLines {
		if w := ansi.StringWidth(l); w > boxW {
			boxW = w
		}
	}
	boxH := len(boxLines)

	x := (width - boxW) / 2
	if x < 0 {
		x = 0
	}
	behindLines := strings.Split(behind, "\n")
	y := (len(behindLines) - boxH) / 2
	if y < 0 {
		y = 0
	}
	for i, ol := range boxLines {
		lineIdx := y + i
		for lineIdx >= len(behindLines) {
			behindLines = append(behindLines, "")
		}
		row := behindLines[lineIdx]
		if visW := ansi.StringWidth(row); visW < x {
			row += strings.Repeat(" ", x-visW)
		}
		left := ansi.Truncate(row, x, "")
		right := ansi.TruncateLeft(row, x+boxW, "")
		behindLines[lineIdx] = left + ol + right
	}
	return strings.Join(behindLines, "\n")
}

// Sanitize removes zero-width C0/C1 control characters that desync the terminal
// cursor (e.g. CR, BS, VT) while preserving ESC so ANSI color/SGR sequences
// survive. Newlines and tabs become spaces, collapsing the text to one line.
// Returns s unchanged when it holds no such characters (the common case).
// Apply to any untrusted single-line text (titles, link labels, chapter names)
// before measuring or composing it into a layout.
func Sanitize(s string) string {
	if !strings.ContainsFunc(s, isBadControl) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteByte(' ')
		case isBadControl(r):
			// drop CR, BS, and other cursor-moving controls
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isBadControl reports whether r is a control character that must not reach the
// terminal verbatim. ESC (0x1B) is excluded because it introduces ANSI escapes.
func isBadControl(r rune) bool {
	if r == 0x1b {
		return false
	}
	return r == '\n' || r == '\t' || r < 0x20 || (r >= 0x7f && r <= 0x9f)
}

// Truncate shortens s to at most n display columns, appending an ellipsis when
// it overflows. Width is measured in terminal cells, so wide runes count as two.
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

// WordWrap splits text into lines of at most width visible characters,
// breaking at word boundaries. Long tokens are hard-broken. Newlines preserved.
func WordWrap(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	// Descriptions often use CRLF (or lone CR) line endings. Normalise them to
	// LF so a stray carriage return never survives into a rendered line, where
	// it would snap the terminal cursor to column 0 and corrupt the row.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var result []string
	for _, para := range strings.Split(text, "\n") {
		if ansi.StringWidth(para) <= width {
			result = append(result, para)
			continue
		}
		words := strings.Fields(para)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		cur := ""
		for _, w := range words {
			if ansi.StringWidth(w) > width {
				if cur != "" {
					result = append(result, cur)
					cur = ""
				}
				result = append(result, hardBreak(w, width)...)
				continue
			}
			candidate := w
			if cur != "" {
				candidate = cur + " " + w
			}
			if ansi.StringWidth(candidate) <= width {
				cur = candidate
			} else {
				result = append(result, cur)
				cur = w
			}
		}
		if cur != "" {
			result = append(result, cur)
		}
	}
	return result
}

// hardBreak splits a single word that is wider than width into chunks of at
// most width visible cells, breaking mid-rune-run as needed. Each chunk is at
// least one rune so the loop always makes progress.
func hardBreak(w string, width int) []string {
	var chunks []string
	runes := []rune(w)
	for len(runes) > 0 {
		taken, col := 0, 0
		for taken < len(runes) {
			cw := RuneWidth(runes[taken])
			if col+cw > width {
				break
			}
			col += cw
			taken++
		}
		if taken == 0 {
			taken = 1
		}
		chunks = append(chunks, string(runes[:taken]))
		runes = runes[taken:]
	}
	return chunks
}

// RuneWidth returns the terminal cell width of a single rune.
func RuneWidth(r rune) int {
	// Fast path: ASCII is always 1 wide.
	if r < 128 {
		return 1
	}
	return ansi.StringWidth(string(r))
}

// ShortenURLs replaces URLs longer than maxLen in text with "domain/…" form.
// This prevents long hyperlinks in video descriptions from overflowing the panel.
func ShortenURLs(text string, maxLen int) string {
	paras := strings.Split(text, "\n")
	for i, para := range paras {
		words := strings.Fields(para)
		changed := false
		for j, w := range words {
			if len(w) > maxLen && (strings.HasPrefix(w, "http://") || strings.HasPrefix(w, "https://")) {
				words[j] = abbreviateURL(w)
				changed = true
			}
		}
		if changed {
			paras[i] = strings.Join(words, " ")
		}
	}
	return strings.Join(paras, "\n")
}

func abbreviateURL(u string) string {
	rest := u
	if i := strings.Index(u, "://"); i >= 0 {
		rest = u[i+3:]
	}
	rest = strings.TrimPrefix(rest, "www.")
	domain := rest
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		domain = rest[:j]
	}
	if domain == "" {
		return u
	}
	return domain + "/…"
}
