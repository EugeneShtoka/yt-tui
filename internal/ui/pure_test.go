package ui

import (
	"strings"
	"testing"

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

// TODO(refactor): Add vs* boundary cases (vsMove, vsPage, vsJump) — Sonnet to test:
// - Cursor and viewStart invariants: cursor in [0,n), vs in [0,n), cursor visible within [vs, vs+height)
// - n==0 edge case
// - Circular navigation wrap-around
// - vsJump centering behavior
// TODO(refactor): Add SponsorBlock round-trip test — Sonnet to verify:
// - adjustedToOriginalMs(originalToAdjustedMs(x, segs), segs) == x for x outside cut regions
// - Monotonicity assertions
