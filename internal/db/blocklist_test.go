package db

import (
	"context"
	"sort"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// TestResolveBlockedName upgrades a name-only block to an ID-keyed block: the
// name leaves blocked_names, the ID joins the blocked set, and the invariant
// keeps it out of subscriptions.
func TestResolveBlockedName(t *testing.T) {
	db := newTestDB(t)

	if err := db.AddBlockedName(context.Background(), "Bad Chan"); err != nil {
		t.Fatalf("AddBlockedName: %v", err)
	}
	if err := db.ResolveBlockedName(context.Background(), "Bad Chan", "chBad"); err != nil {
		t.Fatalf("ResolveBlockedName: %v", err)
	}

	ids, names, err := db.Blocklist(context.Background())
	if err != nil {
		t.Fatalf("Blocklist: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("blocked_names not cleared after resolve: %v", names)
	}
	if len(ids) != 1 || ids[0] != "chBad" {
		t.Errorf("resolved id missing: ids=%v, want [chBad]", ids)
	}

	subs, err := db.GetSubscribedChannels(context.Background())
	if err != nil {
		t.Fatalf("GetSubscribedChannels: %v", err)
	}
	if len(subs) != 0 {
		t.Errorf("resolved block leaked into subscriptions: %+v", subs)
	}
}

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

	ids, _, err := db.Blocklist(context.Background())
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
	ids, names, err := db.Blocklist(context.Background())
	if err != nil {
		t.Fatalf("Blocklist: %v", err)
	}
	sort.Strings(ids)
	sort.Strings(names)
	if len(ids) != 0 || len(names) != 0 {
		t.Errorf("fresh Blocklist = (%v,%v), want empty", ids, names)
	}
}
