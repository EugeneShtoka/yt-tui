package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// addLocal inserts a video + local_videos row pointing at path.
func addLocal(t *testing.T, db *DB, id, path string) {
	t.Helper()
	upsertTestVideo(t, db, id)
	if err := db.AddLocalVideo(context.Background(), domain.LocalVideo{
		ID: id, FilePath: path, DownloadType: "video",
		DownloadedAt: time.Now(), Status: domain.StatusNew,
	}); err != nil {
		t.Fatalf("AddLocalVideo(%q): %v", id, err)
	}
}

// deleteEventDetails returns the details of the video's latest delete event, or
// "" if it has none.
func deleteEventDetails(t *testing.T, db *DB, id string) string {
	t.Helper()
	events, err := db.VideoHistory(context.Background(), id)
	if err != nil {
		t.Fatalf("VideoHistory(%q): %v", id, err)
	}
	for i := range events {
		if events[i].EventType == evtDelete {
			return events[i].Details
		}
	}
	return ""
}

// A local row whose file is present on disk is healthy: kept, no delete event.
func TestReconcileKeepsPresentFile(t *testing.T) {
	db := newTestDB(t)
	f := filepath.Join(t.TempDir(), "present.mkv")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	addLocal(t, db, "present", f)

	if err := db.reconcileDownloads(context.Background()); err != nil {
		t.Fatalf("reconcileDownloads: %v", err)
	}

	if _, ok, err := db.HasLocalVideo(context.Background(), "present"); err != nil || !ok {
		t.Errorf("present-file row was pruned (ok=%v, err=%v)", ok, err)
	}
	if d := deleteEventDetails(t, db, "present"); d != "" {
		t.Errorf("healthy row got a delete event: %q", d)
	}
}

// Phase 11: reconcile backfills file_size for a present row that still records
// zero (written before the size column existed), reusing its stat pass. A row
// that already carries a size is left untouched (no clobber).
func TestReconcileBackfillsFileSize(t *testing.T) {
	db := newTestDB(t)
	dir := t.TempDir()

	content := []byte("hello world")
	f := filepath.Join(dir, "sized.mkv")
	if err := os.WriteFile(f, content, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	addLocal(t, db, "sized", f) // AddLocalVideo leaves FileSize == 0

	// A row that already has a size must not be re-stat-clobbered.
	other := filepath.Join(dir, "kept.mkv")
	if err := os.WriteFile(other, []byte("xxxx"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	upsertTestVideo(t, db, "kept")
	if err := db.AddLocalVideo(context.Background(), domain.LocalVideo{
		ID: "kept", FilePath: other, FileSize: 999, DownloadType: "video",
		DownloadedAt: time.Now(), Status: domain.StatusNew,
	}); err != nil {
		t.Fatalf("AddLocalVideo(kept): %v", err)
	}

	if err := db.reconcileDownloads(context.Background()); err != nil {
		t.Fatalf("reconcileDownloads: %v", err)
	}

	lv, ok, err := db.HasLocalVideo(context.Background(), "sized")
	if err != nil || !ok {
		t.Fatalf("HasLocalVideo(sized): ok=%v err=%v", ok, err)
	}
	if lv.FileSize != int64(len(content)) {
		t.Errorf("backfilled FileSize = %d, want %d", lv.FileSize, len(content))
	}

	kept, _, err := db.HasLocalVideo(context.Background(), "kept")
	if err != nil {
		t.Fatalf("HasLocalVideo(kept): %v", err)
	}
	if kept.FileSize != 999 {
		t.Errorf("existing FileSize clobbered: got %d, want 999", kept.FileSize)
	}
}

// A local row whose file is definitively gone (os.IsNotExist) is pruned, with a
// "auto: file missing" delete event closing the lifecycle.
func TestReconcilePrunesMissingFile(t *testing.T) {
	db := newTestDB(t)
	missing := filepath.Join(t.TempDir(), "gone.mkv") // never created
	addLocal(t, db, "gone", missing)

	if err := db.reconcileDownloads(context.Background()); err != nil {
		t.Fatalf("reconcileDownloads: %v", err)
	}

	if _, ok, err := db.HasLocalVideo(context.Background(), "gone"); err != nil || ok {
		t.Errorf("missing-file row was not pruned (ok=%v, err=%v)", ok, err)
	}
	if d := deleteEventDetails(t, db, "gone"); d != "auto: file missing" {
		t.Errorf("delete details = %q, want %q", d, "auto: file missing")
	}
}

// An ambiguous stat error (here ENOTDIR: a path whose parent is a regular file)
// must never prune — an offline disk or permission blip must not wipe the row.
func TestReconcileKeepsRowOnAmbiguousStatError(t *testing.T) {
	db := newTestDB(t)
	parent := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Stat of "afile/child" returns ENOTDIR, which is NOT os.IsNotExist.
	addLocal(t, db, "ambiguous", filepath.Join(parent, "child.mkv"))

	if err := db.reconcileDownloads(context.Background()); err != nil {
		t.Fatalf("reconcileDownloads: %v", err)
	}

	if _, ok, err := db.HasLocalVideo(context.Background(), "ambiguous"); err != nil || !ok {
		t.Errorf("row pruned on ambiguous stat error (ok=%v, err=%v)", ok, err)
	}
	if d := deleteEventDetails(t, db, "ambiguous"); d != "" {
		t.Errorf("ambiguous row got a delete event: %q", d)
	}
}

// History says a video is downloaded (latest download/delete event is a
// download) but there is no local row: close the lifecycle with an
// "auto: orphaned record" delete event. Re-running must not duplicate it.
func TestReconcileClosesOrphanedHistory(t *testing.T) {
	db := newTestDB(t)
	// The videos row exists (a real download event references it); only the
	// local_videos row is absent — the "history says downloaded, no local
	// record" case.
	upsertTestVideo(t, db, "orphan")
	if err := db.AddHistory(context.Background(), "orphan", evtDownloadVideo, ""); err != nil {
		t.Fatalf("AddHistory: %v", err)
	}

	if err := db.reconcileDownloads(context.Background()); err != nil {
		t.Fatalf("reconcileDownloads: %v", err)
	}
	if d := deleteEventDetails(t, db, "orphan"); d != "auto: orphaned record" {
		t.Errorf("delete details = %q, want %q", d, "auto: orphaned record")
	}

	// Idempotent: latest event is now "delete", so a second pass adds nothing.
	if err := db.reconcileDownloads(context.Background()); err != nil {
		t.Fatalf("reconcileDownloads (2nd): %v", err)
	}
	events, err := db.VideoHistory(context.Background(), "orphan")
	if err != nil {
		t.Fatalf("VideoHistory: %v", err)
	}
	deletes := 0
	for i := range events {
		if events[i].EventType == evtDelete {
			deletes++
		}
	}
	if deletes != 1 {
		t.Errorf("orphan has %d delete events after two passes, want 1 (not idempotent)", deletes)
	}
}
