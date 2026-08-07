package api_test

import (
	"context"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/procexec"
)

// DownloadItem's row accessors (client.go) are pure projections.
func TestDownloadItemAccessors(t *testing.T) {
	di := api.DownloadItem{Title: "Vid", Channel: "Chan", Duration: 90, AudioOnly: true}
	if di.GetBaseTitle() != "Vid" {
		t.Errorf("GetBaseTitle = %q, want Vid", di.GetBaseTitle())
	}
	if !di.IsAudio() {
		t.Error("IsAudio = false, want true")
	}
	if di.GetChannelID() != "" {
		t.Errorf("GetChannelID = %q, want empty", di.GetChannelID())
	}
	if di.GetChannelName() != "Chan" {
		t.Errorf("GetChannelName = %q, want Chan", di.GetChannelName())
	}
	if di.GetDurationSecs() != 90 {
		t.Errorf("GetDurationSecs = %d, want 90", di.GetDurationSecs())
	}
}

func hasChannel(chs []domain.Channel, id string) bool {
	for i := range chs {
		if chs[i].ID == id {
			return true
		}
	}
	return false
}

// Channel CRUD round-trips through the InProc adapter (DB-backed; no yt client).
func TestInProcChannelRoundTrip(t *testing.T) {
	p, _ := newInProc(t, procexec.OS{})
	ctx := context.Background()

	ch := domain.Channel{ID: "ch1", Name: "Chan One", URL: "u1", State: domain.SubYT}
	if err := p.AddSubscribedChannel(ctx, ch); err != nil {
		t.Fatalf("AddSubscribedChannel: %v", err)
	}
	if subs, err := p.GetSubscribedChannels(ctx); err != nil || !hasChannel(subs, "ch1") {
		t.Fatalf("GetSubscribedChannels = %v (err %v), want ch1 present", subs, err)
	}

	if err := p.SetChannelAlias(ctx, "ch1", "Alias"); err != nil {
		t.Fatalf("SetChannelAlias: %v", err)
	}
	if err := p.SetChannelTags(ctx, "ch1", []string{"news", "tech"}); err != nil {
		t.Fatalf("SetChannelTags: %v", err)
	}
	all, err := p.AllChannels(ctx)
	if err != nil || !hasChannel(all, "ch1") {
		t.Fatalf("AllChannels = %v (err %v), want ch1", all, err)
	}
	for i := range all {
		if all[i].ID == "ch1" {
			if all[i].Alias != "Alias" {
				t.Errorf("alias = %q, want Alias", all[i].Alias)
			}
			if len(all[i].Tags) != 2 {
				t.Errorf("tags = %v, want 2", all[i].Tags)
			}
		}
	}

	vids := []domain.Video{{ID: "v1", ChannelID: "ch1", URL: "http://x/v1", Title: "V1", UploadDate: "20260101"}}
	if err := p.SaveChannelVideos(ctx, "ch1", vids); err != nil {
		t.Fatalf("SaveChannelVideos: %v", err)
	}
	if got, err := p.GetChannelVideos(ctx, "ch1"); err != nil || len(got) != 1 {
		t.Fatalf("GetChannelVideos = %v (err %v), want 1", got, err)
	}
	if got, err := p.GetAllChannelVideos(ctx, []string{"ch1"}); err != nil || len(got) != 1 {
		t.Fatalf("GetAllChannelVideos = %v (err %v), want 1", got, err)
	}
	if latest, err := p.GetChannelLatestAll(ctx); err != nil || latest["ch1"].ID != "v1" {
		t.Fatalf("GetChannelLatestAll[ch1] = %v (err %v), want v1", latest["ch1"], err)
	}

	if err := p.BlockChannel(ctx, ch); err != nil {
		t.Fatalf("BlockChannel: %v", err)
	}
	if blocked, err := p.BlockedChannels(ctx); err != nil || !hasChannel(blocked, "ch1") {
		t.Fatalf("BlockedChannels = %v (err %v), want ch1", blocked, err)
	}
	if err := p.UnblockChannel(ctx, "ch1"); err != nil {
		t.Fatalf("UnblockChannel: %v", err)
	}
}

// Playlist + watch-later + YT-cache round-trips through the InProc adapter.
func TestInProcPlaylistRoundTrip(t *testing.T) {
	p, _ := newInProc(t, procexec.OS{})
	ctx := context.Background()

	// A video must exist before it can join a playlist.
	if err := p.SaveChannelVideos(ctx, "ch1", []domain.Video{{ID: "v1", ChannelID: "ch1", URL: "u", Title: "V1"}}); err != nil {
		t.Fatalf("SaveChannelVideos: %v", err)
	}

	id, err := p.CreatePlaylist(ctx, "Fav")
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	pls, err := p.LocalPlaylists(ctx)
	if err != nil || len(pls) != 1 || pls[0].Name != "Fav" {
		t.Fatalf("LocalPlaylists = %v (err %v), want [Fav]", pls, err)
	}

	if err := p.AddToPlaylist(ctx, id, "v1"); err != nil {
		t.Fatalf("AddToPlaylist: %v", err)
	}
	if ids, err := p.PlaylistVideoIDs(ctx, id); err != nil || len(ids) != 1 || ids[0] != "v1" {
		t.Fatalf("PlaylistVideoIDs = %v (err %v), want [v1]", ids, err)
	}
	if err := p.RemoveFromPlaylist(ctx, id, "v1"); err != nil {
		t.Fatalf("RemoveFromPlaylist: %v", err)
	}

	if err := p.AddWatchLater(ctx, "wl1", "Title", "Chan", "url"); err != nil {
		t.Fatalf("AddWatchLater: %v", err)
	}
	if wl, err := p.WatchLater(ctx); err != nil || len(wl) != 1 {
		t.Fatalf("WatchLater = %v (err %v), want 1", wl, err)
	}
	if err := p.RemoveWatchLater(ctx, "wl1"); err != nil {
		t.Fatalf("RemoveWatchLater: %v", err)
	}

	if err := p.SaveYTPlaylists(ctx, []domain.YTPlaylist{{ID: "PL1", Title: "YT"}}); err != nil {
		t.Fatalf("SaveYTPlaylists: %v", err)
	}
	if yt, err := p.GetYTPlaylists(ctx); err != nil || len(yt) != 1 || yt[0].ID != "PL1" {
		t.Fatalf("GetYTPlaylists = %v (err %v), want [PL1]", yt, err)
	}

	if err := p.DeletePlaylist(ctx, id); err != nil {
		t.Fatalf("DeletePlaylist: %v", err)
	}
	if pls, err := p.LocalPlaylists(ctx); err != nil || len(pls) != 0 {
		t.Fatalf("LocalPlaylists after delete = %v (err %v), want empty", pls, err)
	}
}

// History + search-history + activity round-trips through the InProc adapter.
func TestInProcHistoryRoundTrip(t *testing.T) {
	p, _ := newInProc(t, procexec.OS{})
	ctx := context.Background()

	// History rows FK to the videos table, so the video must exist first.
	if err := p.SaveChannelVideos(ctx, "ch1", []domain.Video{{ID: "v1", ChannelID: "ch1", URL: "u", Title: "V1"}}); err != nil {
		t.Fatalf("SaveChannelVideos: %v", err)
	}
	if err := p.AddHistory(ctx, "v1", "playVideo", ""); err != nil {
		t.Fatalf("AddHistory: %v", err)
	}
	if h, err := p.History(ctx, 10); err != nil || len(h) == 0 {
		t.Fatalf("History = %v (err %v), want non-empty", h, err)
	}
	if vh, err := p.VideoHistory(ctx, "v1"); err != nil || len(vh) == 0 {
		t.Fatalf("VideoHistory = %v (err %v), want non-empty", vh, err)
	}

	if err := p.AddHistory(ctx, "", "search", "golang"); err != nil {
		t.Fatalf("AddHistory(search): %v", err)
	}
	if q, err := p.SearchQueries(ctx); err != nil || len(q) == 0 || q[0] != "golang" {
		t.Fatalf("SearchQueries = %v (err %v), want [golang]", q, err)
	}
	if err := p.DeleteSearchHistory(ctx, "golang"); err != nil {
		t.Fatalf("DeleteSearchHistory: %v", err)
	}

	if err := p.ClearHistory(ctx); err != nil {
		t.Fatalf("ClearHistory: %v", err)
	}
	if h, err := p.History(ctx, 10); err != nil || len(h) != 0 {
		t.Fatalf("History after clear = %v (err %v), want empty", h, err)
	}
}
