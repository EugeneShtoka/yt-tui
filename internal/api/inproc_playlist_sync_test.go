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

// When the cache is fresh (within refresh_minutes), SyncYTPlaylists must serve it
// straight from the DB and never hit the network. The InProc here is built with a
// nil YT client, so if the throttle failed to short-circuit, the live path would
// panic — no panic + the cached list back is the assertion that no fetch ran.
func TestInProcSyncYTPlaylistsServesCacheWhenFresh(t *testing.T) {
	database, err := db.New(t.TempDir(), false, 90)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	cfg := &config.Config{}
	cfg.RefreshMinutes = 60
	dl := downloader.NewWithRunner(cfg, database, procexec.OS{})
	p := api.NewInProc(database, nil, dl, cfg) // nil YT client on purpose
	ctx := context.Background()

	// Seed a just-synced cache (SaveYTPlaylists stamps updated_at = now).
	if err = database.SaveYTPlaylists(ctx, []domain.YTPlaylist{{ID: "pl1", Title: "One"}}); err != nil {
		t.Fatalf("SaveYTPlaylists: %v", err)
	}

	got, err := p.SyncYTPlaylists(ctx)
	if err != nil {
		t.Fatalf("SyncYTPlaylists: %v", err)
	}
	if len(got) != 1 || got[0].ID != "pl1" {
		t.Fatalf("SyncYTPlaylists = %v, want cached [pl1]", got)
	}
}
