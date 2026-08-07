package transcripts

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	tagRE   = regexp.MustCompile(`<[^>]*>`)        // <c>, <00:00:01.000>, <i> …
	indexRE = regexp.MustCompile(`^\d+$`)          // SRT cue index line
	tsRE    = regexp.MustCompile(`\d\d:\d\d:\d\d`) // timestamp line contains hh:mm:ss
)

// entityReplacer decodes the handful of XML entities yt-dlp emits in captions.
var entityReplacer = strings.NewReplacer(
	"&amp;", "&", "&lt;", "<", "&gt;", ">", "&#39;", "'", "&quot;", `"`, "&nbsp;", " ",
)

// Cue is a single caption fragment with its start offset (seconds) in the video
// timeline. SRTToCues preserves these start times so callers can bucket the
// transcript under chapters; SRTToText discards them.
type Cue struct {
	Start float64
	Text  string
}

// SRTToCues parses SRT/VTT-style subtitle text into timed cues: it captures each
// caption's start offset from its "HH:MM:SS,mmm --> ..." line, strips markup tags
// and entities from the text, and collapses the rolling duplication typical of
// YouTube auto-captions (each cue repeating the previous line). Each surviving
// text line becomes one Cue carrying the start of the caption it belongs to.
func SRTToCues(srt string) []Cue {
	var out []Cue
	var last string
	var start float64
	for _, raw := range strings.Split(srt, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "",
			indexRE.MatchString(line): // cue index
			continue
		case strings.Contains(line, "-->"): // timestamp range: capture start
			if before, _, ok := strings.Cut(line, "-->"); ok {
				start = parseSRTTimestamp(strings.TrimSpace(before))
			}
			continue
		case tsRE.MatchString(line): // stray timestamp line
			continue
		}
		line = strings.TrimSpace(entityReplacer.Replace(tagRE.ReplaceAllString(line, "")))
		if line == "" || line == last {
			continue
		}
		out = append(out, Cue{Start: start, Text: line})
		last = line
	}
	return out
}

// SRTToText converts SRT/VTT-style subtitle text to a clean plain-text transcript
// by flattening SRTToCues: cue indices and timestamp lines are dropped, markup and
// entities stripped, and consecutive duplicate lines collapsed.
//
// Cues are joined with spaces into a single flowing paragraph rather than one
// line per cue: caption cue boundaries are timing artifacts, not sentence or
// paragraph structure, so preserving them as hard newlines would force the
// reader/renderer to wrap at each fragment instead of filling the available
// width. Callers that display the result should soft-wrap it to their width.
func SRTToText(srt string) string {
	cues := SRTToCues(srt)
	if len(cues) == 0 {
		return ""
	}
	texts := make([]string, len(cues))
	for i, c := range cues {
		texts[i] = c.Text
	}
	return strings.Join(texts, " ") + "\n"
}

// parseSRTTimestamp converts an "HH:MM:SS,mmm" (or VTT "MM:SS.mmm") timestamp to a
// second offset. Malformed input yields 0.
func parseSRTTimestamp(s string) float64 {
	s = strings.ReplaceAll(s, ",", ".")
	parts := strings.Split(s, ":")
	var h, m float64
	var sec string
	switch len(parts) {
	case 3:
		h = atof(parts[0])
		m = atof(parts[1])
		sec = parts[2]
	case 2:
		m = atof(parts[0])
		sec = parts[1]
	default:
		return 0
	}
	return h*3600 + m*60 + atof(sec)
}

// atof parses a float, returning 0 on error (timestamps are best-effort).
func atof(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f
}
