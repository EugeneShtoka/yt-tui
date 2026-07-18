package feed

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/config"
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

// ── FilterByAge ───────────────────────────────────────────────────────────────

func TestFilterByAgeZeroOrNegative(t *testing.T) {
	videos := []domain.Video{{ID: "a", UploadDate: "20200101"}}
	result := FilterByAge(videos, 0)
	if len(result) != len(videos) {
		t.Errorf("zero/negative maxDays: got len=%d, want %d", len(result), len(videos))
	}
}

func TestFilterByAgeNoDate(t *testing.T) {
	videos := []domain.Video{{ID: "a", UploadDate: ""}}
	result := FilterByAge(videos, 7)
	if len(result) != 1 {
		t.Errorf("no date kept: got len=%d, want 1", len(result))
	}
}

func TestFilterByAgeMalformedDate(t *testing.T) {
	videos := []domain.Video{{ID: "a", UploadDate: "invalid"}}
	result := FilterByAge(videos, 7)
	if len(result) != 1 {
		t.Errorf("malformed date kept: got len=%d, want 1", len(result))
	}
}

// ── FilterByMinDuration ───────────────────────────────────────────────────────

func TestFilterByMinDuration(t *testing.T) {
	videos := []domain.Video{
		{ID: "short", Duration: 30},
		{ID: "long", Duration: 600},
		{ID: "unknown", Duration: 0},
	}
	if got := ids(FilterByMinDuration(videos, 0)); got != "short long unknown" {
		t.Errorf("minSecs=0 skips filter: got %q", got)
	}
	if got := ids(FilterByMinDuration(videos, 60)); got != "long unknown" {
		t.Errorf("minSecs=60: got %q, want 'long unknown' (0-duration kept)", got)
	}
}

// ── FilterByMinViews ──────────────────────────────────────────────────────────

func TestFilterByMinViews(t *testing.T) {
	videos := []domain.Video{
		{ID: "few", ViewCount: 50},
		{ID: "many", ViewCount: 5000},
		{ID: "unknown", ViewCount: 0},
	}
	if got := ids(FilterByMinViews(videos, 0)); got != "few many unknown" {
		t.Errorf("minViews=0 skips filter: got %q", got)
	}
	if got := ids(FilterByMinViews(videos, 100)); got != "many unknown" {
		t.Errorf("minViews=100: got %q, want 'many unknown' (0-view kept)", got)
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

// ── MatchBlacklisted / FilterBlacklisted ──────────────────────────────────────

func TestMatchBlacklisted(t *testing.T) {
	list := []config.BlacklistedChannel{
		{ID: "chX"},
		{Name: "Spammy"},
	}
	if i, ok := MatchBlacklisted(domain.Video{ChannelID: "chX"}, list); !ok || i != 0 {
		t.Errorf("match by ID: got (%d,%v), want (0,true)", i, ok)
	}
	if i, ok := MatchBlacklisted(domain.Video{Channel: "spammy"}, list); !ok || i != 1 {
		t.Errorf("match by name (case-insensitive): got (%d,%v), want (1,true)", i, ok)
	}
	if _, ok := MatchBlacklisted(domain.Video{ChannelID: "other"}, list); ok {
		t.Errorf("non-blacklisted matched")
	}
}

func TestFilterBlacklisted(t *testing.T) {
	// Use ID-populated entries so the name-enrichment (SaveAsync) branch is not hit.
	cfg := &config.Config{DaemonConfig: config.DaemonConfig{BlacklistedChannels: []config.BlacklistedChannel{{ID: "chX"}}}}
	videos := []domain.Video{
		{ID: "a", ChannelID: "chX"},
		{ID: "b", ChannelID: "chY"},
	}
	if got := ids(FilterBlacklisted(videos, cfg.BlacklistedChannels, cfg)); got != "b" {
		t.Errorf("FilterBlacklisted: got %q, want 'b'", got)
	}
}
