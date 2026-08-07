package db

import (
	"context"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// TestUpsertChannelFullRow writes a complete none-state annotated row and reads
// it back, then upserts over it to confirm every column is replaced with the
// resolved values the caller supplies.
func TestUpsertChannelFullRow(t *testing.T) {
	db := newTestDB(t)

	in := domain.Channel{
		ID: "c1", Name: "Chan", URL: "u1", Subscribers: 10,
		Alias: "nick", Tags: []string{"news", "tech"}, State: domain.SubNone,
	}
	if err := db.UpsertChannel(context.Background(), in); err != nil {
		t.Fatalf("UpsertChannel: %v", err)
	}
	got := allByID(t, db)["c1"]
	if got.Name != "Chan" || got.URL != "u1" || got.Subscribers != 10 ||
		got.Alias != "nick" || len(got.Tags) != 2 || got.State != domain.SubNone {
		t.Fatalf("first upsert mismatch: %+v", got)
	}

	// Re-upsert with a resolved (already-merged) row: local sub, extra tag.
	in.State = domain.SubLocal
	in.Tags = []string{"news", "tech", "go"}
	in.Alias = "renamed"
	if err := db.UpsertChannel(context.Background(), in); err != nil {
		t.Fatalf("UpsertChannel update: %v", err)
	}
	got = allByID(t, db)["c1"]
	if got.State != domain.SubLocal || !got.IsLocal || got.Alias != "renamed" || len(got.Tags) != 3 {
		t.Fatalf("second upsert mismatch: %+v", got)
	}
}

// TestUpsertChannelBlockInvariant confirms a blocked row is forced to
// subscription_state='none' in SQL even when a (malformed) subscribed state is
// passed in — the DB is the last line of defense for blocked ⟹ none.
func TestUpsertChannelBlockInvariant(t *testing.T) {
	db := newTestDB(t)

	if err := db.UpsertChannel(context.Background(), domain.Channel{
		ID: "b1", Name: "Bad", State: domain.SubYT, Blocked: true,
	}); err != nil {
		t.Fatalf("UpsertChannel: %v", err)
	}
	got := allByID(t, db)["b1"]
	if !got.Blocked || got.State != domain.SubNone {
		t.Fatalf("block invariant violated: blocked=%v state=%q", got.Blocked, got.State)
	}
	// It shows up in the blocklist projection by ID.
	ids, _, err := db.Blocklist(context.Background())
	if err != nil {
		t.Fatalf("Blocklist: %v", err)
	}
	if len(ids) != 1 || ids[0] != "b1" {
		t.Fatalf("blocklist ids: want [b1], got %v", ids)
	}
}

func TestUpsertChannelEmptyIDNoOp(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertChannel(context.Background(), domain.Channel{ID: ""}); err != nil {
		t.Fatalf("UpsertChannel empty id: %v", err)
	}
	if all, _ := db.AllChannels(context.Background()); len(all) != 0 {
		t.Fatalf("empty-id upsert wrote a row: %+v", all)
	}
}

// TestAddBlockedNameIdempotent checks the name-only block lands in the blocklist
// names projection and a duplicate insert is a no-op.
func TestAddBlockedNameIdempotent(t *testing.T) {
	db := newTestDB(t)
	for i := 0; i < 2; i++ {
		if err := db.AddBlockedName(context.Background(), "Spammer"); err != nil {
			t.Fatalf("AddBlockedName: %v", err)
		}
	}
	if err := db.AddBlockedName(context.Background(), ""); err != nil {
		t.Fatalf("AddBlockedName empty: %v", err)
	}
	_, names, err := db.Blocklist(context.Background())
	if err != nil {
		t.Fatalf("Blocklist: %v", err)
	}
	if len(names) != 1 || names[0] != "Spammer" {
		t.Fatalf("blocked names: want [Spammer], got %v", names)
	}
}

// TestAddHistoryEventPreservesTimestamp confirms the explicit timestamp survives
// the round-trip (to the second) and an empty videoID stores a search-style event.
func TestAddHistoryEventPreservesTimestamp(t *testing.T) {
	db := newTestDB(t)

	// history.video_id has an FK → videos(id); the referenced row must exist.
	if err := db.UpsertVideo(context.Background(), "v1", "One", "C", "ch", 60, 1, "20240101", "u"); err != nil {
		t.Fatalf("UpsertVideo: %v", err)
	}
	ts := time.Date(2024, 3, 1, 12, 30, 0, 0, time.UTC)
	if err := db.AddHistoryEvent(context.Background(), "v1", "playVideo", "", ts); err != nil {
		t.Fatalf("AddHistoryEvent: %v", err)
	}
	if err := db.AddHistoryEvent(context.Background(), "", "search", "golang", ts.Add(time.Minute)); err != nil {
		t.Fatalf("AddHistoryEvent search: %v", err)
	}

	entries, err := db.History(context.Background(), -1)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("History: want 2, got %d", len(entries))
	}
	byType := map[string]domain.HistoryEntry{}
	for _, e := range entries {
		byType[e.EventType] = e
	}
	if got := byType["playVideo"]; got.VideoID != "v1" || !got.Timestamp.Equal(ts) {
		t.Errorf("playVideo event: id=%q ts=%v (want v1 / %v)", got.VideoID, got.Timestamp, ts)
	}
	if got := byType["search"]; got.VideoID != "" || got.Details != "golang" {
		t.Errorf("search event mismatch: %+v", got)
	}
}
