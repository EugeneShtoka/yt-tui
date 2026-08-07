package transcripts

import (
	"testing"
)

func TestSRTToTextStripsAndDedups(t *testing.T) {
	// Mimics yt-dlp auto-caption SRT: cue indices, timestamps, markup, and the
	// rolling duplication where each cue repeats the previous line.
	srt := "1\n" +
		"00:00:01,000 --> 00:00:03,000\n" +
		"<c>hello world</c>\n" +
		"\n" +
		"2\n" +
		"00:00:03,000 --> 00:00:05,000\n" +
		"hello world\n" +
		"this &amp; that\n" +
		"\n" +
		"3\n" +
		"00:00:05,000 --> 00:00:07,000\n" +
		"this &amp; that\n"

	got := SRTToText(srt)
	// Cues are joined into a single flowing line (space-separated), not one line
	// per cue, so a renderer can soft-wrap to its own width.
	want := "hello world this & that\n"
	if got != want {
		t.Fatalf("SRTToText:\n got %q\nwant %q", got, want)
	}
}

func TestSRTToTextEmpty(t *testing.T) {
	if got := SRTToText("1\n00:00:01,000 --> 00:00:02,000\n"); got != "" {
		t.Fatalf("expected empty for a text-less cue, got %q", got)
	}
}

func TestSRTToCuesCapturesStartTimes(t *testing.T) {
	srt := "1\n" +
		"00:00:01,000 --> 00:00:03,000\n" +
		"<c>hello world</c>\n" +
		"\n" +
		"2\n" +
		"01:02:05,000 --> 01:02:07,000\n" + // 3725s, hours + entity
		"hello world\n" + // duplicate of previous cue → dropped
		"this &amp; that\n"

	got := SRTToCues(srt)
	want := []Cue{
		{Start: 1, Text: "hello world"},
		{Start: 3725, Text: "this & that"},
	}
	if len(got) != len(want) {
		t.Fatalf("SRTToCues len = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cue %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
