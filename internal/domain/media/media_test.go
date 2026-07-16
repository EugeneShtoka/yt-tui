package media

import (
	"strings"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ── ExtractLinks ──────────────────────────────────────────────────────────────

func TestExtractLinksNone(t *testing.T) {
	result := ExtractLinks("no links here")
	if len(result) != 0 {
		t.Errorf("no links: got len=%d, want 0", len(result))
	}
}

func TestExtractLinksBasic(t *testing.T) {
	desc := "check out https://example.com"
	result := ExtractLinks(desc)
	if len(result) != 1 {
		t.Errorf("basic link: got len=%d, want 1", len(result))
	}
	if len(result) > 0 && !strings.Contains(result[0].URL, "example.com") {
		t.Errorf("basic link: URL=%s, want example.com", result[0].URL)
	}
}

func TestExtractLinksDedupe(t *testing.T) {
	desc := "https://example.com and https://example.com again"
	result := ExtractLinks(desc)
	if len(result) != 1 {
		t.Errorf("dedupe links: got len=%d, want 1", len(result))
	}
}

func TestExtractLinksTrimPunctuation(t *testing.T) {
	desc := "visit https://example.com."
	result := ExtractLinks(desc)
	if len(result) > 0 && strings.HasSuffix(result[0].URL, ".") {
		t.Errorf("trim punctuation: URL ends with '.': %s", result[0].URL)
	}
}

func TestExtractLinksLabel(t *testing.T) {
	desc := "Website: https://example.com"
	result := ExtractLinks(desc)
	if len(result) != 1 {
		t.Fatalf("labeled link: got len=%d, want 1", len(result))
	}
	if result[0].Label != "Website" {
		t.Errorf("label = %q, want %q", result[0].Label, "Website")
	}
}

// ── SponsorBlock timeline math ────────────────────────────────────────────────

func TestSBRoundTripNoSegments(t *testing.T) {
	for _, x := range []int64{0, 5000, 60000} {
		adj := OriginalToAdjustedMs(x, nil)
		got := AdjustedToOriginalMs(adj, nil)
		if got != x {
			t.Errorf("SB round-trip (no segs) x=%d: got %d", x, got)
		}
	}
}

func TestSBRoundTripBeforeSegment(t *testing.T) {
	segs := []domain.SBSegment{{Start: 10.0, End: 20.0}}
	x := int64(5000) // 5s, before segment
	adj := OriginalToAdjustedMs(x, segs)
	got := AdjustedToOriginalMs(adj, segs)
	if got != x {
		t.Errorf("SB round-trip (before seg) x=%d: adj=%d got=%d", x, adj, got)
	}
}

func TestSBRoundTripAfterSegment(t *testing.T) {
	segs := []domain.SBSegment{{Start: 10.0, End: 20.0}}
	x := int64(25000) // 25s, after segment
	adj := OriginalToAdjustedMs(x, segs)
	got := AdjustedToOriginalMs(adj, segs)
	if got != x {
		t.Errorf("SB round-trip (after seg) x=%d: adj=%d got=%d", x, adj, got)
	}
}

func TestSBRoundTripMultipleSegments(t *testing.T) {
	segs := []domain.SBSegment{
		{Start: 5.0, End: 10.0},
		{Start: 30.0, End: 45.0},
	}
	// points before, between, and after both segments
	for _, x := range []int64{2000, 15000, 50000} {
		adj := OriginalToAdjustedMs(x, segs)
		got := AdjustedToOriginalMs(adj, segs)
		if got != x {
			t.Errorf("SB round-trip (multi-seg) x=%d: adj=%d got=%d", x, adj, got)
		}
	}
}

func TestSBMonotonicityOutsideSegments(t *testing.T) {
	segs := []domain.SBSegment{{Start: 10.0, End: 20.0}}
	// two points outside the segment — adjusted order must match original order
	x1, x2 := int64(5000), int64(25000)
	adj1 := OriginalToAdjustedMs(x1, segs)
	adj2 := OriginalToAdjustedMs(x2, segs)
	if adj1 >= adj2 {
		t.Errorf("SB monotonicity: adj1=%d >= adj2=%d for x1=%d < x2=%d", adj1, adj2, x1, x2)
	}
}

func TestSBInsideSegmentMapsToStart(t *testing.T) {
	// a position inside a cut segment maps to the segment's start in adjusted time
	segs := []domain.SBSegment{{Start: 10.0, End: 20.0}}
	x := int64(15000) // inside [10s, 20s]
	adj := OriginalToAdjustedMs(x, segs)
	wantAdj := int64(10000) // segment start in adjusted ms
	if adj != wantAdj {
		t.Errorf("SB inside segment: adj=%d, want %d", adj, wantAdj)
	}
}

// ── Edge cases (plan item #2 verification) ────────────────────────────────────

func TestSBSegmentAtZero(t *testing.T) {
	// segment starting at t=0: everything shifts left by its duration.
	segs := []domain.SBSegment{{Start: 0.0, End: 5.0}}
	if adj := OriginalToAdjustedMs(0, segs); adj != 0 {
		t.Errorf("t=0 inside seg starting at 0: adj=%d, want 0", adj)
	}
	x := int64(8000) // after the 5s intro
	adj := OriginalToAdjustedMs(x, segs)
	if adj != 3000 {
		t.Errorf("after seg at 0: adj=%d, want 3000", adj)
	}
	if got := AdjustedToOriginalMs(adj, segs); got != x {
		t.Errorf("round-trip after seg at 0: got %d, want %d", got, x)
	}
}

func TestSBZeroLengthSegment(t *testing.T) {
	// a zero-length segment must not shift the timeline at all.
	segs := []domain.SBSegment{{Start: 10.0, End: 10.0}}
	for _, x := range []int64{5000, 10000, 20000} {
		if adj := OriginalToAdjustedMs(x, segs); adj != x {
			t.Errorf("zero-length seg x=%d: adj=%d, want %d", x, adj, x)
		}
	}
}

func TestOriginalToAdjustedSecMatchesMs(t *testing.T) {
	// the seconds and ms variants must agree at second boundaries.
	segs := []domain.SBSegment{{Start: 5.0, End: 10.0}}
	for _, sec := range []float64{2, 12, 30} {
		gotSec := OriginalToAdjustedSec(sec, segs)
		gotMs := OriginalToAdjustedMs(int64(sec*1000), segs)
		if int64(gotSec*1000) != gotMs {
			t.Errorf("sec/ms mismatch at %.0fs: sec=%.3f (%d ms) vs ms=%d", sec, gotSec, int64(gotSec*1000), gotMs)
		}
	}
}

// ── ProcessChapters ───────────────────────────────────────────────────────────

func TestProcessChaptersSplitsAndAdjusts(t *testing.T) {
	all := []domain.RawChapter{
		{Title: "Intro", StartTime: 0, EndTime: 10},
		{Title: "[SponsorBlock]: Sponsor", StartTime: 10, EndTime: 20},
		{Title: "Content", StartTime: 20, EndTime: 40},
	}
	chapters, segs := ProcessChapters(all)
	if len(segs) != 1 || segs[0].Start != 10 || segs[0].End != 20 {
		t.Fatalf("segs = %+v, want one [10,20]", segs)
	}
	if len(chapters) != 2 {
		t.Fatalf("chapters = %d, want 2 (Intro, Content)", len(chapters))
	}
	// Content starts at original 20 but adjusted 10 (10s of sponsor removed).
	content := chapters[1]
	if content.Title != "Content" || content.OriginalStart != 20 || content.AdjustedStart != 10 {
		t.Errorf("Content chapter = %+v, want adjustedStart=10", content)
	}
}

func TestProcessChaptersDropsChapterCoincidingWithSegment(t *testing.T) {
	// a real chapter whose bounds match a SB segment (±3s) is dropped.
	all := []domain.RawChapter{
		{Title: "Sponsor spot", StartTime: 10, EndTime: 20},
		{Title: "[SponsorBlock]: Sponsor", StartTime: 11, EndTime: 19},
	}
	chapters, _ := ProcessChapters(all)
	for _, ch := range chapters {
		if ch.Title == "Sponsor spot" {
			t.Errorf("chapter coinciding with SB segment was not dropped: %+v", ch)
		}
	}
}
