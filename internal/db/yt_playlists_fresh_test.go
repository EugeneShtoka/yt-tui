package db

import (
	"context"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

func TestYTPlaylistsFresh(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// No rows → nothing has ever synced → stale.
	if fresh, err := db.YTPlaylistsFresh(ctx, 60); err != nil || fresh {
		t.Fatalf("empty cache: fresh=%v err=%v, want false", fresh, err)
	}

	// SaveYTPlaylists stamps updated_at = now → within any positive window.
	if err := db.SaveYTPlaylists(ctx, []domain.YTPlaylist{{ID: "a", Title: "A"}}); err != nil {
		t.Fatalf("SaveYTPlaylists: %v", err)
	}
	if fresh, err := db.YTPlaylistsFresh(ctx, 60); err != nil || !fresh {
		t.Fatalf("just synced: fresh=%v err=%v, want true", fresh, err)
	}

	// withinMinutes <= 0 disables the throttle (treated as always stale).
	if fresh, err := db.YTPlaylistsFresh(ctx, 0); err != nil || fresh {
		t.Fatalf("window<=0: fresh=%v err=%v, want false", fresh, err)
	}

	// Age the rows past the window → stale.
	if _, err := db.sql.ExecContext(ctx, `UPDATE collections SET updated_at = datetime('now','-120 minutes') WHERE kind='yt'`); err != nil {
		t.Fatalf("age rows: %v", err)
	}
	if fresh, err := db.YTPlaylistsFresh(ctx, 60); err != nil || fresh {
		t.Fatalf("2h-old vs 60m window: fresh=%v err=%v, want false", fresh, err)
	}
}
