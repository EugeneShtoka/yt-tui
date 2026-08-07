package downloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/procexec"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.New(t.TempDir(), false, 90)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

// waitEvent drains the event channel until an event of the given kind arrives
// or the deadline passes.
func waitEvent(t *testing.T, ch <-chan Event, kind EventKind) Event {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Kind == kind {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for event kind %d", kind)
		}
	}
}

// A successful run must parse progress, capture the destination, mark the item
// Complete, emit EventComplete, and persist the local video to the DB.
func TestRunCompletesAndPersists(t *testing.T) {
	database := newTestDB(t)
	stdout := "[download]  10.0% of ~5.00MiB at 1.00MiB/s ETA 00:05\n" +
		"[download] Destination: /tmp/Chan - Title.mkv\n" +
		"[download] 100.0% of ~5.00MiB at 2.00MiB/s ETA 00:00\n"
	d := New(&config.Config{}, database)
	d.runner = procexec.FakeRunner{New: func([]string) procexec.Cmd {
		return &procexec.FakeCmd{Stdout: stdout}
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := d.Subscribe(ctx)

	d.Start(domain.Video{ID: "v1", Title: "Title", Channel: "Chan", URL: "http://x/v1"}, TypeVideo)

	ev := waitEvent(t, ch, EventComplete)
	if ev.FilePath != "/tmp/Chan - Title.mkv" {
		t.Errorf("completion path = %q, want the parsed Destination", ev.FilePath)
	}

	lv, ok, err := database.HasLocalVideo(context.Background(), "v1")
	if err != nil {
		t.Fatalf("HasLocalVideo: %v", err)
	}
	if !ok {
		t.Fatal("completed download was not persisted to the DB (orphan file)")
	}
	if lv.FilePath != "/tmp/Chan - Title.mkv" {
		t.Errorf("persisted FilePath = %q", lv.FilePath)
	}
}

// Phase 11: on completion the downloader stats the finished file and persists
// its byte size, so the Local tab can show and sort by size.
func TestRunPersistsFileSize(t *testing.T) {
	database := newTestDB(t)
	f := filepath.Join(t.TempDir(), "sized.mkv")
	content := []byte("some downloaded bytes")
	if err := os.WriteFile(f, content, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	stdout := "[download] Destination: " + f + "\n" +
		"[download] 100.0% of ~5.00MiB at 2.00MiB/s ETA 00:00\n"
	d := New(&config.Config{}, database)
	d.runner = procexec.FakeRunner{New: func([]string) procexec.Cmd {
		return &procexec.FakeCmd{Stdout: stdout}
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := d.Subscribe(ctx)

	d.Start(domain.Video{ID: "v1", Title: "Title", Channel: "Chan", URL: "http://x/v1"}, TypeVideo)
	waitEvent(t, ch, EventComplete)

	lv, ok, err := database.HasLocalVideo(context.Background(), "v1")
	if err != nil || !ok {
		t.Fatalf("HasLocalVideo: ok=%v err=%v", ok, err)
	}
	if lv.FileSize != int64(len(content)) {
		t.Errorf("persisted FileSize = %d, want %d", lv.FileSize, len(content))
	}
}

// A non-zero yt-dlp exit must mark the item Failed and carry the stderr tail.
func TestRunFailureCarriesStderr(t *testing.T) {
	d := New(&config.Config{}, newTestDB(t))
	d.runner = procexec.FakeRunner{New: func([]string) procexec.Cmd {
		return &procexec.FakeCmd{
			Stderr:  "ERROR: video unavailable\nsecond line",
			WaitErr: fmt.Errorf("exit status 1"),
		}
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := d.Subscribe(ctx)

	d.Start(domain.Video{ID: "boom", URL: "http://x/boom"}, TypeVideo)

	ev := waitEvent(t, ch, EventError)
	if ev.Err == nil {
		t.Fatal("EventError carried no error")
	}
	if got := ev.Err.Error(); !strings.Contains(got, "video unavailable") {
		t.Errorf("error %q does not include stderr tail", got)
	}
}

// Concurrent downloads must never exceed the configured slot count, and the
// shared item map / semaphore must stay race-free (run under -race).
func TestConcurrentDownloadsRespectSemaphore(t *testing.T) {
	const maxSlots = 2
	release := make(chan struct{})
	var active, peak int32
	var mu sync.Mutex

	d := New(&config.Config{DaemonConfig: config.DaemonConfig{MaxDownloads: maxSlots}}, newTestDB(t))
	d.runner = procexec.FakeRunner{New: func([]string) procexec.Cmd {
		return &procexec.FakeCmd{
			Stdout: "[download] Destination: /tmp/x.mkv\n",
			WaitFn: func() error {
				n := atomic.AddInt32(&active, 1)
				mu.Lock()
				if n > peak {
					peak = n
				}
				mu.Unlock()
				<-release // hold the slot until the test releases it
				atomic.AddInt32(&active, -1)
				return nil
			},
		}
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := d.Subscribe(ctx)

	const total = 6
	for i := range total {
		d.Start(domain.Video{ID: fmt.Sprintf("v%d", i), URL: "http://x"}, TypeVideo)
	}

	// Let the goroutines pile up against the semaphore, then release them.
	deadline := time.After(2 * time.Second)
	for atomic.LoadInt32(&active) < maxSlots {
		select {
		case <-deadline:
			t.Fatal("downloads never reached the slot limit")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(release)

	// Drain all completion events.
	for range total {
		waitEvent(t, ch, EventComplete)
	}
	if peak > maxSlots {
		t.Fatalf("peak concurrency %d exceeded %d slots", peak, maxSlots)
	}
}
