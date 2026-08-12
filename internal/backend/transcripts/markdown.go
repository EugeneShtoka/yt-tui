package transcripts

import (
	"fmt"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/media"
)

// MarkdownDoc is the data BuildMarkdown renders into a single note. It is
// assembled by the caller from the unified yt-dlp fetch (metadata + chapters +
// thumbnail URL) plus the transcript text converted from the sidecar .srt.
type MarkdownDoc struct {
	Title        string           // video title
	Channel      string           // channel name
	UploadDate   string           // yt-dlp's 8-digit YYYYMMDD; rendered as YYYY-MM-DD
	WatchURL     string           // canonical watch URL
	ThumbnailURL string           // remote CDN URL — always recorded, the durable fallback
	ImageRef     string           // relative path to the local cached image; "" omits the embed
	Chapters     []domain.Chapter // original-timeline chapters; empty omits the section
	Cues         []Cue            // timed transcript cues; when present with Chapters, the transcript is split into per-chapter sections
	Transcript   string           // flat transcript text; fallback used when Cues/Chapters can't be interleaved
}

// BuildMarkdown renders a MarkdownDoc as an Obsidian-friendly note: YAML
// frontmatter (with the remote thumbnail URL always present so a GC'd local
// image is still recoverable), an optional local image embed, and the transcript
// body. When chapters and timed cues are both present the transcript is split
// into per-chapter sections, each introduced by a `## <timestamp> <title>`
// header; otherwise it is one flat `## Transcript` block. The frontmatter/image
// ordering is what stripForDisplay relies on when serving the note to the viewer.
func BuildMarkdown(d MarkdownDoc) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: " + yamlString(d.Title) + "\n")
	b.WriteString("channel: " + yamlString(d.Channel) + "\n")
	if date := formatUploadDate(d.UploadDate); date != "" {
		b.WriteString("upload_date: " + date + "\n")
	}
	if d.WatchURL != "" {
		b.WriteString("url: " + d.WatchURL + "\n")
	}
	if d.ThumbnailURL != "" {
		b.WriteString("thumbnail: " + d.ThumbnailURL + "\n")
	}
	b.WriteString("---\n\n")

	if d.ImageRef != "" {
		b.WriteString("![](" + d.ImageRef + ")\n\n")
	}

	if secs := splitByChapters(d.Cues, d.Chapters); len(secs) > 0 {
		for _, s := range secs {
			fmt.Fprintf(&b, "## %s %s\n\n", formatTimestamp(s.Start), s.Title)
			b.WriteString(s.Text)
			b.WriteString("\n\n")
		}
		return strings.TrimRight(b.String(), "\n") + "\n"
	}

	b.WriteString("## Transcript\n\n")
	b.WriteString(strings.TrimRight(transcriptText(d), "\n"))
	b.WriteString("\n")
	return b.String()
}

// BuildAndWriteNote assembles the canonical markdown note for a freshly-fetched
// video — from its metadata plus the sidecar .srt the unified yt-dlp fetch just
// wrote — and writes it. imagePath is the local cached thumbnail path ("" if
// none); it is embedded as a relative image ref. subtitleLangs is the caller's
// language preference, in priority order.
//
// It returns (false, nil) — building nothing, not an error — when markdown is
// disabled or the video has no captions, so metadata-only videos never produce
// an empty note; (false, err) when the write fails; and (true, nil) on success.
// It does not perform the yt-dlp fetch or touch the DB details cache: callers own
// those, so this is the single place that turns a fetched VideoDetails into a note
// (previously duplicated in enrich.writeNote and api.buildTranscriptNote — M-1).
func (s *Store) BuildAndWriteNote(videoID, watchURL string, d domain.VideoDetails, subtitleLangs []string, imagePath string) (bool, error) {
	if !s.MarkdownEnabled() || videoID == "" {
		return false, nil
	}
	cues, ok := s.SelectCues(videoID, d.Language, subtitleLangs)
	if !ok || len(cues) == 0 {
		return false, nil
	}
	cues = DropSBSegments(cues, d.SBSegments) // excise sponsor/etc. spans the player cuts
	chapters, _ := media.ProcessChapters(d.Chapters)
	var imageRef string
	if imagePath != "" {
		imageRef = s.RelImageRef(imagePath)
	}
	note := BuildMarkdown(MarkdownDoc{
		Title:        d.Title,
		Channel:      d.Channel,
		UploadDate:   d.UploadDate,
		WatchURL:     watchURL,
		ThumbnailURL: d.ThumbnailURL,
		ImageRef:     imageRef,
		Chapters:     chapters,
		Cues:         cues,
	})
	if err := s.WriteMarkdown(videoID, note); err != nil {
		return false, fmt.Errorf("BuildAndWriteNote %s: %w", videoID, err)
	}
	return true, nil
}

// DropSBSegments removes the cues covered by SponsorBlock segments, excising the
// sponsor/self-promo/etc. spans the player cuts under the user's settings. Each
// segment's cut is snapped to sentence boundaries (like the chapter headers): the
// start moves back to the end of the last complete sentence (dropping the sentence
// the sponsor interrupted) and the end moves forward through the end of the
// sentence the segment-end lands in. Unpunctuated captions fall back to the raw
// segment span. segs share the cues' (original) timeline.
func DropSBSegments(cues []Cue, segs []domain.SBSegment) []Cue {
	if len(segs) == 0 || len(cues) == 0 {
		return cues
	}
	remove := make([]bool, len(cues))
	for _, s := range segs {
		lo, hi := len(cues), len(cues)
		for i := range cues {
			if cues[i].Start >= s.Start {
				lo = i
				break
			}
		}
		for i := range cues {
			if cues[i].Start >= s.End {
				hi = i
				break
			}
		}
		if lo >= hi {
			continue // no cue starts inside this segment
		}
		lo = snapStartToSentence(cues, lo, 0) // back to the start of the interrupted sentence
		hi = snapEndToSentence(cues, hi)      // forward through the end of the landing sentence
		for i := lo; i < hi; i++ {
			remove[i] = true
		}
	}
	out := make([]Cue, 0, len(cues))
	for i, c := range cues {
		if !remove[i] {
			out = append(out, c)
		}
	}
	return out
}

// snapEndToSentence moves a cut's exclusive end-cue index forward to just past the
// cue that ends the sentence the raw end lands in, so the cut finishes on a
// sentence boundary. Returns raw unchanged when it already ends at a sentence
// break or no later break exists (unpunctuated captions).
func snapEndToSentence(cues []Cue, raw int) int {
	if raw <= 0 || raw >= len(cues) {
		return raw
	}
	if endsSentence(cues[raw-1].Text) {
		return raw // cut already ends between two sentences
	}
	for k := raw + 1; k <= len(cues); k++ {
		if endsSentence(cues[k-1].Text) {
			return k // sentence ends at cue k-1
		}
	}
	return raw // no later sentence break; keep the raw span
}

// section is a chapter's header data plus the transcript text that falls under it.
type section struct {
	Title string
	Start float64
	Text  string
}

// splitByChapters buckets timed cues into chapter sections. A cue belongs to a
// chapter by start time, but because caption cues are timing fragments a chapter
// boundary usually lands mid-sentence; each boundary is therefore snapped back to
// the start of the sentence it falls inside (see snapStartToSentence), so a
// chapter header never interrupts a sentence. Cues before the first chapter fold
// into it. Chapters with no transcript text are dropped. Returns nil when there
// is nothing to interleave (no chapters or no cues), signaling the flat fallback.
func splitByChapters(cues []Cue, chapters []domain.Chapter) []section {
	if len(chapters) == 0 || len(cues) == 0 {
		return nil
	}
	// starts[i] is the index of the first cue that opens chapter i. Chapter 0 opens
	// at cue 0 (absorbing any pre-chapter text). Each later boundary is the first
	// cue at/after the chapter's timestamp, snapped back to a sentence start.
	starts := make([]int, len(chapters))
	for i := 1; i < len(chapters); i++ {
		raw := len(cues)
		for j := starts[i-1]; j < len(cues); j++ {
			if cues[j].Start >= chapters[i].OriginalStart {
				raw = j
				break
			}
		}
		starts[i] = snapStartToSentence(cues, raw, starts[i-1])
	}
	secs := make([]section, 0, len(chapters))
	for i, ch := range chapters {
		end := len(cues)
		if i+1 < len(chapters) {
			end = starts[i+1]
		}
		var b strings.Builder
		for j := starts[i]; j < end; j++ {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(cues[j].Text)
		}
		if text := b.String(); strings.TrimSpace(text) != "" { // drop empty chapters
			secs = append(secs, section{Title: ch.Title, Start: ch.OriginalStart, Text: text})
		}
	}
	return secs
}

// snapStartToSentence moves a chapter's raw (time-based) start-cue index back to
// the beginning of the sentence that the raw boundary falls inside, so a header
// isn't dropped mid-sentence. floor is the previous chapter's start — the snap
// never crosses it. If the boundary already sits at a sentence break, or no
// sentence break exists back to floor (e.g. unpunctuated auto-captions), the raw
// index is kept.
func snapStartToSentence(cues []Cue, raw, floor int) int {
	if raw <= floor || raw >= len(cues) {
		return raw
	}
	if endsSentence(cues[raw-1].Text) {
		return raw // boundary already falls between two sentences
	}
	for k := raw - 1; k > floor; k-- {
		if endsSentence(cues[k-1].Text) {
			return k // sentence starts at cue k
		}
	}
	return raw // no sentence break found; keep the time-based split
}

// endsSentence reports whether a cue's text ends a sentence — the last
// non-space, non-closing-punctuation character is sentence-terminating. Trailing
// quotes/brackets (incl. the Russian »/”) are ignored so `…конец.»` still counts.
func endsSentence(s string) bool {
	s = strings.TrimRight(strings.TrimSpace(s), `"'»”)]`)
	return strings.HasSuffix(s, ".") || strings.HasSuffix(s, "!") ||
		strings.HasSuffix(s, "?") || strings.HasSuffix(s, "…")
}

// transcriptText returns the flat transcript for the non-interleaved fallback:
// the explicit Transcript field, or the cues joined with spaces.
func transcriptText(d MarkdownDoc) string {
	if d.Transcript != "" {
		return d.Transcript
	}
	texts := make([]string, len(d.Cues))
	for i, c := range d.Cues {
		texts[i] = c.Text
	}
	return strings.Join(texts, " ")
}

// yamlString quotes a scalar for a YAML value, escaping embedded quotes and
// backslashes so titles with colons, quotes or leading specials stay valid.
func yamlString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// formatUploadDate turns yt-dlp's 8-digit YYYYMMDD into YYYY-MM-DD; anything
// else (empty, "0", a malformed value) yields "" so the field is dropped.
func formatUploadDate(s string) string {
	if len(s) != 8 {
		return ""
	}
	return s[0:4] + "-" + s[4:6] + "-" + s[6:8]
}

// formatTimestamp renders a second offset as H:MM:SS (hours dropped when zero),
// matching how chapter marks read in a video player.
func formatTimestamp(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	total := int(seconds)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// stripForDisplay returns a note's body for the in-app transcript viewer: the
// YAML frontmatter and the image embed are removed (a TUI can't render an
// image), leaving the chapter-headed transcript sections as readable text.
func stripForDisplay(md string) string {
	// Drop a leading frontmatter block delimited by --- ... ---.
	if rest, ok := strings.CutPrefix(md, "---\n"); ok {
		if i := strings.Index(rest, "\n---\n"); i >= 0 {
			md = rest[i+len("\n---\n"):]
		}
	}
	var out []string
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		// Skip image embeds: markdown ![](...) and Obsidian ![[...]].
		if strings.HasPrefix(trimmed, "![](") || strings.HasPrefix(trimmed, "![[") {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimLeft(strings.Join(out, "\n"), "\n")
}
