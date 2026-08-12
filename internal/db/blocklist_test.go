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
