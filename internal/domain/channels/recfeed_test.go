package channels

import (
	"reflect"
	"sort"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

func TestRecFeedIDs(t *testing.T) {
	vids := []domain.Video{
		{ID: "v1", ChannelID: "a"},
		{ID: "v2", ChannelID: "b"},
		{ID: "v3", ChannelID: "a"}, // dup
		{ID: "v4", ChannelID: ""},  // missing channel id, skipped
	}
	got := RecFeedIDs(vids)
	want := map[string]bool{"a": true, "b": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RecFeedIDs: got %v, want %v", got, want)
	}
}

func TestSynthesizeRecSkipsStoredAndDeduplicates(t *testing.T) {
	have := []domain.Channel{
		{ID: "sub", Name: "Subbed", State: domain.SubYT},
		{ID: "blk", Name: "Blocked", State: domain.SubNone, Blocked: true},
	}
	vids := []domain.Video{
		{ID: "v1", ChannelID: "sub", Channel: "Subbed"},  // already stored → skip
		{ID: "v2", ChannelID: "blk", Channel: "Blocked"}, // already stored → skip
		{ID: "v3", ChannelID: "new", Channel: "NewChan"}, // synthesize
		{ID: "v4", ChannelID: "new", Channel: "NewChan"}, // dup → once
		{ID: "v5", ChannelID: ""},                        // no id → skip
	}
	got := SynthesizeRec(have, vids)
	if len(got) != 1 {
		t.Fatalf("want 1 synthesized channel, got %d (%+v)", len(got), got)
	}
	ch := got[0]
	if ch.ID != "new" || ch.Name != "NewChan" {
		t.Errorf("synthesized channel identity: got %+v", ch)
	}
	if ch.State != domain.SubNone {
		t.Errorf("synthesized channel must be state=none, got %q", ch.State)
	}
	if ch.Blocked {
		t.Error("synthesized channel must not be blocked")
	}
	if ch.URL != "https://www.youtube.com/channel/new" {
		t.Errorf("synthesized channel URL: got %q", ch.URL)
	}
}

func TestSynthesizeRecPreservesFirstSeenName(t *testing.T) {
	vids := []domain.Video{
		{ID: "v1", ChannelID: "x", Channel: "First"},
		{ID: "v2", ChannelID: "x", Channel: "Second"},
	}
	got := SynthesizeRec(nil, vids)
	if len(got) != 1 || got[0].Name != "First" {
		ids := make([]string, len(got))
		for i := range got {
			ids[i] = got[i].Name
		}
		sort.Strings(ids)
		t.Fatalf("want single channel named First, got %v", ids)
	}
}
