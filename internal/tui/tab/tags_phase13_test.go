package tab

import (
	"context"
	"reflect"
	"sort"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

func tagNames(t Tags) []string {
	out := make([]string, len(t.sortedTagRows))
	for i := range t.sortedTagRows {
		out[i] = t.sortedTagRows[i].Tag
	}
	sort.Strings(out)
	return out
}

func tagsLoaded(mode string, chans []domain.Channel, rec []domain.Video) Tags {
	tg := NewTags(context.Background(), &fakeBackend{}, testKeys(), false, TagsOpts{Mode: mode, StaleDays: 30})
	m, _ := tg.Update(tagsDataMsg{chans: chans, recVideos: rec})
	return m.(Tags)
}

// phase13Channels: a subscribed channel, a tagged rec-feed (state=none) channel,
// and a blocked one — each carrying a distinct tag.
func phase13Channels() []domain.Channel {
	return []domain.Channel{
		{ID: "sub", Name: "Sub", State: domain.SubYT, Tags: []string{"gonews"}},
		{ID: "rec", Name: "Rec", State: domain.SubNone, Tags: []string{"sci"}},
		{ID: "blk", Name: "Blk", State: domain.SubNone, Blocked: true, Tags: []string{"bad"}},
	}
}

func TestTagsModeFiltersTagSet(t *testing.T) {
	// "rec" appears in the recommended feed → recommended/mixed modes see it.
	rec := []domain.Video{{ID: "v1", ChannelID: "rec", Channel: "Rec"}}
	cases := []struct {
		mode string
		want []string
	}{
		{"subscribed", []string{"gonews"}},
		{"recommended", []string{"sci"}},     // rec-feed, not subscribed, not blocked
		{"mixed", []string{"gonews", "sci"}}, // union; blocked "bad" excluded
	}
	for _, c := range cases {
		got := tagNames(tagsLoaded(c.mode, phase13Channels(), rec))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("mode %q: got %v, want %v", c.mode, got, c.want)
		}
	}
}

// A tagged state=none channel that is NOT in the recommended feed is excluded from
// the recommended mode (the mode reflects the live feed, not every stored row).
func TestTagsRecommendedModeRequiresFeedPresence(t *testing.T) {
	got := tagNames(tagsLoaded("recommended", phase13Channels(), nil))
	if len(got) != 0 {
		t.Errorf("recommended mode with no feed videos should be empty, got %v", got)
	}
}

func TestTagsModePickerSwitches(t *testing.T) {
	rec := []domain.Video{{ID: "v1", ChannelID: "rec", Channel: "Rec"}}
	tg := tagsLoaded("subscribed", phase13Channels(), rec)

	m, _ := tg.Update(tea.KeyPressMsg{Text: "M"}) // PanelMode → open picker
	tg = m.(Tags)
	if !tg.picker.isOpen() {
		t.Fatal("PanelMode should open the mode picker")
	}
	if !tg.InterceptsInput() {
		t.Error("open picker must intercept input")
	}
	if tg.picker.selection() != modeIndex(tagModes, srcSubscribed) {
		t.Errorf("picker should start on the active mode, got %d", tg.picker.selection())
	}

	// From Subscribed (index 1) move up to Recommended (index 0) and confirm.
	m, _ = tg.Update(tea.KeyPressMsg{Text: "k"})
	tg = m.(Tags)
	m, _ = tg.Update(tea.KeyPressMsg{Text: "enter"})
	tg = m.(Tags)

	if tg.picker.isOpen() {
		t.Error("Enter should close the picker")
	}
	if tg.mode != srcRecommended {
		t.Errorf("mode should be recommended, got %d", tg.mode)
	}
	if got := tagNames(tg); !reflect.DeepEqual(got, []string{"sci"}) {
		t.Errorf("recommended tags after switch: got %v", got)
	}
}

func TestNewTagsRejectsBlockedMode(t *testing.T) {
	tg := NewTags(context.Background(), &fakeBackend{}, testKeys(), false, TagsOpts{Mode: "blocked", StaleDays: 30})
	if tg.mode != srcSubscribed {
		t.Errorf("Tags has no blocked mode; should fall back to subscribed, got %d", tg.mode)
	}
}
