package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/backend/service"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
)

// fakePortabilityRepo is a hand-seeded PortabilityRepo for exercising bundle
// assembly without a real DB.
type fakePortabilityRepo struct {
	channels     []domain.Channel
	blockedNames []string
	playlists    []domain.Playlist
	playlistVids map[int64][]domain.Video
	watchLater   []domain.WatchLaterEntry
	ytPlaylists  []domain.YTPlaylist
	history      []domain.HistoryEntry
	positions    map[string]int64
}

func (f *fakePortabilityRepo) AllChannels(ctx context.Context) ([]domain.Channel, error) {
	return f.channels, nil
}
func (f *fakePortabilityRepo) Blocklist(ctx context.Context) ([]string, []string, error) {
	return nil, f.blockedNames, nil
}
func (f *fakePortabilityRepo) Playlists(ctx context.Context) ([]domain.Playlist, error) {
	return f.playlists, nil
}
func (f *fakePortabilityRepo) PlaylistVideos(ctx context.Context, id int64) ([]domain.Video, error) {
	return f.playlistVids[id], nil
}
func (f *fakePortabilityRepo) WatchLater(ctx context.Context) ([]domain.WatchLaterEntry, error) {
	return f.watchLater, nil
}
func (f *fakePortabilityRepo) GetYTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error) {
	return f.ytPlaylists, nil
}
func (f *fakePortabilityRepo) History(context.Context, int) ([]domain.HistoryEntry, error) {
	return f.history, nil
}
func (f *fakePortabilityRepo) AllVideoPositions(ctx context.Context) (map[string]int64, error) {
	return f.positions, nil
}

func seededRepo() *fakePortabilityRepo {
	return &fakePortabilityRepo{
		channels: []domain.Channel{
			{ID: "yt1", Name: "YT Chan", URL: "u1", Alias: "a1", Tags: []string{"news"}, State: domain.SubYT},
			{ID: "loc1", Name: "Local Chan", State: domain.SubLocal},
			{ID: "blk1", Name: "Blocked Chan", State: domain.SubNone, Blocked: true},
			{ID: "none1", Name: "Annotated", Tags: []string{"tech"}, State: domain.SubNone},
		},
		blockedNames: []string{"Spammer"},
		playlists:    []domain.Playlist{{ID: 1, Name: "Favorites"}, {ID: 2, Name: "Empty"}},
		playlistVids: map[int64][]domain.Video{
			1: {
				{ID: "v1", Title: "One", Channel: "C", ChannelID: "yt1", Duration: 60},
				{ID: "v2", Title: "Two"},
			},
			2: {},
		},
		watchLater: []domain.WatchLaterEntry{
			{VideoID: "wl1", Title: "Later", Channel: "C", URL: "wlurl"},
		},
		ytPlaylists: []domain.YTPlaylist{{ID: "PL1", Title: "My YT PL"}},
		history: []domain.HistoryEntry{
			{VideoID: "v1", Title: "One", EventType: "playVideo", Timestamp: time.Unix(1000, 0)},
			{VideoID: "", EventType: "search", Details: "golang", Timestamp: time.Unix(2000, 0)},
		},
		positions: map[string]int64{"v1": 42000},
	}
}

func exportSeeded(t *testing.T, watchData bool) portability.Bundle {
	t.Helper()
	b, err := service.NewPortabilityService(seededRepo(), nil).
		Export(context.Background(), portability.ExportOptions{IncludeWatchData: watchData})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	return b
}

func TestExportChannelsAndBlocklist(t *testing.T) {
	b := exportSeeded(t, false)

	if b.SchemaVersion != portability.SchemaVersion {
		t.Errorf("SchemaVersion: want %d, got %d", portability.SchemaVersion, b.SchemaVersion)
	}
	if len(b.Channels) != 4 {
		t.Fatalf("Channels: want 4, got %d", len(b.Channels))
	}
	byID := map[string]portability.ChannelExport{}
	for _, c := range b.Channels {
		byID[c.ChannelID] = c
	}
	if blk := byID["blk1"]; !blk.Blocked || blk.SubscriptionState != string(domain.SubNone) {
		t.Errorf("blocked channel: want blocked+none, got %+v", blk)
	}
	if yt := byID["yt1"]; yt.SubscriptionState != string(domain.SubYT) || len(yt.Tags) != 1 || yt.Alias != "a1" {
		t.Errorf("yt channel export mismatch: %+v", yt)
	}
	if len(b.BlockedNames) != 1 || b.BlockedNames[0] != "Spammer" {
		t.Errorf("BlockedNames: want [Spammer], got %v", b.BlockedNames)
	}
}

func TestExportPlaylistsAndVideos(t *testing.T) {
	b := exportSeeded(t, false)

	// Playlists: names + ordered refs; empty playlist still present.
	if len(b.Playlists) != 2 {
		t.Fatalf("Playlists: want 2, got %d", len(b.Playlists))
	}
	if b.Playlists[0].Name != "Favorites" || len(b.Playlists[0].VideoIDs) != 2 ||
		b.Playlists[0].VideoIDs[0] != "v1" || b.Playlists[0].VideoIDs[1] != "v2" {
		t.Errorf("Favorites playlist refs wrong: %+v", b.Playlists[0])
	}
	// Videos: deduplicated metadata for playlist refs.
	if len(b.Videos) != 2 {
		t.Fatalf("Videos: want 2 (v1,v2), got %d", len(b.Videos))
	}
	var v1 portability.VideoExport
	for _, v := range b.Videos {
		if v.ID == "v1" {
			v1 = v
		}
	}
	if v1.Title != "One" || v1.Duration != 60 || v1.ChannelID != "yt1" {
		t.Errorf("v1 metadata mismatch: %+v", v1)
	}
}

func TestExportRefsAndWatchDataGating(t *testing.T) {
	b := exportSeeded(t, false)

	if len(b.WatchLater) != 1 || b.WatchLater[0].VideoID != "wl1" || b.WatchLater[0].URL != "wlurl" {
		t.Errorf("WatchLater mismatch: %+v", b.WatchLater)
	}
	if len(b.YTPlaylists) != 1 || b.YTPlaylists[0].ID != "PL1" || b.YTPlaylists[0].Title != "My YT PL" {
		t.Errorf("YTPlaylists mismatch: %+v", b.YTPlaylists)
	}
	// Watch data must be absent when the flag is off.
	if len(b.History) != 0 || len(b.Positions) != 0 {
		t.Errorf("watch data leaked with flag off: history=%d positions=%d", len(b.History), len(b.Positions))
	}
}

func TestExportWatchDataOptIn(t *testing.T) {
	b := exportSeeded(t, true)

	if len(b.History) != 2 {
		t.Fatalf("History: want 2, got %d", len(b.History))
	}
	// Search event: no video id, timestamp carried as unix seconds.
	var search portability.HistoryExport
	for _, h := range b.History {
		if h.EventType == "search" {
			search = h
		}
	}
	if search.Details != "golang" || search.Timestamp != 2000 {
		t.Errorf("search history mismatch: %+v", search)
	}
	if len(b.Positions) != 1 || b.Positions[0].VideoID != "v1" || b.Positions[0].PositionMs != 42000 {
		t.Errorf("Positions mismatch: %+v", b.Positions)
	}
}

func TestExportVideoDedup(t *testing.T) {
	repo := seededRepo()
	// v1 appears in two playlists; must be exported once.
	repo.playlists = append(repo.playlists, domain.Playlist{ID: 3, Name: "Dupes"})
	repo.playlistVids[3] = []domain.Video{{ID: "v1", Title: "One"}}

	b, err := service.NewPortabilityService(repo, nil).Export(context.Background(), portability.ExportOptions{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	count := 0
	for _, v := range b.Videos {
		if v.ID == "v1" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("v1 should appear once in Videos, got %d", count)
	}
}
