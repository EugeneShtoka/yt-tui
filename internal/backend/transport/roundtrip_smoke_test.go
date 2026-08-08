package transport_test

import (
	"context"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// TestRoundTripForwardingHandlers drives every thin forwarding handler over the
// real Connect boundary against a no-op backend, asserting the success path
// works end to end (request encode → handler → protoconv → response decode).
// It complements the field-preserving read/write round-trip tests by covering
// the many one-line verbs cheaply. Streaming Events is exercised elsewhere.
func TestRoundTripForwardingHandlers(t *testing.T) {
	r := newRemote(t, apitest.NopBackend{}, "")
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
	}{
		// ── ChannelService ──────────────────────────────────────────────────
		{"GetSubscribedChannels", func() error { _, e := r.GetSubscribedChannels(ctx); return e }},
		{"AllChannels", func() error { _, e := r.AllChannels(ctx); return e }},
		{"BlockedChannels", func() error { _, e := r.BlockedChannels(ctx); return e }},
		{"ChannelLatestN", func() error { _, e := r.ChannelLatestN(ctx, "https://c", "c1", 3); return e }},
		{"GetChannelVideos", func() error { _, e := r.GetChannelVideos(ctx, "c1"); return e }},
		{"GetAllChannelVideos", func() error { _, e := r.GetAllChannelVideos(ctx, []string{"c1", "c2"}); return e }},
		{"GetChannelLatestAll", func() error { _, e := r.GetChannelLatestAll(ctx); return e }},
		{"ChannelHideStats", func() error { _, _, e := r.ChannelHideStats(ctx, "c1"); return e }},
		{"Search", func() error { _, _, e := r.Search(ctx, "query"); return e }},
		{"BlockChannel", func() error { return r.BlockChannel(ctx, domain.Channel{ID: "c1"}) }},
		{"UnblockChannel", func() error { return r.UnblockChannel(ctx, "c1") }},
		{"AddSubscribedChannel", func() error { return r.AddSubscribedChannel(ctx, domain.Channel{ID: "c1"}) }},
		{"RemoveSubscribedChannel", func() error { return r.RemoveSubscribedChannel(ctx, "c1") }},
		{"DeleteChannelVideos", func() error { return r.DeleteChannelVideos(ctx, "c1") }},
		{"SetChannelAlias", func() error { return r.SetChannelAlias(ctx, "c1", "Alias") }},
		{"Subscribe", func() error { return r.Subscribe(ctx, domain.Channel{ID: "c1"}) }},
		{"Unsubscribe", func() error { return r.Unsubscribe(ctx, domain.Channel{ID: "c1"}) }},

		// ── VideoService ────────────────────────────────────────────────────
		{"VideoPosition", func() error { _, _, e := r.VideoPosition(ctx, "v1"); return e }},
		{"AllVideoPositions", func() error { _, e := r.AllVideoPositions(ctx); return e }},
		{"SetVideoStatus", func() error { return r.SetVideoStatus(ctx, "v1", domain.StatusWatched) }},
		{"SaveVideoPosition", func() error { return r.SaveVideoPosition(ctx, "v1", 5000) }},
		{"DeleteVideoPosition", func() error { return r.DeleteVideoPosition(ctx, "v1") }},
		{"UpdateLastPosition", func() error { return r.UpdateLastPosition(ctx, "v1", 5000) }},
		{"ClearVideoDetailsCache", func() error { return r.ClearVideoDetailsCache(ctx) }},
		{"DeleteVideoCompletely", func() error { return r.DeleteVideoCompletely(ctx, "v1") }},
		{"GetThumbnail", func() error { _, _, e := r.GetThumbnail(ctx, "v1", "https://img"); return e }},
		{"GetTranscript", func() error { _, _, e := r.GetTranscript(ctx, "v1", "https://v"); return e }},
		{"EligibleThumbnailIDs", func() error { _, e := r.EligibleThumbnailIDs(ctx); return e }},
		{"ResolveSource", func() error { _, e := r.ResolveSource(ctx, "v1", "https://v"); return e }},

		// ── FeedService ─────────────────────────────────────────────────────
		{"GetFeedCache", func() error { _, e := r.GetFeedCache(ctx, "recommended"); return e }},
		{"SaveFeedCache", func() error { return r.SaveFeedCache(ctx, "recommended", nil) }},
		{"PurgeFeedCacheMissingChannelID", func() error { return r.PurgeFeedCacheMissingChannelID(ctx, "recommended") }},
		{"HideRecVideo", func() error { return r.HideRecVideo(ctx, "v1") }},
		{"HiddenRecVideoIDs", func() error { _, e := r.HiddenRecVideoIDs(ctx); return e }},
		{"WatchedVideoIDs", func() error { _, e := r.WatchedVideoIDs(ctx); return e }},
		{"ClearRecommended", func() error { return r.ClearRecommended(ctx) }},

		// ── HistoryService ──────────────────────────────────────────────────
		{"HistoryVideos", func() error { _, e := r.HistoryVideos(ctx, 50); return e }},
		{"VideoHistory", func() error { _, e := r.VideoHistory(ctx, "v1"); return e }},
		{"ActivityLog", func() error { _, e := r.ActivityLog(ctx, 50); return e }},
		{"SearchQueries", func() error { _, e := r.SearchQueries(ctx); return e }},
		{"AddHistory", func() error { return r.AddHistory(ctx, "v1", "streamVideo", "d") }},
		{"DeleteVideoHistory", func() error { return r.DeleteVideoHistory(ctx, "v1") }},
		{"DeleteSearchHistory", func() error { return r.DeleteSearchHistory(ctx, "q") }},
		{"ClearHistory", func() error { return r.ClearHistory(ctx) }},

		// ── PlaylistService ─────────────────────────────────────────────────
		{"LocalPlaylists", func() error { _, e := r.LocalPlaylists(ctx); return e }},
		{"PlaylistVideoIDs", func() error { _, e := r.PlaylistVideoIDs(ctx, 1); return e }},
		{"CreatePlaylist", func() error { _, e := r.CreatePlaylist(ctx, "name"); return e }},
		{"DeletePlaylist", func() error { return r.DeletePlaylist(ctx, 1) }},
		{"AddToPlaylist", func() error { return r.AddToPlaylist(ctx, 1, "v1") }},
		{"RemoveFromPlaylist", func() error { return r.RemoveFromPlaylist(ctx, 1, "v1") }},
		{"GetYTPlaylists", func() error { _, e := r.GetYTPlaylists(ctx); return e }},
		{"GetYTPlaylistVideos", func() error { _, e := r.GetYTPlaylistVideos(ctx, "pl1"); return e }},
		{"SaveYTPlaylists", func() error { return r.SaveYTPlaylists(ctx, nil) }},
		{"SaveYTPlaylistVideos", func() error { return r.SaveYTPlaylistVideos(ctx, "pl1", nil) }},

		// ── LibraryService ──────────────────────────────────────────────────
		{"HasLocalVideo", func() error { _, _, e := r.HasLocalVideo(ctx, "v1"); return e }},
		{"AddLocalVideo", func() error { return r.AddLocalVideo(ctx, domain.LocalVideo{ID: "v1"}) }},
		{"DeleteLocalVideo", func() error { return r.DeleteLocalVideo(ctx, "v1") }},
		{"DeleteAllLocalFiles", func() error { _, e := r.DeleteAllLocalFiles(ctx); return e }},

		// ── DownloadService ─────────────────────────────────────────────────
		{"Enqueue", func() error { return r.Enqueue(ctx, domain.Video{ID: "v1"}, false) }},
		{"CancelDownload", func() error { return r.CancelDownload(ctx, "v1") }},
		{"DownloadItems", func() error { _, e := r.DownloadItems(ctx); return e }},
		{"ClearDownloads", func() error { return r.ClearDownloads(ctx) }},

		// ── StatusService ───────────────────────────────────────────────────
		{"CheckAvailability", func() error { _, e := r.CheckAvailability(ctx); return e }},
		{"Capabilities", func() error { _, e := r.Capabilities(ctx); return e }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.call(); err != nil {
				t.Errorf("%s: unexpected error over the transport boundary: %v", c.name, err)
			}
		})
	}
}
