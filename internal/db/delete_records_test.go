package db

import (
	"context"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// TestSchemaVersionStamped verifies migrate() applies every embedded migration
// and stamps the database's user_version to the latest version, giving the
// migration runner a real version to branch on.
func TestSchemaVersionStamped(t *testing.T) {
	db := newTestDB(t)
	got, err := db.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	want, err := latestSchemaVersion()
	if err != nil {
		t.Fatalf("latestSchemaVersion: %v", err)
	}
	if got != want {
		t.Errorf("user_version = %d, want %d", got, want)
	}
}

// TestDeleteVideoRecordsAtomic verifies the transactional multi-store delete
// (M-3): the local-file row, history, and saved position all go, the parent
// videos row stays, and — the point of the transaction — nothing is left half
// deleted.
func TestDeleteVideoRecordsAtomic(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	const id = "vid1"

	upsertTestVideo(t, db, id)
	if err := db.AddLocalVideo(ctx, domain.LocalVideo{
		ID: id, Title: "Test Title", Channel: "Test Channel",
		FilePath: "/tmp/vid1.mkv", DownloadType: "video",
		DownloadedAt: time.Now(), Status: domain.StatusNew,
	}); err != nil {
		t.Fatalf("AddLocalVideo: %v", err)
	}
	if err := db.AddHistory(ctx, id, "play", ""); err != nil {
		t.Fatalf("AddHistory: %v", err)
	}
	if err := db.SaveVideoPosition(ctx, id, 4200); err != nil {
		t.Fatalf("SaveVideoPosition: %v", err)
	}

	if err := db.DeleteVideoRecords(ctx, id); err != nil {
		t.Fatalf("DeleteVideoRecords: %v", err)
	}

	if _, ok, err := db.HasLocalVideo(ctx, id); err != nil || ok {
		t.Errorf("local_videos row survived: ok=%v err=%v", ok, err)
	}
	if events, err := db.VideoHistory(ctx, id); err != nil || len(events) != 0 {
		t.Errorf("history survived: %d events, err=%v", len(events), err)
	}
	if _, ok, err := db.VideoPosition(ctx, id); err != nil || ok {
		t.Errorf("video_positions row survived: ok=%v err=%v", ok, err)
	}
	// The parent videos row is deliberately preserved (only the per-video
	// records are cleared, not the canonical metadata).
	var n int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM videos WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count videos: %v", err)
	}
	if n != 1 {
		t.Errorf("parent videos row = %d, want 1 (must be preserved)", n)
	}
}
