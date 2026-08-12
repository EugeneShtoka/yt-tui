package feed

import (
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ids joins the IDs of a video slice for compact assertions.
func ids(vs []domain.Video) string {
	out := ""
	for i, v := range vs {
		if i > 0 {
			out += " "
		}
		out += v.ID
	}
	return out
}

// ── FilterRecommended ───────────────────────────────────────────────────────

func TestFilterRecommendedAllDisabled(t *testing.T) {
	videos := []domain.Video{{ID: "a", Duration: 1, ViewCount: 1, UploadDate: "20200101"}}
	if got := ids(FilterRecommended(videos, 0, 0, 0)); got != "a" {
		t.Errorf("all thresholds <=0 returns input unchanged: got %q", got)
	}
}

func TestFilterRecommendedMinDuration(t *testing.T) {
	videos := []domain.Video{
		{ID: "short", Duration: 30},
		{ID: "long", Duration: 600},
		{ID: "unknown", Duration: 0}, // unknown duration kept
	}
	if got := ids(FilterRecommended(videos, 0, 60, 0)); got != "long unknown" {
		t.Errorf("minSecs=60: got %q, want 'long unknown'", got)
	}
}

func TestFilterRecommendedMinViews(t *testing.T) {
	videos := []domain.Video{
		{ID: "few", ViewCount: 50},
		{ID: "many", ViewCount: 5000},
		{ID: "unknown", ViewCount: 0}, // unknown views kept
	}
	if got := ids(FilterRecommended(videos, 0, 0, 100)); got != "many unknown" {
		t.Errorf("minViews=100: got %q, want 'many unknown'", got)
	}
}

func TestFilterRecommendedAge(t *testing.T) {
	videos := []domain.Video{
		{ID: "old", UploadDate: "20200101"}, // well before cutoff → dropped
		{ID: "nodate", UploadDate: ""},      // unknown date kept
		{ID: "malformed", UploadDate: "invalid"},
	}
	if got := ids(FilterRecommended(videos, 7, 0, 0)); got != "nodate malformed" {
		t.Errorf("maxDays=7: got %q, want 'nodate malformed' (old dropped, unknown dates kept)", got)
	}
}

// All three thresholds enabled at once — the single pass must drop a video that
// fails any one of them and keep only the one that satisfies all.
func TestFilterRecommendedCombinedSinglePass(t *testing.T) {
	today := time.Now().Format("20060102")
	videos := []domain.Video{
		{ID: "keep", Duration: 600, ViewCount: 5000, UploadDate: today},
		{ID: "short", Duration: 30, ViewCount: 5000, UploadDate: today},
		{ID: "lowviews", Duration: 600, ViewCount: 5, UploadDate: today},
		{ID: "old", Duration: 600, ViewCount: 5000, UploadDate: "20200101"},
	}
	if got := ids(FilterRecommended(videos, 7, 60, 100)); got != "keep" {
		t.Errorf("combined age=7 dur=60 views=100: got %q, want 'keep'", got)
	}
}

// ── FilterDownloaded / FilterHidden ───────────────────────────────────────────

func TestFilterDownloaded(t *testing.T) {
	videos := []domain.Video{{ID: "a"}, {ID: "b"}}
	local := map[string]domain.LocalVideo{"a": {ID: "a"}}
	if got := ids(FilterDownloaded(videos, local)); got != "b" {
		t.Errorf("FilterDownloaded: got %q, want 'b'", got)
	}
}

func TestFilterHidden(t *testing.T) {
	videos := []domain.Video{{ID: "a"}, {ID: "b"}}
	hidden := map[string]bool{"b": true}
	if got := ids(FilterHidden(videos, hidden)); got != "a" {
		t.Errorf("FilterHidden: got %q, want 'a'", got)
	}
}

// ── FilterSubscribed ──────────────────────────────────────────────────────────

func TestFilterSubscribedEmpty(t *testing.T) {
	videos := []domain.Video{{ID: "a", ChannelID: "ch1"}}
	subscribed := make(map[string]bool)
	result := FilterSubscribed(videos, subscribed)
	if len(result) != len(videos) {
		t.Errorf("empty subscribed map: got len=%d, want %d", len(result), len(videos))
	}
}

func TestFilterSubscribedRemoves(t *testing.T) {
	videos := []domain.Video{
		{ID: "a", ChannelID: "ch1"},
		{ID: "b", ChannelID: "ch2"},
	}
	subscribed := map[string]bool{"ch1": true}
	result := FilterSubscribed(videos, subscribed)
	if len(result) != 1 || result[0].ID != "b" {
		t.Errorf("filter subscribed: got %d items, want 1 with ID='b'", len(result))
	}
}

func TestFilterSubscribedByName(t *testing.T) {
	videos := []domain.Video{
		{ID: "a", Channel: "MyChannel"},
	}
	subscribed := map[string]bool{"name:mychannel": true}
	result := FilterSubscribed(videos, subscribed)
	if len(result) != 0 {
		t.Errorf("filter by name: should filter case-insensitive, got len=%d", len(result))
	}
}

// ── Blocklist.Match / FilterBlacklisted ───────────────────────────────────────

func TestBlocklistMatch(t *testing.T) {
	bl := NewBlocklist([]string{"chX"})
	if !bl.Match(domain.Video{ChannelID: "chX"}) {
		t.Errorf("match by ID: want true")
	}
	if bl.Match(domain.Video{ChannelID: "other"}) {
		t.Errorf("non-blocked matched")
	}
	// Blocking is channel-ID-only: a channel name must never match.
	if bl.Match(domain.Video{Channel: "chX"}) {
		t.Errorf("name matched a blocklist that is ID-only")
	}
}

func TestFilterBlacklisted(t *testing.T) {
	bl := NewBlocklist([]string{"chX"})
	videos := []domain.Video{
		{ID: "a", ChannelID: "chX"},
		{ID: "b", ChannelID: "chY"},
	}
	out := FilterBlacklisted(videos, bl)
	if got := ids(out); got != "b" {
		t.Errorf("FilterBlacklisted: got %q, want 'b'", got)
	}
}
