package db

import (
	"context"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// channelActivity returns the stored last_activity_at for a channel.
func channelActivity(t *testing.T, db *DB, id string) int64 {
	t.Helper()
	chs, err := db.AllChannels(context.Background())
	if err != nil {
		t.Fatalf("AllChannels: %v", err)
	}
	for i := range chs {
		if chs[i].ID == id {
			return chs[i].LastActivityAt
		}
	}
	t.Fatalf("channel %q not found", id)
	return 0
}

// addBareChannel creates a channel row without stamping activity (unlike
// AddSubscribedChannel, which counts creation as activity), so tests can control
// the last_activity_at baseline from 0.
func addBareChannel(t *testing.T, db *DB, id string) {
	t.Helper()
	if err := db.SetChannelState(context.Background(), id, domain.SubNone); err != nil {
		t.Fatalf("SetChannelState(%q): %v", id, err)
	}
}

func TestStampChannelActivityMonotonic(t *testing.T) {
	db := newTestDB(t)
	addBareChannel(t, db, "c1")
	// Stamp forward.
	if err := db.stampChannelActivityAt(context.Background(), []string{"c1"}, 1000); err != nil {
		t.Fatalf("stamp: %v", err)
	}
	if got := channelActivity(t, db, "c1"); got != 1000 {
		t.Fatalf("after stamp 1000: got %d", got)
	}
	// A lower stamp must not move it backward (MAX).
	if err := db.stampChannelActivityAt(context.Background(), []string{"c1"}, 500); err != nil {
		t.Fatalf("stamp: %v", err)
	}
	if got := channelActivity(t, db, "c1"); got != 1000 {
		t.Fatalf("lower stamp moved backward: got %d, want 1000", got)
	}
	// A higher stamp advances it.
	if err := db.stampChannelActivityAt(context.Background(), []string{"c1"}, 2000); err != nil {
		t.Fatalf("stamp: %v", err)
	}
	if got := channelActivity(t, db, "c1"); got != 2000 {
		t.Fatalf("higher stamp: got %d, want 2000", got)
	}
}

func TestStampChannelActivityAbsentIsNoOp(t *testing.T) {
	db := newTestDB(t)
	// No row for "ghost" — must not error and must create nothing.
	if err := db.stampChannelActivityAt(context.Background(), []string{"ghost", ""}, 1000); err != nil {
		t.Fatalf("stamp absent: %v", err)
	}
	chs, err := db.AllChannels(context.Background())
	if err != nil {
		t.Fatalf("AllChannels: %v", err)
	}
	if len(chs) != 0 {
		t.Fatalf("expected no channels, got %d", len(chs))
	}
}

func TestAddSubscribedChannelStampsActivity(t *testing.T) {
	db := newTestDB(t)
	before := time.Now().Unix()
	if err := db.AddSubscribedChannel(context.Background(), domain.Channel{ID: "c1", Name: "C1", State: domain.SubNone}); err != nil {
		t.Fatalf("AddSubscribedChannel: %v", err)
	}
	if got := channelActivity(t, db, "c1"); got < before {
		t.Fatalf("AddSubscribedChannel did not stamp activity: got %d, before %d", got, before)
	}
}

func TestSaveFeedCacheStampsPresentChannels(t *testing.T) {
	db := newTestDB(t)
	// A channel with a row (tracked, baseline 0) and one without (untracked,
	// must no-op — no row created).
	addBareChannel(t, db, "tracked")
	before := time.Now().Unix()
	videos := []domain.Video{
		{ID: "v1", Title: "V1", ChannelID: "tracked"},
		{ID: "v2", Title: "V2", ChannelID: "untracked"},
	}
	if err := db.SaveFeedCache(context.Background(), "recommended", videos); err != nil {
		t.Fatalf("SaveFeedCache: %v", err)
	}
	if got := channelActivity(t, db, "tracked"); got < before {
		t.Fatalf("feed did not stamp tracked channel: got %d, before %d", got, before)
	}
	// The untracked channel got no row created.
	chs, _ := db.AllChannels(context.Background())
	for i := range chs {
		if chs[i].ID == "untracked" {
			t.Fatalf("untracked channel should not have been created")
		}
	}
}

func TestAddHistoryStampsChannelForActivityEvents(t *testing.T) {
	db := newTestDB(t)
	// upsertTestVideo sets channel_id = "ch-"+id.
	upsertTestVideo(t, db, "v1")
	addBareChannel(t, db, "ch-v1")
	before := time.Now().Unix()
	if err := db.AddHistory(context.Background(), "v1", "playVideo", ""); err != nil {
		t.Fatalf("AddHistory: %v", err)
	}
	if got := channelActivity(t, db, "ch-v1"); got < before {
		t.Fatalf("playVideo did not stamp channel: got %d, before %d", got, before)
	}
}

func TestAddHistoryDoesNotStampForNonActivityEvents(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "v1")
	addBareChannel(t, db, "ch-v1")
	if err := db.stampChannelActivityAt(context.Background(), []string{"ch-v1"}, 42); err != nil {
		t.Fatalf("stamp baseline: %v", err)
	}
	// A delete event is not an activity event; it must not bump the stamp.
	if err := db.AddHistory(context.Background(), "v1", evtDelete, "manual"); err != nil {
		t.Fatalf("AddHistory: %v", err)
	}
	if got := channelActivity(t, db, "ch-v1"); got != 42 {
		t.Fatalf("non-activity event stamped channel: got %d, want 42", got)
	}
}
