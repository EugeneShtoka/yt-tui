package db

import (
	"context"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ── SaveChannelVideos / GetChannelVideos round-trips ─────────────────────────

func TestSaveChannelVideosRoundTrip(t *testing.T) {
	db := newTestDB(t)

	videos := []domain.Video{
		{ID: "v1", Title: "T1", Channel: "C", ChannelID: "ch1", Duration: 100, ViewCount: 500, UploadDate: "20240101", URL: "https://example.com/v1"},
	}
	if err := db.SaveChannelVideos(context.Background(), "ch1", videos); err != nil {
		t.Fatalf("SaveChannelVideos: %v", err)
	}

	got, err := db.GetChannelVideos(context.Background(), "ch1")
	if err != nil {
		t.Fatalf("GetChannelVideos: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("GetChannelVideos len = %d, want 1", len(got))
	}
	if got[0].URL != "https://example.com/v1" {
		t.Errorf("URL = %q, want preserved", got[0].URL)
	}
	if got[0].Title != "T1" || got[0].Duration != 100 || got[0].ViewCount != 500 {
		t.Errorf("unexpected video: %+v", got[0])
	}
}

func TestSaveChannelVideosUpdatesURLOnRefresh(t *testing.T) {
	db := newTestDB(t)

	first := []domain.Video{{ID: "v1", Title: "T1", Channel: "C", ChannelID: "ch1", UploadDate: "20240101", URL: "https://old.example.com/v1"}}
	if err := db.SaveChannelVideos(context.Background(), "ch1", first); err != nil {
		t.Fatalf("SaveChannelVideos first: %v", err)
	}

	second := []domain.Video{{ID: "v1", Title: "T1", Channel: "C", ChannelID: "ch1", UploadDate: "20240101", URL: "https://new.example.com/v1"}}
	if err := db.SaveChannelVideos(context.Background(), "ch1", second); err != nil {
		t.Fatalf("SaveChannelVideos second: %v", err)
	}

	got, err := db.GetChannelVideos(context.Background(), "ch1")
	if err != nil {
		t.Fatalf("GetChannelVideos: %v", err)
	}
	if len(got) != 1 || got[0].URL != "https://new.example.com/v1" {
		t.Errorf("SaveChannelVideos did not refresh URL: got %+v", got)
	}
}

func TestGetAllChannelVideosOrdersNewestFirst(t *testing.T) {
	db := newTestDB(t)

	if err := db.SaveChannelVideos(context.Background(), "ch1", []domain.Video{{ID: "old", Title: "Old", ChannelID: "ch1", UploadDate: "20240101"}}); err != nil {
		t.Fatalf("SaveChannelVideos ch1: %v", err)
	}
	if err := db.SaveChannelVideos(context.Background(), "ch2", []domain.Video{{ID: "new", Title: "New", ChannelID: "ch2", UploadDate: "20240102"}}); err != nil {
		t.Fatalf("SaveChannelVideos ch2: %v", err)
	}

	got, err := db.GetAllChannelVideos(context.Background(), []string{"ch1", "ch2"})
	if err != nil {
		t.Fatalf("GetAllChannelVideos: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetAllChannelVideos len = %d, want 2", len(got))
	}
	if got[0].ID != "new" || got[1].ID != "old" {
		t.Errorf("GetAllChannelVideos order: got [%s %s], want [new old]", got[0].ID, got[1].ID)
	}
}

// ── GetChannelLatestAll (M-8: deterministic same-day tiebreak) ───────────────

func TestGetChannelLatestAllPicksLatestDate(t *testing.T) {
	db := newTestDB(t)

	videos := []domain.Video{
		{ID: "early", Title: "Early", ChannelID: "ch1", UploadDate: "20240101"},
		{ID: "late", Title: "Late", ChannelID: "ch1", UploadDate: "20240201"},
	}
	if err := db.SaveChannelVideos(context.Background(), "ch1", videos); err != nil {
		t.Fatalf("SaveChannelVideos: %v", err)
	}

	latest, err := db.GetChannelLatestAll(context.Background())
	if err != nil {
		t.Fatalf("GetChannelLatestAll: %v", err)
	}
	got, ok := latest["ch1"]
	if !ok {
		t.Fatal("GetChannelLatestAll: ch1 missing")
	}
	if got.ID != "late" {
		t.Errorf("GetChannelLatestAll = %q, want %q (later upload_date)", got.ID, "late")
	}
}

// TestGetChannelLatestAllSameDayDeterministic guards M-8: a bare MAX()+GROUP BY
// self-join let SQLite pick an arbitrary row among same-day uploads, so which
// video "won" could flicker between calls. Repeated calls must agree.
func TestGetChannelLatestAllSameDayDeterministic(t *testing.T) {
	db := newTestDB(t)

	videos := []domain.Video{
		{ID: "vid_a", Title: "A", ChannelID: "ch1", UploadDate: "20240101"},
		{ID: "vid_b", Title: "B", ChannelID: "ch1", UploadDate: "20240101"},
	}
	if err := db.SaveChannelVideos(context.Background(), "ch1", videos); err != nil {
		t.Fatalf("SaveChannelVideos: %v", err)
	}

	var want string
	for i := 0; i < 5; i++ {
		latest, err := db.GetChannelLatestAll(context.Background())
		if err != nil {
			t.Fatalf("GetChannelLatestAll run %d: %v", i, err)
		}
		got, ok := latest["ch1"]
		if !ok {
			t.Fatalf("GetChannelLatestAll run %d: ch1 missing", i)
		}
		if i == 0 {
			want = got.ID
			continue
		}
		if got.ID != want {
			t.Errorf("GetChannelLatestAll run %d: got %q, want %q (nondeterministic same-day tiebreak)", i, got.ID, want)
		}
	}
}

func TestGetChannelLatestAllMultipleChannels(t *testing.T) {
	db := newTestDB(t)

	if err := db.SaveChannelVideos(context.Background(), "ch1", []domain.Video{{ID: "a1", ChannelID: "ch1", UploadDate: "20240101"}}); err != nil {
		t.Fatalf("SaveChannelVideos ch1: %v", err)
	}
	if err := db.SaveChannelVideos(context.Background(), "ch2", []domain.Video{{ID: "b1", ChannelID: "ch2", UploadDate: "20240102"}}); err != nil {
		t.Fatalf("SaveChannelVideos ch2: %v", err)
	}

	latest, err := db.GetChannelLatestAll(context.Background())
	if err != nil {
		t.Fatalf("GetChannelLatestAll: %v", err)
	}
	if len(latest) != 2 {
		t.Fatalf("GetChannelLatestAll len = %d, want 2", len(latest))
	}
	if latest["ch1"].ID != "a1" || latest["ch2"].ID != "b1" {
		t.Errorf("GetChannelLatestAll: got %+v", latest)
	}
}

// ── SaveSubscribedChannels ────────────────────────────────────────────────────

func TestSaveSubscribedChannelsPreservesAliasAndTags(t *testing.T) {
	db := newTestDB(t)

	if err := db.AddSubscribedChannel(context.Background(), domain.Channel{ID: "ch1", Name: "Alpha", URL: "u1"}); err != nil {
		t.Fatalf("AddSubscribedChannel: %v", err)
	}
	if err := db.SetChannelAlias(context.Background(), "ch1", "my alias"); err != nil {
		t.Fatalf("SetChannelAlias: %v", err)
	}
	if err := db.SetChannelTags(context.Background(), "ch1", []string{"tech", "news"}); err != nil {
		t.Fatalf("SetChannelTags: %v", err)
	}

	if err := db.SaveSubscribedChannels(context.Background(), []domain.Channel{{ID: "ch1", Name: "Alpha Renamed", URL: "u1", Subscribers: 100}}); err != nil {
		t.Fatalf("SaveSubscribedChannels: %v", err)
	}

	chans, err := db.GetSubscribedChannels(context.Background())
	if err != nil {
		t.Fatalf("GetSubscribedChannels: %v", err)
	}
	if len(chans) != 1 {
		t.Fatalf("GetSubscribedChannels len = %d, want 1", len(chans))
	}
	ch := chans[0]
	if ch.Alias != "my alias" {
		t.Errorf("Alias = %q, want preserved %q", ch.Alias, "my alias")
	}
	if len(ch.Tags) != 2 || ch.Tags[0] != "tech" {
		t.Errorf("Tags = %v, want preserved [tech news]", ch.Tags)
	}
	if ch.Name != "Alpha Renamed" {
		t.Errorf("Name = %q, want updated to %q", ch.Name, "Alpha Renamed")
	}
	if ch.Subscribers != 100 {
		t.Errorf("Subscribers = %d, want updated to 100", ch.Subscribers)
	}
}

func TestSaveSubscribedChannelsRemovesUnsubscribed(t *testing.T) {
	db := newTestDB(t)

	if err := db.AddSubscribedChannel(context.Background(), domain.Channel{ID: "ch1", Name: "A"}); err != nil {
		t.Fatalf("AddSubscribedChannel ch1: %v", err)
	}
	if err := db.AddSubscribedChannel(context.Background(), domain.Channel{ID: "ch2", Name: "B"}); err != nil {
		t.Fatalf("AddSubscribedChannel ch2: %v", err)
	}

	if err := db.SaveSubscribedChannels(context.Background(), []domain.Channel{{ID: "ch1", Name: "A"}}); err != nil {
		t.Fatalf("SaveSubscribedChannels: %v", err)
	}

	chans, err := db.GetSubscribedChannels(context.Background())
	if err != nil {
		t.Fatalf("GetSubscribedChannels: %v", err)
	}
	if len(chans) != 1 || chans[0].ID != "ch1" {
		t.Errorf("GetSubscribedChannels = %+v, want only ch1", chans)
	}
}

func TestSaveSubscribedChannelsPreservesLocalOnly(t *testing.T) {
	db := newTestDB(t)

	if err := db.AddSubscribedChannel(context.Background(), domain.Channel{ID: "local1", Name: "Local", IsLocal: true}); err != nil {
		t.Fatalf("AddSubscribedChannel local1: %v", err)
	}
	if err := db.AddSubscribedChannel(context.Background(), domain.Channel{ID: "ch1", Name: "A"}); err != nil {
		t.Fatalf("AddSubscribedChannel ch1: %v", err)
	}

	// A fresh YT fetch that mentions neither: local1 must survive (is_local=1),
	// ch1 must be removed (YT-managed, no longer present).
	if err := db.SaveSubscribedChannels(context.Background(), []domain.Channel{{ID: "ch2", Name: "B"}}); err != nil {
		t.Fatalf("SaveSubscribedChannels: %v", err)
	}

	chans, err := db.GetSubscribedChannels(context.Background())
	if err != nil {
		t.Fatalf("GetSubscribedChannels: %v", err)
	}
	ids := make(map[string]bool, len(chans))
	for _, ch := range chans {
		ids[ch.ID] = true
	}
	if !ids["local1"] {
		t.Error("local-only channel was deleted")
	}
	if ids["ch1"] {
		t.Error("unsubscribed YT channel should have been removed")
	}
	if !ids["ch2"] {
		t.Error("newly subscribed channel missing")
	}
}

// TestSaveSubscribedChannelsEmptyListIsNoop documents the L-13 policy: an empty
// list means "nothing to sync" (likely a transient fetch failure), not "delete
// every subscription."
func TestSaveSubscribedChannelsEmptyListIsNoop(t *testing.T) {
	db := newTestDB(t)

	if err := db.AddSubscribedChannel(context.Background(), domain.Channel{ID: "ch1", Name: "A"}); err != nil {
		t.Fatalf("AddSubscribedChannel: %v", err)
	}

	if err := db.SaveSubscribedChannels(context.Background(), nil); err != nil {
		t.Fatalf("SaveSubscribedChannels(nil): %v", err)
	}

	chans, err := db.GetSubscribedChannels(context.Background())
	if err != nil {
		t.Fatalf("GetSubscribedChannels: %v", err)
	}
	if len(chans) != 1 || chans[0].ID != "ch1" {
		t.Errorf("SaveSubscribedChannels(nil) should be a no-op, got %+v", chans)
	}
}

// ── ChannelHideStats ──────────────────────────────────────────────────────────

func TestChannelHideStats(t *testing.T) {
	db := newTestDB(t)
	upsertTestVideo(t, db, "v1") // channel_id = "ch-v1" (see upsertTestVideo)

	if err := db.HideRecVideo(context.Background(), "v1"); err != nil {
		t.Fatalf("HideRecVideo: %v", err)
	}
	if err := db.AddHistory(context.Background(), "v1", "playVideo", ""); err != nil {
		t.Fatalf("AddHistory: %v", err)
	}

	hidden, played, err := db.ChannelHideStats(context.Background(), "ch-v1")
	if err != nil {
		t.Fatalf("ChannelHideStats: %v", err)
	}
	if hidden != 1 {
		t.Errorf("hidden = %d, want 1", hidden)
	}
	if played != 1 {
		t.Errorf("played = %d, want 1", played)
	}
}

// TestSetChannelFetchOffset verifies the deep-crawl resume cursor is independent
// of the latest-N refresh stamp — the crux of the backfill fix: a latest-N
// refresh must NOT touch fetched_videos (leaving the channel "never crawled"),
// and the offset round-trips as both a mid-crawl resume position and the
// fully-crawled sentinel.
func TestSetChannelFetchOffset(t *testing.T) {
	db := newTestDB(t)
	if err := db.AddSubscribedChannel(context.Background(), domain.Channel{ID: "ch1", Name: "A", URL: "u1"}); err != nil {
		t.Fatalf("AddSubscribedChannel: %v", err)
	}

	getCh := func() domain.Channel {
		t.Helper()
		chans, err := db.GetSubscribedChannels(context.Background())
		if err != nil {
			t.Fatalf("GetSubscribedChannels: %v", err)
		}
		for _, c := range chans {
			if c.ID == "ch1" {
				return c
			}
		}
		t.Fatal("ch1 not found")
		return domain.Channel{}
	}

	if ch := getCh(); ch.FetchedVideos != 0 || ch.FullyCrawled() {
		t.Fatalf("fresh channel FetchedVideos = %d, want 0 (never crawled)", ch.FetchedVideos)
	}

	// A latest-N refresh stamps videos_refreshed_at but must leave fetched_videos at 0.
	if err := db.TouchChannelVideosRefreshed(context.Background(), "ch1"); err != nil {
		t.Fatalf("TouchChannelVideosRefreshed: %v", err)
	}
	if ch := getCh(); ch.VideosRefreshedAt == 0 || ch.FetchedVideos != 0 {
		t.Fatalf("after latest-N: VideosRefreshedAt=%d FetchedVideos=%d, want refreshed>0, fetched=0",
			ch.VideosRefreshedAt, ch.FetchedVideos)
	}

	// A paused mid-crawl records its resume offset.
	if err := db.SetChannelFetchOffset(context.Background(), "ch1", 200); err != nil {
		t.Fatalf("SetChannelFetchOffset: %v", err)
	}
	if ch := getCh(); ch.FetchedVideos != 200 || ch.FullyCrawled() || ch.ResumeOffset() != 200 {
		t.Fatalf("after pause: FetchedVideos=%d FullyCrawled=%v ResumeOffset=%d, want 200/false/200",
			ch.FetchedVideos, ch.FullyCrawled(), ch.ResumeOffset())
	}

	// Completion stores the sentinel; FullyCrawled flips and ResumeOffset resets.
	if err := db.SetChannelFetchOffset(context.Background(), "ch1", domain.FetchOffsetComplete); err != nil {
		t.Fatalf("SetChannelFetchOffset complete: %v", err)
	}
	if ch := getCh(); !ch.FullyCrawled() || ch.ResumeOffset() != 0 {
		t.Errorf("after complete: FullyCrawled=%v ResumeOffset=%d, want true/0", ch.FullyCrawled(), ch.ResumeOffset())
	}
}
