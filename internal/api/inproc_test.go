package api_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/procexec"
)

func newInProc(t *testing.T, runner procexec.Runner) (*api.InProc, *db.DB) {
	t.Helper()
	database, err := db.New(t.TempDir(), false, 90)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	cfg := &config.Config{}
	dl := downloader.NewWithRunner(cfg, database, runner)
	return api.NewInProc(database, nil, dl, cfg), database
}

func addLocalVideo(t *testing.T, database *db.DB, id, filePath string) {
	t.Helper()
	if err := database.UpsertVideo(context.Background(), id, "T-"+id, "Chan", "UC1", 60, 1, "20240101", "http://x/"+id); err != nil {
		t.Fatalf("UpsertVideo: %v", err)
	}
	if err := database.AddLocalVideo(context.Background(), domain.LocalVideo{
		ID: id, Title: "T-" + id, Channel: "Chan", FilePath: filePath,
		DownloadType: "video", DownloadedAt: time.Now(), Status: domain.StatusNew,
	}); err != nil {
		t.Fatalf("AddLocalVideo: %v", err)
	}
}

// ResolveSource returns the on-disk path for a downloaded video, else the
// caller's fallback URL (M-6: the "what is playable" rule).
func TestInProcResolveSource(t *testing.T) {
	p, database := newInProc(t, procexec.OS{})
	ctx := context.Background()

	if src, err := p.ResolveSource(ctx, "missing", "http://fallback"); err != nil || src.URI != "http://fallback" {
		t.Fatalf("no local video: got (%+v, %v), want fallback URL", src, err)
	}

	addLocalVideo(t, database, "local1", "/tmp/local1.mkv")
	src, err := p.ResolveSource(ctx, "local1", "http://fallback")
	if err != nil || src.URI != "/tmp/local1.mkv" {
		t.Fatalf("local video: got (%+v, %v), want the file path", src, err)
	}
}

// ClearDownloads is now a queue-only dismiss (Phase 10): it must NOT touch the
// downloaded file on disk nor the local_videos DB row — those belong to the
// Local tab (DeleteAllLocalFiles). Deleting files lives on that path now.
func TestInProcClearDownloadsLeavesFilesAndRows(t *testing.T) {
	p, database := newInProc(t, procexec.OS{})
	ctx := context.Background()

	f := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(f, []byte("data"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	addLocalVideo(t, database, "dl1", f)

	if err := p.ClearDownloads(ctx); err != nil {
		t.Fatalf("ClearDownloads: %v", err)
	}
	if _, err := os.Stat(f); err != nil {
		t.Errorf("ClearDownloads deleted the file %q — it must leave files alone: %v", f, err)
	}
	if _, ok, err := database.HasLocalVideo(context.Background(), "dl1"); err != nil || !ok {
		t.Errorf("ClearDownloads removed the DB row — it must leave rows alone (ok=%v, err=%v)", ok, err)
	}
}

// DeleteAllLocalFiles is the bulk reclaim-disk action: every file + row is
// removed and a delete history event is written for each (mirroring the
// Local-tab single delete).
func TestInProcDeleteAllLocalFiles(t *testing.T) {
	p, database := newInProc(t, procexec.OS{})
	ctx := context.Background()

	dir := t.TempDir()
	ids := []string{"a1", "a2", "a3"}
	for _, id := range ids {
		f := filepath.Join(dir, id+".mkv")
		if err := os.WriteFile(f, []byte("data"), 0o600); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
		addLocalVideo(t, database, id, f)
	}

	deleted, err := p.DeleteAllLocalFiles(ctx)
	if err != nil {
		t.Fatalf("DeleteAllLocalFiles: %v", err)
	}
	if deleted != len(ids) {
		t.Errorf("deleted = %d, want %d", deleted, len(ids))
	}
	for _, id := range ids {
		if _, err := os.Stat(filepath.Join(dir, id+".mkv")); !os.IsNotExist(err) {
			t.Errorf("file for %s still present: %v", id, err)
		}
		if _, ok, err := database.HasLocalVideo(context.Background(), id); err != nil || ok {
			t.Errorf("row for %s still present (ok=%v, err=%v)", id, ok, err)
		}
		events, err := database.VideoHistory(context.Background(), id)
		if err != nil {
			t.Fatalf("VideoHistory %s: %v", id, err)
		}
		if !hasDeleteEvent(events) {
			t.Errorf("no delete history event recorded for %s", id)
		}
	}
}

func hasDeleteEvent(events []domain.HistoryEntry) bool {
	for i := range events {
		if events[i].EventType == "delete" {
			return true
		}
	}
	return false
}

// DeleteVideoCompletely must clear history even when there is no local record.
func TestInProcDeleteVideoCompletelyWithoutLocalFile(t *testing.T) {
	p, database := newInProc(t, procexec.OS{})
	ctx := context.Background()

	if err := database.UpsertVideo(context.Background(), "h1", "T", "C", "UC1", 60, 1, "20240101", "http://x/h1"); err != nil {
		t.Fatalf("UpsertVideo: %v", err)
	}
	if err := database.AddHistory(context.Background(), "h1", "playVideo", ""); err != nil {
		t.Fatalf("AddHistory: %v", err)
	}

	if err := p.DeleteVideoCompletely(ctx, "h1"); err != nil {
		t.Fatalf("DeleteVideoCompletely: %v", err)
	}
	hist, err := database.VideoHistory(context.Background(), "h1")
	if err != nil {
		t.Fatalf("VideoHistory: %v", err)
	}
	if len(hist) != 0 {
		t.Errorf("history not cleared: %d entries remain", len(hist))
	}
}

// Enqueue starts a download that DownloadItems reports, and Events bridges the
// downloader's completion into an api.EventDownloadDone (item 18 runner + 19 InProc).
func TestInProcEnqueueDownloadItemsAndEvents(t *testing.T) {
	release := make(chan struct{})
	runner := procexec.FakeRunner{New: func([]string) procexec.Cmd {
		return &procexec.FakeCmd{
			Stdout: "[download] Destination: /tmp/x.mkv\n",
			WaitFn: func() error { <-release; return nil },
		}
	}}
	p, _ := newInProc(t, runner)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	evCh, err := p.Events(ctx)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	if err := p.Enqueue(ctx, domain.Video{ID: "v1", URL: "http://x/v1"}, false); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// The queued/active item must be visible while the fake process is held open.
	deadline := time.After(2 * time.Second)
	for {
		items, _ := p.DownloadItems(ctx)
		if len(items) == 1 && items[0].VideoID == "v1" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("DownloadItems never reported the enqueued download")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	close(release) // let the fake process finish
	for {
		select {
		case ev := <-evCh:
			if ev.Kind == api.EventDownloadDone && ev.VideoID == "v1" {
				return
			}
		case <-time.After(2 * time.Second):
			t.Fatal("no EventDownloadDone bridged from the downloader")
		}
	}
}

// StartBackgroundEnrichment's goroutine must be joinable and exit promptly when
// its context is canceled, so the single-binary shutdown can wait it out before
// database.Close() (H-1: no background DB write may race the close). Run under
// -race, newInProc's t.Cleanup Close after WaitEnrichment is the real assertion.
func TestWaitEnrichmentJoinsOnCancel(t *testing.T) {
	p, _ := newInProc(t, procexec.OS{})
	ctx, cancel := context.WithCancel(context.Background())
	p.StartBackgroundEnrichment(ctx)
	cancel()

	done := make(chan struct{})
	go func() { p.WaitEnrichment(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("WaitEnrichment did not return within 5s of cancel")
	}

	// Idempotent: a second wait on an already-exited goroutine returns at once.
	p.WaitEnrichment()
}

// WaitEnrichment is a no-op (does not block or panic) when enrichment was never
// started — closeBackend must stay safe on that path.
func TestWaitEnrichmentNeverStarted(t *testing.T) {
	p, _ := newInProc(t, procexec.OS{})
	p.WaitEnrichment()
}

// With a refresh cadence configured the enrichment goroutine loops until its
// context is canceled; cancel must still break the loop promptly so shutdown's
// WaitEnrichment returns instead of hanging on the ticker. YouTubeEnabled stays
// false so each pass is a cheap no-op — this isolates the loop/cancel wiring.
func TestWaitEnrichmentStopsPeriodicLoopOnCancel(t *testing.T) {
	database, err := db.New(t.TempDir(), false, 90)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	// Long tick; cancel, not the tick, must end the loop. ChannelRefreshMinutes
	// lives on the embedded DaemonConfig.
	cfg := &config.Config{DaemonConfig: config.DaemonConfig{ChannelRefreshMinutes: 60}}
	dl := downloader.NewWithRunner(cfg, database, procexec.OS{})
	p := api.NewInProc(database, nil, dl, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	p.StartBackgroundEnrichment(ctx)
	cancel()

	done := make(chan struct{})
	go func() { p.WaitEnrichment(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("periodic enrichment loop did not stop within 5s of cancel")
	}
}
