package db

import (
	"context"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	_ "modernc.org/sqlite"
)

// TestAddSubscribedChannelPersistsState covers both an explicit State and the
// legacy IsLocal-derived path through SubState().
func TestAddSubscribedChannelPersistsState(t *testing.T) {
	db := newTestDB(t)

	if err := db.AddSubscribedChannel(context.Background(), domain.Channel{ID: "yt1", Name: "YT", State: domain.SubYT}); err != nil {
		t.Fatalf("Add yt: %v", err)
	}
	if err := db.AddSubscribedChannel(context.Background(), domain.Channel{ID: "loc1", Name: "Local", IsLocal: true}); err != nil {
		t.Fatalf("Add local: %v", err)
	}

	got, err := db.GetSubscribedChannels(context.Background())
	if err != nil {
		t.Fatalf("GetSubscribedChannels: %v", err)
	}
	states := make(map[string]domain.SubscriptionState, len(got))
	for _, ch := range got {
		states[ch.ID] = ch.State
		if ch.Blocked {
			t.Errorf("channel %q unexpectedly blocked", ch.ID)
		}
	}
	if states["yt1"] != domain.SubYT {
		t.Errorf("yt1 state = %q, want %q", states["yt1"], domain.SubYT)
	}
	if states["loc1"] != domain.SubLocal {
		t.Errorf("loc1 state = %q, want %q (derived from IsLocal)", states["loc1"], domain.SubLocal)
	}
}

// TestSaveSubscribedChannelsSetsYTState confirms a YT sync marks rows subscribed_yt.
func TestSaveSubscribedChannelsSetsYTState(t *testing.T) {
	db := newTestDB(t)

	if err := db.SaveSubscribedChannels(context.Background(), []domain.Channel{
		{ID: "ch1", Name: "One"},
		{ID: "ch2", Name: "Two"},
	}); err != nil {
		t.Fatalf("SaveSubscribedChannels: %v", err)
	}

	got, err := db.GetSubscribedChannels(context.Background())
	if err != nil {
		t.Fatalf("GetSubscribedChannels: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	for _, ch := range got {
		if ch.State != domain.SubYT {
			t.Errorf("channel %q state = %q, want %q", ch.ID, ch.State, domain.SubYT)
		}
	}
}
