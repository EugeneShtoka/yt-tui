package media

import (
	"regexp"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/db"
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
