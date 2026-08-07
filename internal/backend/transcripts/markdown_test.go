package transcripts

import (
	"strings"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

func TestBuildMarkdownFrontmatter(t *testing.T) {
	md := BuildMarkdown(MarkdownDoc{
		Title:        `He said "hi": part 1`,
		Channel:      "Some Channel",
		UploadDate:   "20260728",
		WatchURL:     "https://youtu.be/abc",
		ThumbnailURL: "https://i.ytimg.com/vi/abc/hqdefault.jpg",
		ImageRef:     "../thumbnails/abc.jpg",
		Transcript:   "hello world\n",
	})

	for _, want := range []string{
		`title: "He said \"hi\": part 1"`, // quotes escaped
		"channel: \"Some Channel\"",
		"upload_date: 2026-07-28", // 8-digit reformatted
		"url: https://youtu.be/abc",
		"thumbnail: https://i.ytimg.com/vi/abc/hqdefault.jpg",
		"![](../thumbnails/abc.jpg)",
		"## Transcript",
		"hello world",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q\n---\n%s", want, md)
		}
	}
}

// With both chapters and timed cues, the transcript is split into per-chapter
// sections headed by "## <timestamp> <title>", each carrying the cues that fall
// within it. There is no standalone "## Chapters" list any more.
func TestBuildMarkdownInterleavesChapters(t *testing.T) {
	md := BuildMarkdown(MarkdownDoc{
		Title:   "T",
		Channel: "C",
		Chapters: []domain.Chapter{
			{Title: "Intro", OriginalStart: 0},
			{Title: "Main", OriginalStart: 100},
		},
		Cues: []Cue{
			{Start: 0, Text: "alpha"},
			{Start: 50, Text: "beta"},
			{Start: 100, Text: "gamma"},
			{Start: 150, Text: "delta"},
		},
	})

	want := "## 0:00 Intro\n\nalpha beta\n\n## 1:40 Main\n\ngamma delta\n"
	if !strings.HasSuffix(md, want) {
		t.Errorf("interleaved body wrong\n got:\n%s\n want suffix:\n%s", md, want)
	}
	if strings.Contains(md, "## Chapters") || strings.Contains(md, "## Transcript") {
		t.Errorf("interleaved note should not carry a Chapters list or flat Transcript header:\n%s", md)
	}
}

// A chapter boundary that lands mid-sentence is snapped back to the start of that
// sentence, so the header never interrupts a sentence. Here chapter "Topic" is
// timestamped at 10s, which falls inside the sentence that began at 6s; the whole
// sentence moves under "Topic" and "Intro" ends at the prior sentence.
func TestBuildMarkdownSnapsHeaderToSentenceStart(t *testing.T) {
	md := BuildMarkdown(MarkdownDoc{
		Title:   "T",
		Channel: "C",
		Chapters: []domain.Chapter{
			{Title: "Intro", OriginalStart: 0},
			{Title: "Topic", OriginalStart: 10},
		},
		Cues: []Cue{
			{Start: 0, Text: "First chapter intro."},
			{Start: 6, Text: "A sentence that begins here"},
			{Start: 9, Text: "and keeps going"},
			{Start: 11, Text: "until it ends."}, // raw boundary (>=10) lands here, mid-sentence
			{Start: 15, Text: "Next sentence."},
		},
	})
	want := "## 0:00 Intro\n\nFirst chapter intro.\n\n" +
		"## 0:10 Topic\n\nA sentence that begins here and keeps going until it ends. Next sentence.\n"
	if !strings.HasSuffix(md, want) {
		t.Errorf("header not snapped to sentence start\n got:\n%s\n want suffix:\n%s", md, want)
	}
}

// Cues before the first chapter's timestamp fold into the first chapter section.
func TestBuildMarkdownFoldsPreChapterText(t *testing.T) {
	md := BuildMarkdown(MarkdownDoc{
		Title:    "T",
		Channel:  "C",
		Chapters: []domain.Chapter{{Title: "First", OriginalStart: 10}},
		Cues: []Cue{
			{Start: 0, Text: "pre"},
			{Start: 20, Text: "post"},
		},
	})
	if !strings.Contains(md, "## 0:10 First\n\npre post\n") {
		t.Errorf("pre-chapter text not folded into first chapter:\n%s", md)
	}
}

// Chapters present but no cues → flat transcript fallback (no interleaving).
func TestBuildMarkdownFallsBackWithoutCues(t *testing.T) {
	md := BuildMarkdown(MarkdownDoc{
		Title:      "T",
		Channel:    "C",
		Chapters:   []domain.Chapter{{Title: "Intro", OriginalStart: 0}},
		Transcript: "hello world",
	})
	if !strings.Contains(md, "## Transcript\n\nhello world") {
		t.Errorf("expected flat transcript fallback:\n%s", md)
	}
	if strings.Contains(md, "## 0:00 Intro") {
		t.Errorf("should not emit chapter headers without cues:\n%s", md)
	}
}

// A chapter that no cue falls under is dropped from the note rather than left as
// an empty header.
func TestBuildMarkdownSkipsEmptyChapters(t *testing.T) {
	md := BuildMarkdown(MarkdownDoc{
		Title:   "T",
		Channel: "C",
		Chapters: []domain.Chapter{
			{Title: "Intro", OriginalStart: 0},
			{Title: "Silent Gap", OriginalStart: 100},
			{Title: "Outro", OriginalStart: 200},
		},
		Cues: []Cue{
			{Start: 0, Text: "hello"},
			// nothing between 100 and 200 → "Silent Gap" is empty
			{Start: 200, Text: "goodbye"},
		},
	})
	if strings.Contains(md, "Silent Gap") {
		t.Errorf("empty chapter should be dropped:\n%s", md)
	}
	if !strings.Contains(md, "## 0:00 Intro") || !strings.Contains(md, "## 3:20 Outro") {
		t.Errorf("non-empty chapters should survive:\n%s", md)
	}
}

func TestDropSBSegments(t *testing.T) {
	cues := []Cue{
		{Start: 0, Text: "intro"},
		{Start: 120, Text: "this video is sponsored by"}, // inside [119.8,193.4]
		{Start: 150, Text: "buy now"},                    // inside
		{Start: 200, Text: "back to content"},            // after segment
		{Start: 650, Text: "another sponsor"},            // inside [642,715]
		{Start: 720, Text: "outro"},                      // after
	}
	segs := []domain.SBSegment{{Start: 119.8, End: 193.4}, {Start: 642.4, End: 715.5}}
	got := DropSBSegments(cues, segs)
	var texts []string
	for _, c := range got {
		texts = append(texts, c.Text)
	}
	want := []string{"intro", "back to content", "outro"}
	if strings.Join(texts, "|") != strings.Join(want, "|") {
		t.Errorf("DropSBSegments = %v, want %v", texts, want)
	}
	// No segments → unchanged.
	if len(DropSBSegments(cues, nil)) != len(cues) {
		t.Error("no segments should leave cues unchanged")
	}
}

// When captions are punctuated, an SB cut snaps to sentence boundaries: it pulls
// back to the end of the last complete sentence (dropping the sentence the sponsor
// interrupted) and extends forward through the end of the sentence the segment end
// lands in.
func TestDropSBSegmentsSnapsToSentences(t *testing.T) {
	cues := []Cue{
		{Start: 0, Text: "Intro sentence one."},
		{Start: 5, Text: "A long second sentence that keeps going"}, // interrupted by the sponsor
		{Start: 10, Text: "and mentions the sponsor"},               // segment start lands here
		{Start: 13, Text: "briefly"},
		{Start: 16, Text: "before wrapping up the sponsor read."}, // segment end's sentence finishes here
		{Start: 20, Text: "Now genuinely back to content."},
	}
	segs := []domain.SBSegment{{Start: 10, End: 14}}
	var texts []string
	for _, c := range DropSBSegments(cues, segs) {
		texts = append(texts, c.Text)
	}
	want := []string{"Intro sentence one.", "Now genuinely back to content."}
	if strings.Join(texts, "|") != strings.Join(want, "|") {
		t.Errorf("sentence-snapped SB cut = %v, want %v", texts, want)
	}
}

func TestBuildMarkdownOmitsEmptyImageAndChapters(t *testing.T) {
	md := BuildMarkdown(MarkdownDoc{Title: "T", Channel: "C", Transcript: "body"})
	if strings.Contains(md, "![](") {
		t.Error("expected no image embed when ImageRef is empty")
	}
	if strings.Contains(md, "## Chapters") {
		t.Error("expected no Chapters section when there are none")
	}
	if strings.Contains(md, "upload_date:") {
		t.Error("expected no upload_date when unset")
	}
}

func TestStripForDisplay(t *testing.T) {
	md := BuildMarkdown(MarkdownDoc{
		Title:    "T",
		Channel:  "C",
		ImageRef: "../thumbnails/abc.jpg",
		Chapters: []domain.Chapter{{Title: "Intro", OriginalStart: 0}},
		Cues:     []Cue{{Start: 0, Text: "the words"}},
	})
	got := stripForDisplay(md)

	if strings.Contains(got, "---") || strings.Contains(got, "title:") {
		t.Errorf("frontmatter not stripped:\n%s", got)
	}
	if strings.Contains(got, "![](") {
		t.Errorf("image embed not stripped:\n%s", got)
	}
	if !strings.Contains(got, "## 0:00 Intro") || !strings.Contains(got, "the words") {
		t.Errorf("chapter header/transcript should survive:\n%s", got)
	}
	if strings.HasPrefix(got, "\n") {
		t.Errorf("leading blank lines not trimmed: %q", got)
	}
}
