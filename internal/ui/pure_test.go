package ui

import (
	"strings"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

// mergeVideos tests
func TestMergeVideosEmpty(t *testing.T) {
	result := mergeVideos(nil, nil)
	if len(result) != 0 {
		t.Errorf("merging empty slices: got len=%d, want 0", len(result))
	}
}

func TestMergeVideosNoConflict(t *testing.T) {
	existing := []youtube.Video{{ID: "a"}, {ID: "b"}}
	incoming := []youtube.Video{{ID: "c"}, {ID: "d"}}
	result := mergeVideos(existing, incoming)
	if len(result) != 4 {
		t.Errorf("no conflict merge: got len=%d, want 4", len(result))
	}
}

func TestMergeVideosIncomingWins(t *testing.T) {
	existing := []youtube.Video{{ID: "a", Title: "old"}}
	incoming := []youtube.Video{{ID: "a", Title: "new"}}
	result := mergeVideos(existing, incoming)
	if len(result) != 1 {
		t.Errorf("merge conflict: got len=%d, want 1", len(result))
	}
	// Find the merged video by ID
	for _, v := range result {
		if v.ID == "a" && v.Title != "new" {
			t.Errorf("merge conflict: incoming didn't win, title=%s", v.Title)
		}
	}
}

// filterSubscribed tests
func TestFilterSubscribedEmpty(t *testing.T) {
	videos := []youtube.Video{{ID: "a", ChannelID: "ch1"}}
	subscribed := make(map[string]bool)
	result := filterSubscribed(videos, subscribed)
	if len(result) != len(videos) {
		t.Errorf("empty subscribed map: got len=%d, want %d", len(result), len(videos))
	}
}

func TestFilterSubscribedRemoves(t *testing.T) {
	videos := []youtube.Video{
		{ID: "a", ChannelID: "ch1"},
		{ID: "b", ChannelID: "ch2"},
	}
	subscribed := map[string]bool{"ch1": true}
	result := filterSubscribed(videos, subscribed)
	if len(result) != 1 || result[0].ID != "b" {
		t.Errorf("filter subscribed: got %d items, want 1 with ID='b'", len(result))
	}
}

func TestFilterSubscribedByName(t *testing.T) {
	videos := []youtube.Video{
		{ID: "a", Channel: "MyChannel"},
	}
	subscribed := map[string]bool{"name:mychannel": true}
	result := filterSubscribed(videos, subscribed)
	if len(result) != 0 {
		t.Errorf("filter by name: should filter case-insensitive, got len=%d", len(result))
	}
}

// removeVideoByID tests
func TestRemoveVideoByIDEmpty(t *testing.T) {
	result := removeVideoByID(nil, "any")
	if len(result) != 0 {
		t.Errorf("remove from nil: got len=%d, want 0", len(result))
	}
}

func TestRemoveVideoByIDNotFound(t *testing.T) {
	videos := []youtube.Video{{ID: "a"}, {ID: "b"}}
	result := removeVideoByID(videos, "c")
	if len(result) != 2 {
		t.Errorf("remove not found: got len=%d, want 2", len(result))
	}
}

func TestRemoveVideoByIDFound(t *testing.T) {
	videos := []youtube.Video{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	result := removeVideoByID(videos, "b")
	if len(result) != 2 {
		t.Errorf("remove found: got len=%d, want 2", len(result))
	}
	for _, v := range result {
		if v.ID == "b" {
			t.Errorf("remove found: 'b' was not removed")
		}
	}
}

// extractLinks tests
func TestExtractLinksNone(t *testing.T) {
	result := extractLinks("no links here")
	if len(result) != 0 {
		t.Errorf("no links: got len=%d, want 0", len(result))
	}
}

func TestExtractLinksBasic(t *testing.T) {
	desc := "check out https://example.com"
	result := extractLinks(desc)
	if len(result) != 1 {
		t.Errorf("basic link: got len=%d, want 1", len(result))
	}
	if len(result) > 0 && !strings.Contains(result[0].URL, "example.com") {
		t.Errorf("basic link: URL=%s, want example.com", result[0].URL)
	}
}

func TestExtractLinksDedupe(t *testing.T) {
	desc := "https://example.com and https://example.com again"
	result := extractLinks(desc)
	if len(result) != 1 {
		t.Errorf("dedupe links: got len=%d, want 1", len(result))
	}
}

func TestExtractLinksTrimPunctuation(t *testing.T) {
	desc := "visit https://example.com."
	result := extractLinks(desc)
	if len(result) > 0 && strings.HasSuffix(result[0].URL, ".") {
		t.Errorf("trim punctuation: URL ends with '.': %s", result[0].URL)
	}
}

// cmdCompletionsFor tests
func TestCmdCompletionsForEmpty(t *testing.T) {
	result := cmdCompletionsFor("")
	if len(result) == 0 {
		t.Errorf("empty prefix: got no completions")
	}
}

func TestCmdCompletionsForFirstWord(t *testing.T) {
	result := cmdCompletionsFor("t")
	// Should return commands starting with "t"
	found := false
	for _, c := range result {
		if strings.HasPrefix(c, "t") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("first word 't': no completions start with 't'")
	}
}

func TestCmdCompletionsForSecondWord(t *testing.T) {
	result := cmdCompletionsFor("tab ")
	// Should return second-word completions for "tab"
	for _, c := range result {
		if !strings.HasPrefix(c, "tab ") {
			t.Errorf("second word 'tab ': got completion not starting with 'tab ': %s", c)
		}
	}
}

// filterByAge tests
func TestFilterByAgeZeroOrNegative(t *testing.T) {
	videos := []youtube.Video{{ID: "a", UploadDate: "20200101"}}
	result := filterByAge(videos, 0)
	if len(result) != len(videos) {
		t.Errorf("zero/negative maxDays: got len=%d, want %d", len(result), len(videos))
	}
}

func TestFilterByAgeNoDate(t *testing.T) {
	videos := []youtube.Video{{ID: "a", UploadDate: ""}}
	result := filterByAge(videos, 7)
	if len(result) != 1 {
		t.Errorf("no date kept: got len=%d, want 1", len(result))
	}
}

func TestFilterByAgeMalformedDate(t *testing.T) {
	videos := []youtube.Video{{ID: "a", UploadDate: "invalid"}}
	result := filterByAge(videos, 7)
	if len(result) != 1 {
		t.Errorf("malformed date kept: got len=%d, want 1", len(result))
	}
}

// vsMove boundary cases
func TestVsMoveNZero(t *testing.T) {
	c, vs := vsMove(0, 0, 0, 1, 10, false)
	if c != 0 || vs != 0 {
		t.Errorf("vsMove n=0: got (%d,%d), want (0,0)", c, vs)
	}
}

func TestVsMoveClampHigh(t *testing.T) {
	c, _ := vsMove(8, 0, 10, 5, 10, false)
	if c != 9 {
		t.Errorf("vsMove clamp high: got %d, want 9", c)
	}
}

func TestVsMoveClampLow(t *testing.T) {
	c, _ := vsMove(1, 0, 10, -5, 10, false)
	if c != 0 {
		t.Errorf("vsMove clamp low: got %d, want 0", c)
	}
}

func TestVsMoveCircularWrapForward(t *testing.T) {
	c, _ := vsMove(9, 0, 10, 1, 10, true)
	if c != 0 {
		t.Errorf("vsMove circular wrap forward: got %d, want 0", c)
	}
}

func TestVsMoveCircularWrapBackward(t *testing.T) {
	c, _ := vsMove(0, 0, 10, -1, 10, true)
	if c != 9 {
		t.Errorf("vsMove circular wrap backward: got %d, want 9", c)
	}
}

func TestVsMoveViewportInvariantDown(t *testing.T) {
	// cursor at viewport bottom edge moves down — vs must shift to keep cursor visible
	height := 5
	c, vs := vsMove(4, 0, 20, 1, height, false)
	if c < vs || c >= vs+height {
		t.Errorf("vsMove viewport invariant (down): cursor=%d not in [%d,%d)", c, vs, vs+height)
	}
	if vs != 1 {
		t.Errorf("vsMove viewport invariant (down): vs=%d, want 1", vs)
	}
}

func TestVsMoveViewportInvariantUp(t *testing.T) {
	// cursor at vs, moves up one — vs must shift down to keep cursor visible
	height := 5
	c, vs := vsMove(5, 5, 20, -1, height, false)
	if c < vs || c >= vs+height {
		t.Errorf("vsMove viewport invariant (up): cursor=%d not in [%d,%d)", c, vs, vs+height)
	}
	if vs != 4 {
		t.Errorf("vsMove viewport invariant (up): vs=%d, want 4", vs)
	}
}

// vsPage boundary cases
func TestVsPageUpAtStart(t *testing.T) {
	c, vs := vsPage(0, 0, 20, -1, 5, false)
	if c != 0 || vs != 0 {
		t.Errorf("vsPage up at start: got (%d,%d), want (0,0)", c, vs)
	}
}

func TestVsPageDownAtEnd(t *testing.T) {
	// at last page, page down should not advance past last item
	c, vs := vsPage(15, 15, 20, 1, 5, false)
	if c >= 20 {
		t.Errorf("vsPage down at end: cursor=%d >= n=20", c)
	}
	if vs != 15 {
		t.Errorf("vsPage down at end: vs=%d, want 15", vs)
	}
}

func TestVsPageCircularWrapDown(t *testing.T) {
	// page down from last page wraps to first page (circular)
	_, vs := vsPage(16, 15, 20, 1, 5, true)
	if vs != 0 {
		t.Errorf("vsPage circular wrap down: vs=%d, want 0", vs)
	}
}

func TestVsPageCircularWrapUp(t *testing.T) {
	// page up from first page wraps to last page (circular)
	_, vs := vsPage(0, 0, 20, -1, 5, true)
	if vs != 15 {
		t.Errorf("vsPage circular wrap up: vs=%d, want 15", vs)
	}
}

// vsJump boundary cases
func TestVsJumpNegativeTarget(t *testing.T) {
	c, vs := vsJump(-5, 20, 5)
	if c != 0 || vs != 0 {
		t.Errorf("vsJump negative target: got (%d,%d), want (0,0)", c, vs)
	}
}

func TestVsJumpPastEnd(t *testing.T) {
	c, vs := vsJump(25, 20, 5)
	if c != 19 {
		t.Errorf("vsJump past end: cursor=%d, want 19", c)
	}
	if vs != 15 {
		t.Errorf("vsJump past end: vs=%d, want 15", vs)
	}
}

func TestVsJumpCenterInMiddle(t *testing.T) {
	// target in middle of large list — vs should center around it
	c, vs := vsJump(10, 20, 5)
	if c != 10 {
		t.Errorf("vsJump center: cursor=%d, want 10", c)
	}
	if vs != 8 {
		t.Errorf("vsJump center: vs=%d, want 8 (height/2=2 above target)", vs)
	}
}

func TestVsJumpNearTop(t *testing.T) {
	c, vs := vsJump(1, 20, 5)
	if c != 1 {
		t.Errorf("vsJump near top: cursor=%d, want 1", c)
	}
	if vs != 0 {
		t.Errorf("vsJump near top: vs=%d, want 0", vs)
	}
}

func TestVsJumpNearBottom(t *testing.T) {
	c, vs := vsJump(18, 20, 5)
	if c != 18 {
		t.Errorf("vsJump near bottom: cursor=%d, want 18", c)
	}
	if vs != 15 {
		t.Errorf("vsJump near bottom: vs=%d, want 15", vs)
	}
}

// SponsorBlock round-trip tests
func TestSBRoundTripNoSegments(t *testing.T) {
	for _, x := range []int64{0, 5000, 60000} {
		adj := originalToAdjustedMs(x, nil)
		got := adjustedToOriginalMs(adj, nil)
		if got != x {
			t.Errorf("SB round-trip (no segs) x=%d: got %d", x, got)
		}
	}
}

func TestSBRoundTripBeforeSegment(t *testing.T) {
	segs := []db.SBSegment{{Start: 10.0, End: 20.0}}
	x := int64(5000) // 5s, before segment
	adj := originalToAdjustedMs(x, segs)
	got := adjustedToOriginalMs(adj, segs)
	if got != x {
		t.Errorf("SB round-trip (before seg) x=%d: adj=%d got=%d", x, adj, got)
	}
}

func TestSBRoundTripAfterSegment(t *testing.T) {
	segs := []db.SBSegment{{Start: 10.0, End: 20.0}}
	x := int64(25000) // 25s, after segment
	adj := originalToAdjustedMs(x, segs)
	got := adjustedToOriginalMs(adj, segs)
	if got != x {
		t.Errorf("SB round-trip (after seg) x=%d: adj=%d got=%d", x, adj, got)
	}
}

func TestSBRoundTripMultipleSegments(t *testing.T) {
	segs := []db.SBSegment{
		{Start: 5.0, End: 10.0},
		{Start: 30.0, End: 45.0},
	}
	// points before, between, and after both segments
	for _, x := range []int64{2000, 15000, 50000} {
		adj := originalToAdjustedMs(x, segs)
		got := adjustedToOriginalMs(adj, segs)
		if got != x {
			t.Errorf("SB round-trip (multi-seg) x=%d: adj=%d got=%d", x, adj, got)
		}
	}
}

func TestSBMonotonicityOutsideSegments(t *testing.T) {
	segs := []db.SBSegment{{Start: 10.0, End: 20.0}}
	// two points outside the segment — adjusted order must match original order
	x1, x2 := int64(5000), int64(25000)
	adj1 := originalToAdjustedMs(x1, segs)
	adj2 := originalToAdjustedMs(x2, segs)
	if adj1 >= adj2 {
		t.Errorf("SB monotonicity: adj1=%d >= adj2=%d for x1=%d < x2=%d", adj1, adj2, x1, x2)
	}
}

func TestSBInsideSegmentMapsToStart(t *testing.T) {
	// a position inside a cut segment maps to the segment's start in adjusted time
	segs := []db.SBSegment{{Start: 10.0, End: 20.0}}
	x := int64(15000) // inside [10s, 20s]
	adj := originalToAdjustedMs(x, segs)
	wantAdj := int64(10000) // segment start in adjusted ms
	if adj != wantAdj {
		t.Errorf("SB inside segment: adj=%d, want %d", adj, wantAdj)
	}
}
