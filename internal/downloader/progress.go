package downloader

import (
	"regexp"
	"strconv"
	"strings"
)

// progressParser turns yt-dlp stdout lines into structured progress updates and
// resolved destination paths. It holds the compiled regexes and is pure — no
// I/O, no shared state — so it is unit-testable in isolation from the download
// queue (H-4: separates stdout parsing from orchestration).
type progressParser struct {
	progressRe *regexp.Regexp
	destRe     *regexp.Regexp
	mergerRe   *regexp.Regexp
}

func newProgressParser() *progressParser {
	return &progressParser{
		progressRe: regexp.MustCompile(`\[download\]\s+(\d+\.?\d*)%\s+of\s+~?\S+\s+at\s+(\S+)\s+ETA\s+(\S+)`),
		destRe:     regexp.MustCompile(`\[download\] Destination: (.+)`),
		mergerRe:   regexp.MustCompile(`\[Merger\] Merging formats into "(.+)"`),
	}
}

// progressUpdate is one parsed progress line: percent complete plus the speed
// and ETA strings yt-dlp printed.
type progressUpdate struct {
	Progress float64
	Speed    string
	ETA      string
}

// parseLine classifies a single yt-dlp stdout line. It returns a non-nil
// *progressUpdate for a progress line, or a non-empty finalPath for a
// Destination/Merger line; a line that is neither yields (nil, ""). The Merger
// and Destination lines both name the output file, so either updates finalPath.
func (p *progressParser) parseLine(line string) (*progressUpdate, string) {
	if m := p.progressRe.FindStringSubmatch(line); len(m) == 4 {
		pct, _ := strconv.ParseFloat(m[1], 64)
		return &progressUpdate{Progress: pct, Speed: m[2], ETA: m[3]}, ""
	}
	if m := p.mergerRe.FindStringSubmatch(line); len(m) == 2 {
		return nil, strings.TrimSpace(m[1])
	}
	if m := p.destRe.FindStringSubmatch(line); len(m) == 2 {
		return nil, strings.TrimSpace(m[1])
	}
	return nil, ""
}
