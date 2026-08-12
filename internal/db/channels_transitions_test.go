package db

import (
	"context"
	"errors"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// stateByID reads every channel row (via AllChannels) into an id→Channel map so
// tests can assert on state/blocked without depending on ordering.
func allByID(t *testing.T, db *DB) map[string]domain.Channel {
	t.Helper()
	all, err := db.AllChannels(context.Background())
	if err != nil {
		t.Fatalf("AllChannels: %v", err)
	}
	m := make(map[string]domain.Channel, len(all))
	for i := range all {
		m[all[i].ID] = all[i]
	}
	return m
}

// TestAllChannelsIncludesEveryState confirms AllChannels returns subscribed,
// none-annotated, and blocked rows alike, while GetSubscribedChannels projects
// down to subscriptions only.
func TestAllChannelsIncludesEveryState(t *testing.T) {
	db := newTestDB(t)

	if err := db.AddSubscribedChannel(context.Background(), domain.Channel{ID: "yt1", Name: "YT", State: domain.SubYT}); err != nil {
		t.Fatalf("add yt: %v", err)
	}
	// A none-annotated row (tagged but not subscribed).
	if err := db.SetChannelState(context.Background(), "none1", domain.SubNone); err != nil {
		t.Fatalf("set none: %v", err)
	}
	if err := db.SetChannelTags(context.Background(), "none1", []string{"topic"}); err != nil {
		t.Fatalf("set tags: %v", err)
	}
	if err := db.BlockChannel(context.Background(), "blk1"); err != nil {
		t.Fatalf("block: %v", err)
	}

	all := allByID(t, db)
	for _, id := range []string{"yt1", "none1", "blk1"} {
		if _, ok := all[id]; !ok {
			t.Errorf("AllChannels missing %q", id)
		}
	}

	subs, err := db.GetSubscribedChannels(context.Background())
	if err != nil {
		t.Fatalf("GetSubscribedChannels: %v", err)
	}
	if len(subs) != 1 || subs[0].ID != "yt1" {
		t.Errorf("GetSubscribedChannels = %+v, want only yt1", subs)
	}
}

// TestBlockUnblockChannel covers the guarded block transition and unblock.
func TestBlockUnblockChannel(t *testing.T) {
	db := newTestDB(t)

	// Blocking a currently-subscribed channel unsubscribes it (state→none).
	if err := db.AddSubscribedChannel(context.Background(), domain.Channel{ID: "ch1", Name: "One", State: domain.SubYT}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := db.BlockChannel(context.Background(), "ch1"); err != nil {
		t.Fatalf("block: %v", err)
	}

	got := allByID(t, db)["ch1"]
	if !got.Blocked {
		t.Error("ch1 should be blocked")
	}
	if got.State != domain.SubNone {
		t.Errorf("ch1 state = %q, want none (blocked ⟹ none)", got.State)
	}
	if got.Name != "One" {
		t.Errorf("ch1 name = %q, want preserved 'One'", got.Name)
	}

	blocked, err := db.BlockedChannels(context.Background())
	if err != nil {
		t.Fatalf("BlockedChannels: %v", err)
	}
	if len(blocked) != 1 || blocked[0].ID != "ch1" {
		t.Errorf("BlockedChannels = %+v, want [ch1]", blocked)
	}

	// Unblock leaves the channel at none, not resubscribed.
	if err := db.UnblockChannel(context.Background(), "ch1"); err != nil {
		t.Fatalf("unblock: %v", err)
	}
	got = allByID(t, db)["ch1"]
	if got.Blocked {
		t.Error("ch1 should be unblocked")
	}
	if got.State != domain.SubNone {
		t.Errorf("ch1 state after unblock = %q, want none", got.State)
	}
	if bl, _ := db.BlockedChannels(context.Background()); len(bl) != 0 {
		t.Errorf("BlockedChannels after unblock = %+v, want empty", bl)
	}
}

// TestSubscribeBlockedChannelRejected covers the block invariant on the two
// subscribe paths: AddSubscribedChannel and SetChannelState.
func TestSubscribeBlockedChannelRejected(t *testing.T) {
	db := newTestDB(t)
	if err := db.BlockChannel(context.Background(), "blk1"); err != nil {
		t.Fatalf("block: %v", err)
	}

	if err := db.AddSubscribedChannel(context.Background(), domain.Channel{ID: "blk1", Name: "Blk", State: domain.SubYT}); !errors.Is(err, domain.ErrChannelBlocked) {
		t.Errorf("AddSubscribedChannel on blocked = %v, want ErrChannelBlocked", err)
	}
	if err := db.SetChannelState(context.Background(), "blk1", domain.SubLocal); !errors.Is(err, domain.ErrChannelBlocked) {
		t.Errorf("SetChannelState(local) on blocked = %v, want ErrChannelBlocked", err)
	}
	// The channel stayed blocked/none despite the rejected attempts.
	got := allByID(t, db)["blk1"]
	if !got.Blocked || got.State != domain.SubNone {
		t.Errorf("blk1 = {blocked:%v state:%q}, want blocked+none", got.Blocked, got.State)
	}

	// Transitioning to none is always allowed.
	if err := db.SetChannelState(context.Background(), "blk1", domain.SubNone); err != nil {
		t.Errorf("SetChannelState(none) on blocked = %v, want nil", err)
	}
}

// TestSetChannelStateTransitions covers the non-blocked state transitions and
// that is_local stays in sync for the local state.
func TestSetChannelStateTransitions(t *testing.T) {
	db := newTestDB(t)
	if err := db.SetChannelState(context.Background(), "ch1", domain.SubLocal); err != nil {
		t.Fatalf("set local: %v", err)
	}
	got := allByID(t, db)["ch1"]
	if got.State != domain.SubLocal || !got.IsLocal {
		t.Errorf("ch1 = {state:%q isLocal:%v}, want local+true", got.State, got.IsLocal)
	}
	if err := db.SetChannelState(context.Background(), "ch1", domain.SubYT); err != nil {
		t.Fatalf("set yt: %v", err)
	}
	got = allByID(t, db)["ch1"]
	if got.State != domain.SubYT || got.IsLocal {
		t.Errorf("ch1 = {state:%q isLocal:%v}, want yt+false", got.State, got.IsLocal)
	}
}

// TestSaveSubscribedChannelsTransitionNotDelete is the core sync-safety test: a
// YT channel that drops off the fetch must be transitioned to none (keeping its
// annotations) rather than deleted, an annotation-free one is GC'd, a blocked
// row is never resurrected, and local subs are untouched.
func TestSaveSubscribedChannelsTransitionNotDelete(t *testing.T) {
	db := newTestDB(t)

	// Seed: two YT subs (one annotated), a local sub, and a blocked channel.
	if err := db.SaveSubscribedChannels(context.Background(), []domain.Channel{
		{ID: "keep", Name: "Keep"},
		{ID: "annot", Name: "Annotated"},
		{ID: "bare", Name: "Bare"},
	}); err != nil {
		t.Fatalf("seed yt: %v", err)
	}
	if err := db.SetChannelTags(context.Background(), "annot", []string{"news"}); err != nil {
		t.Fatalf("tag annot: %v", err)
	}
	if err := db.AddSubscribedChannel(context.Background(), domain.Channel{ID: "loc", Name: "Local", IsLocal: true}); err != nil {
		t.Fatalf("add local: %v", err)
	}
	if err := db.BlockChannel(context.Background(), "blk"); err != nil {
		t.Fatalf("block: %v", err)
	}

	// Next sync only returns "keep" (annot, bare dropped off) — and, perversely,
	// still lists the blocked channel, which must not un-block it.
	if err := db.SaveSubscribedChannels(context.Background(), []domain.Channel{
		{ID: "keep", Name: "Keep"},
		{ID: "blk", Name: "Blk"},
	}); err != nil {
		t.Fatalf("resync: %v", err)
	}

	all := allByID(t, db)

	if all["keep"].State != domain.SubYT {
		t.Errorf("keep state = %q, want subscribed_yt", all["keep"].State)
	}
	// Annotated dropped-off channel kept, transitioned to none, tags intact.
	annot, ok := all["annot"]
	if !ok {
		t.Fatal("annotated channel was deleted; want transitioned to none")
	}
	if annot.State != domain.SubNone {
		t.Errorf("annot state = %q, want none", annot.State)
	}
	if len(annot.Tags) != 1 || annot.Tags[0] != "news" {
		t.Errorf("annot tags = %v, want [news]", annot.Tags)
	}
	// Bare dropped-off channel garbage-collected.
	if _, ok := all["bare"]; ok {
		t.Error("bare annotation-free channel should have been GC'd")
	}
	// Local sub untouched.
	if all["loc"].State != domain.SubLocal {
		t.Errorf("loc state = %q, want subscribed_local", all["loc"].State)
	}
	// Blocked channel not resurrected despite appearing in the fetch.
	if !all["blk"].Blocked || all["blk"].State != domain.SubNone {
		t.Errorf("blk = {blocked:%v state:%q}, want blocked+none", all["blk"].Blocked, all["blk"].State)
	}
}
