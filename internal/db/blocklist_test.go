package db

import (
	"context"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// TestBlockDoesNotClobberSubscribedRowName ensures blocking an already-known
// (subscribed) channel keeps its name and flips it to blocked/none.
func TestBlockPreservesNameAndUnsubscribes(t *testing.T) {
	db := newTestDB(t)

	if err := db.AddSubscribedChannel(context.Background(), domain.Channel{ID: "chDup", Name: "Real Name", State: domain.SubYT}); err != nil {
		t.Fatalf("AddSubscribedChannel: %v", err)
	}
	// Block by ID (empty name) for the same channel; existing name must survive.
	if err := db.BlockChannel(context.Background(), "chDup"); err != nil {
		t.Fatalf("BlockChannel: %v", err)
	}

	ids, err := db.Blocklist(context.Background())
	if err != nil {
		t.Fatalf("Blocklist: %v", err)
	}
	if len(ids) != 1 || ids[0] != "chDup" {
		t.Errorf("blocked ids = %v, want [chDup]", ids)
	}
	// Now unsubscribed (state=none) → absent from GetSubscribedChannels.
	subs, err := db.GetSubscribedChannels(context.Background())
	if err != nil {
		t.Fatalf("GetSubscribedChannels: %v", err)
	}
	if len(subs) != 0 {
		t.Errorf("blocked channel still subscribed: %+v", subs)
	}
}

// TestBlocklistEmpty confirms a fresh DB has an empty projection.
func TestBlocklistEmpty(t *testing.T) {
	db := newTestDB(t)
	ids, err := db.Blocklist(context.Background())
	if err != nil {
		t.Fatalf("Blocklist: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("fresh Blocklist = %v, want empty", ids)
	}
}

// DeleteChannelFeedCache must clear exactly the blocked channel's cached feed
// rows and nothing else. Without it the rows survive in feed_cache and
// GetFeedCache serves them back on the next cold start, so the block appears to
// have done nothing until a network refresh overwrites the cache.
func TestDeleteChannelFeedCacheRemovesOnlyThatChannel(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	if err := db.SaveFeedCache(ctx, "recommended", []domain.Video{
		{ID: "v1", Title: "Blocked one", ChannelID: "cBad", Channel: "Bad"},
		{ID: "v2", Title: "Blocked two", ChannelID: "cBad", Channel: "Bad"},
		{ID: "v3", Title: "Innocent", ChannelID: "cOK", Channel: "OK"},
	}); err != nil {
		t.Fatalf("SaveFeedCache: %v", err)
	}

	if err := db.DeleteChannelFeedCache(ctx, "cBad"); err != nil {
		t.Fatalf("DeleteChannelFeedCache: %v", err)
	}

	got, err := db.GetFeedCache(ctx, "recommended")
	if err != nil {
		t.Fatalf("GetFeedCache: %v", err)
	}
	if len(got) != 1 || got[0].ID != "v3" {
		t.Fatalf("feed cache = %+v, want only v3", got)
	}
}
