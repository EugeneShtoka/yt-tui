package api_test

import (
	"context"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/procexec"
)

// watchLaterLocalIDs returns the video ids in the reserved local "Watch Later"
// playlist, or an empty slice if the playlist doesn't exist yet.
func watchLaterLocalIDs(t *testing.T, p *api.InProc) []string {
	t.Helper()
	ctx := context.Background()
	pls, err := p.LocalPlaylists(ctx)
	if err != nil {
		t.Fatalf("LocalPlaylists: %v", err)
	}
	for _, pl := range pls {
		if pl.Name == domain.WatchLaterPlaylistName {
			ids, err := p.PlaylistVideoIDs(ctx, pl.ID)
			if err != nil {
				t.Fatalf("PlaylistVideoIDs: %v", err)
			}
			return ids
		}
	}
	return nil
}

// With no YT client initialized, Watch Later falls back to the reserved local
// playlist: add materializes it there, remove takes it back out.
func TestInProcWatchLaterOfflineFallback(t *testing.T) {
	p, _ := newInProc(t, procexec.OS{})
	ctx := context.Background()

	if err := p.AddToWatchLater(ctx, domain.Video{ID: "v1", Title: "V1", URL: "u"}); err != nil {
		t.Fatalf("AddToWatchLater: %v", err)
	}
	if ids := watchLaterLocalIDs(t, p); len(ids) != 1 || ids[0] != "v1" {
		t.Fatalf("after add, local WL = %v, want [v1]", ids)
	}

	if err := p.RemoveFromWatchLater(ctx, "v1"); err != nil {
		t.Fatalf("RemoveFromWatchLater: %v", err)
	}
	if ids := watchLaterLocalIDs(t, p); len(ids) != 0 {
		t.Fatalf("after remove, local WL = %v, want empty", ids)
	}
}

// SaveVideoPosition auto-removes a video from Watch Later once it crosses the
// configured watched-percent threshold, and leaves it alone below the threshold.
func TestInProcWatchLaterAutoRemoveOnWatched(t *testing.T) {
	database, err := db.New(t.TempDir(), false, 90)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	cfg := &config.Config{}
	cfg.WatchLaterAutoRemovePercent = 90
	dl := downloader.NewWithRunner(cfg, database, procexec.OS{})
	p := api.NewInProc(database, nil, dl, cfg)
	ctx := context.Background()

	// 300s video queued in Watch Later (offline → local playlist).
	if err := p.AddToWatchLater(ctx, domain.Video{ID: "v1", Title: "V1", URL: "u", Duration: 300}); err != nil {
		t.Fatalf("AddToWatchLater: %v", err)
	}

	// 200s / 300s ≈ 66% < 90% → stays.
	if err := p.SaveVideoPosition(ctx, "v1", 200_000); err != nil {
		t.Fatalf("SaveVideoPosition: %v", err)
	}
	if ids := watchLaterLocalIDs(t, p); len(ids) != 1 {
		t.Fatalf("below threshold, local WL = %v, want [v1]", ids)
	}

	// 280s / 300s ≈ 93% ≥ 90% → auto-removed.
	if err := p.SaveVideoPosition(ctx, "v1", 280_000); err != nil {
		t.Fatalf("SaveVideoPosition: %v", err)
	}
	if ids := watchLaterLocalIDs(t, p); len(ids) != 0 {
		t.Fatalf("above threshold, local WL = %v, want empty", ids)
	}
}
