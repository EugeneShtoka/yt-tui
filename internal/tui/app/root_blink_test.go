package app

import (
	"testing"

	"charm.land/bubbles/v2/cursor"
	tea "charm.land/bubbletea/v2"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// A cursor blink reaches only the visible tab. Broadcast to a hidden tab whose
// input is focused (Search, from startup), it re-armed the blink timer there
// forever and woke the whole program twice a second while idle.
func TestBlinkReachesOnlyActiveTab(t *testing.T) {
	r := Root{keys: testKeyMap(), router: tabRouter{tabs: []tuipkg.Tab{fakeTab{}, fakeTab{}}}}

	model, _ := r.Update(cursor.BlinkMsg{})
	r = model.(Root)

	if got := len(r.router.tabs[0].(fakeTab).received); got != 1 {
		t.Errorf("active tab got %d blink messages, want 1", got)
	}
	if got := r.router.tabs[1].(fakeTab).received; len(got) != 0 {
		t.Errorf("hidden tab got %v, want no blink", got)
	}
}

// Other broadcasts still reach every tab.
func TestBroadcastStillReachesHiddenTabs(t *testing.T) {
	type otherMsg struct{}
	r := Root{keys: testKeyMap(), router: tabRouter{tabs: []tuipkg.Tab{fakeTab{}, fakeTab{}}}}

	model, _ := r.Update(tea.Msg(otherMsg{}))
	r = model.(Root)

	for i, tab := range r.router.tabs {
		if got := len(tab.(fakeTab).received); got != 1 {
			t.Errorf("tab %d got %d messages, want 1", i, got)
		}
	}
}
