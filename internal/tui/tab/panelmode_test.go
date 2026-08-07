package tab

import (
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

func TestSrcModeIncludes(t *testing.T) {
	sub := domain.Channel{ID: "s", State: domain.SubYT}
	none := domain.Channel{ID: "n", State: domain.SubNone}
	blk := domain.Channel{ID: "b", State: domain.SubNone, Blocked: true}

	cases := []struct {
		name      string
		mode      srcMode
		ch        domain.Channel
		inRecFeed bool
		want      bool
	}{
		{"recommended: rec-feed none channel", srcRecommended, none, true, true},
		{"recommended: none but not in feed", srcRecommended, none, false, false},
		{"recommended: subscribed excluded", srcRecommended, sub, true, false},
		{"recommended: blocked excluded", srcRecommended, blk, true, false},
		{"subscribed: yes", srcSubscribed, sub, false, true},
		{"subscribed: none excluded", srcSubscribed, none, true, false},
		{"subscribed: blocked excluded", srcSubscribed, blk, false, false},
		{"mixed: subscribed", srcMixed, sub, false, true},
		{"mixed: rec none", srcMixed, none, true, true},
		{"mixed: annotated none not in feed still shown", srcMixed, none, false, true},
		{"mixed: blocked excluded", srcMixed, blk, true, false},
		{"blocked: only blocked", srcBlocked, blk, false, true},
		{"blocked: subscribed excluded", srcBlocked, sub, false, false},
	}
	for _, c := range cases {
		if got := c.mode.includes(c.ch, c.inRecFeed); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestParseSrcModeLegacyAll(t *testing.T) {
	if parseSrcMode("all") != srcMixed {
		t.Error("legacy \"all\" should map to mixed")
	}
	if parseSrcMode("garbage") != srcSubscribed {
		t.Error("unknown mode should default to subscribed")
	}
	if parseSrcMode("recommended") != srcRecommended {
		t.Error("recommended parse")
	}
	if parseSrcMode("stale") != srcStale {
		t.Error("stale parse")
	}
}

// staleChannel builds a tagged, unsubscribed channel whose last activity is
// daysAgo before now.
func staleChannel(id string, now time.Time, daysAgo int) domain.Channel {
	return domain.Channel{
		ID: id, State: domain.SubNone, Tags: []string{"topic"},
		LastActivityAt: now.Add(-time.Duration(daysAgo) * 24 * time.Hour).Unix(),
	}
}

func TestSelectChannelsStaleMode(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	staleCh := staleChannel("stale", now, 40)                                    // tagged, inactive 40d
	freshCh := staleChannel("fresh", now, 5)                                     // tagged, active 5d ago
	subCh := domain.Channel{ID: "sub", State: domain.SubYT, Tags: []string{"t"}} // subscribed, exempt
	untagged := domain.Channel{ID: "untag", State: domain.SubNone}               // no tags, not stale
	universe := []domain.Channel{staleCh, freshCh, subCh, untagged}
	sf := staleFilter{hide: true, days: 30}

	// srcStale surfaces exactly the stale tagged channel.
	got := selectChannels(universe, srcStale, map[string]bool{}, sf, now)
	if len(got) != 1 || got[0].ID != "stale" {
		t.Fatalf("srcStale: got %v, want [stale]", ids(got))
	}

	// srcMixed with hide on excludes the stale channel but keeps the rest.
	got = selectChannels(universe, srcMixed, map[string]bool{}, sf, now)
	if contains(got, "stale") {
		t.Errorf("srcMixed with hide on should exclude stale channel: %v", ids(got))
	}
	if !contains(got, "fresh") || !contains(got, "sub") || !contains(got, "untag") {
		t.Errorf("srcMixed dropped non-stale channels: %v", ids(got))
	}
}

func TestSelectChannelsHideOffKeepsStaleInOtherModes(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	staleCh := staleChannel("stale", now, 40)
	universe := []domain.Channel{staleCh}
	sf := staleFilter{hide: false, days: 30}

	// Hide off: the stale channel still appears in mixed…
	got := selectChannels(universe, srcMixed, map[string]bool{}, sf, now)
	if !contains(got, "stale") {
		t.Errorf("hide off: stale channel should remain in mixed: %v", ids(got))
	}
	// …and the stale mode still lists it (no exclusion needed to populate it).
	got = selectChannels(universe, srcStale, map[string]bool{}, sf, now)
	if !contains(got, "stale") {
		t.Errorf("stale mode should list the stale channel regardless of hide: %v", ids(got))
	}
}

func TestSelectChannelsInRecFeedIsNeverStale(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	staleCh := staleChannel("c", now, 40) // old timestamp…
	universe := []domain.Channel{staleCh}
	sf := staleFilter{hide: true, days: 30}
	inFeed := map[string]bool{"c": true} // …but currently in the recommended feed

	// Not stale (in feed = active), so srcStale excludes it…
	if got := selectChannels(universe, srcStale, inFeed, sf, now); len(got) != 0 {
		t.Errorf("in-feed channel should not be stale: %v", ids(got))
	}
	// …and recommended keeps it (unsubscribed rec-feed channel, not hidden).
	if got := selectChannels(universe, srcRecommended, inFeed, sf, now); !contains(got, "c") {
		t.Errorf("in-feed channel should show in recommended: %v", ids(got))
	}
}

func ids(chs []domain.Channel) []string {
	out := make([]string, len(chs))
	for i := range chs {
		out[i] = chs[i].ID
	}
	return out
}

func contains(chs []domain.Channel, id string) bool {
	for i := range chs {
		if chs[i].ID == id {
			return true
		}
	}
	return false
}
