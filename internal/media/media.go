// Package media holds pure, UI-free media-processing logic: SponsorBlock
// chapter/timeline math and description link extraction. Everything here is a
// pure function with no tea/ui dependency, so it is cheap to unit-test.
package media

import (
	"math"
	"regexp"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

var linkRe = regexp.MustCompile(`https?://[^\s\]>)"']+`)

// ExtractLinks parses URLs (with any leading label text) out of a video
// description, one entry per unique URL, trimming trailing punctuation and
// leading list/bullet decoration from the label.
func ExtractLinks(desc string) []db.Link {
	seen := make(map[string]bool)
	var out []db.Link
	for _, line := range strings.Split(desc, "\n") {
		for _, loc := range linkRe.FindAllStringIndex(line, -1) {
			url := strings.TrimRight(line[loc[0]:loc[1]], ".,;:!?)'\"")
			if seen[url] {
				continue
			}
			seen[url] = true
			label := strings.TrimRight(strings.TrimSpace(line[:loc[0]]), ":,;-–—•►▶→")
			label = strings.TrimSpace(label)
			out = append(out, db.Link{Label: label, URL: url})
		}
	}
	return out
}

// ProcessChapters splits raw yt-dlp chapters (which include [SponsorBlock] entries
// when --sponsorblock-chapters was passed) into display chapters and SB segments.
// Display chapters have their timecodes adjusted to reflect the local-file timeline
// (after SB cuts); chapters whose boundaries coincide with a SB segment (±3 s on
// both start AND end) are dropped entirely.
func ProcessChapters(all []youtube.Chapter) ([]db.Chapter, []db.SBSegment) {
	const tol = 3.0
	var sbChapters []youtube.Chapter
	var realChapters []youtube.Chapter
	for _, ch := range all {
		if strings.HasPrefix(ch.Title, "[SponsorBlock]") {
			sbChapters = append(sbChapters, ch)
		} else {
			realChapters = append(realChapters, ch)
		}
	}

	segs := make([]db.SBSegment, len(sbChapters))
	for i, ch := range sbChapters {
		segs[i] = db.SBSegment{Start: ch.StartTime, End: ch.EndTime}
	}

	var out []db.Chapter
	for _, ch := range realChapters {
		skip := false
		for _, sb := range segs {
			if math.Abs(ch.StartTime-sb.Start) <= tol && math.Abs(ch.EndTime-sb.End) <= tol {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		out = append(out, db.Chapter{
			Title:         ch.Title,
			OriginalStart: ch.StartTime,
			OriginalEnd:   ch.EndTime,
			AdjustedStart: OriginalToAdjustedSec(ch.StartTime, segs),
			AdjustedEnd:   OriginalToAdjustedSec(ch.EndTime, segs),
		})
	}
	return out, segs
}

// OriginalToAdjustedSec converts an original-timeline position (seconds) to the
// adjusted local-file position after SB segments have been removed.
func OriginalToAdjustedSec(origSec float64, segs []db.SBSegment) float64 {
	offset := 0.0
	for _, seg := range segs {
		if origSec < seg.Start {
			break
		}
		if origSec < seg.End {
			return seg.Start - offset
		}
		offset += seg.End - seg.Start
	}
	return origSec - offset
}

// OriginalToAdjustedMs converts an original-timeline position (ms) to the adjusted
// local-file position in ms.
func OriginalToAdjustedMs(origMs int64, segs []db.SBSegment) int64 {
	offset := int64(0)
	for _, seg := range segs {
		segStartMs := int64(seg.Start * 1000)
		segEndMs := int64(seg.End * 1000)
		if origMs < segStartMs {
			break
		}
		if origMs < segEndMs {
			return segStartMs - offset
		}
		offset += segEndMs - segStartMs
	}
	return origMs - offset
}

// AdjustedToOriginalMs converts a local-file position (ms) back to the original
// video timeline in ms, undoing the SB cuts.
func AdjustedToOriginalMs(adjMs int64, segs []db.SBSegment) int64 {
	offset := int64(0)
	for _, seg := range segs {
		segDur := int64((seg.End - seg.Start) * 1000)
		segStartAdj := int64(seg.Start*1000) - offset
		if adjMs < segStartAdj {
			break
		}
		offset += segDur
	}
	return adjMs + offset
}
