package downloader

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/procexec"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusActive   Status = "active"
	StatusComplete Status = "complete"
	StatusFailed   Status = "failed"
)

type DownloadType string

const (
	TypeVideo DownloadType = "video"
	TypeAudio DownloadType = "audio"
)

type Item struct {
	Video     domain.Video
	Type      DownloadType
	Progress  float64
	Speed     string
	ETA       string
	Status    Status
	FilePath  string
	Err       error
	StartedAt time.Time
	cancel    context.CancelFunc
}

const (
	// eventBufferSize is the depth of the downloader→consumer event channel.
	// Progress events are lossy (drop-newest), so this only needs to absorb
	// bursts between consumer polls.
	eventBufferSize = 64
	// maxStderrTailLines is how many trailing yt-dlp stderr lines are retained
	// to build a failure message.
	maxStderrTailLines = 20
	// maxErrMsgLen caps a persisted/emitted error string.
	maxErrMsgLen = 500
)

type EventKind int

const (
	EventProgress EventKind = iota
	EventComplete
	EventError
)

type Event struct {
	Kind     EventKind
	VideoID  string
	Type     DownloadType
	Progress float64
	Speed    string
	ETA      string
	FilePath string
	Err      error
}

// evictCompletedAfter is how long a completed item lingers in the queue before
// auto-eviction. Failed items are never auto-evicted — they stay visible until
// a manual dismiss / Clear / restart so the failure is seen.
const evictCompletedAfter = 2 * time.Minute

type Downloader struct {
	cfg       *config.Config
	db        completionSink
	runner    procexec.Runner
	parser    *progressParser
	mu        sync.RWMutex
	items     map[string]*Item
	order     []string
	events    *fanout[Event] // pub/sub broadcaster (C-2): each subscriber sees every event
	semaphore chan struct{}
	wg        sync.WaitGroup // tracks in-flight run goroutines for Stop() to join

	// evictAfter is the completed-item eviction delay. Defaults to
	// evictCompletedAfter; overridable in tests to keep them fast.
	evictAfter time.Duration
}

// New creates a Downloader that execs the real yt-dlp binary.
func New(cfg *config.Config, database *db.DB) *Downloader {
	return NewWithRunner(cfg, database, procexec.OS{})
}

// NewWithRunner creates a Downloader backed by an explicit process runner and
// completion sink. It is the injection seam for tests, which supply a
// procexec.FakeRunner in place of a real yt-dlp process (and may supply a fake
// sink in place of *db.DB). Production callers pass *db.DB, which satisfies
// completionSink.
func NewWithRunner(cfg *config.Config, sink completionSink, runner procexec.Runner) *Downloader {
	max := cfg.MaxDownloads
	if max <= 0 {
		max = 3
	}
	d := &Downloader{
		cfg:        cfg,
		db:         sink,
		runner:     runner,
		parser:     newProgressParser(),
		items:      make(map[string]*Item),
		events:     newFanout[Event](eventBufferSize),
		semaphore:  make(chan struct{}, max),
		evictAfter: evictCompletedAfter,
	}
	return d
}

// Subscribe registers a new listener and returns a channel that receives every
// event emitted from now on, until ctx is canceled (at which point the
// channel is closed). Callers must cancel ctx when done listening — e.g. on
// resubscribe — or the registration and its goroutine leak.
func (d *Downloader) Subscribe(ctx context.Context) <-chan Event {
	return d.events.subscribe(ctx, eventBufferSize)
}

// Start enqueues a video download. Idempotent if already queued.
func (d *Downloader) Start(video domain.Video, dlType DownloadType) {
	d.mu.Lock()
	if _, ok := d.items[video.ID]; ok {
		d.mu.Unlock()
		return
	}
	// Create the cancel func up front so a download that is still queued
	// (blocked on the semaphore) is cancellable by Remove/Stop.
	ctx, cancel := context.WithCancel(context.Background())
	item := &Item{
		Video:     video,
		Type:      dlType,
		Status:    StatusPending,
		StartedAt: time.Now(),
		cancel:    cancel,
	}
	d.items[video.ID] = item
	d.order = append(d.order, video.ID)
	d.wg.Add(1)
	d.mu.Unlock()

	go d.run(ctx, item)
}

func (d *Downloader) run(ctx context.Context, item *Item) {
	defer d.wg.Done()
	defer item.cancel()

	// Wait for a concurrency slot, but bail out if canceled while queued.
	select {
	case d.semaphore <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-d.semaphore }()

	d.mu.Lock()
	item.Status = StatusActive
	d.mu.Unlock()

	cmd := d.runner.Command(ctx, "yt-dlp", d.buildArgs(item)...)
	stdout, stderr, ok := d.startProcess(item, cmd)
	if !ok {
		return
	}

	stderrLines, stderrDone := scanStderr(stderr)
	finalPath := d.scanProgress(item, stdout)

	<-stderrDone // drain stderr before Wait (StderrPipe contract)
	if err := cmd.Wait(); err != nil {
		d.fail(item, waitError(err, *stderrLines))
		return
	}

	d.complete(item, d.resolveFinalPath(item, finalPath))
}

// startProcess opens the stdout/stderr pipes and starts cmd. Any failure means
// the process is not running: it fails the item and returns ok=false so the
// caller aborts. The raw error is surfaced verbatim (no wrapping), matching the
// pre-refactor behavior.
func (d *Downloader) startProcess(item *Item, cmd procexec.Cmd) (stdout, stderr io.ReadCloser, ok bool) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		d.fail(item, err)
		return nil, nil, false
	}
	stderr, err = cmd.StderrPipe()
	if err != nil {
		d.fail(item, err)
		return nil, nil, false
	}
	if err := cmd.Start(); err != nil {
		d.fail(item, err)
		return nil, nil, false
	}
	return stdout, stderr, true
}

// scanStderr drains stderr in a goroutine, retaining only the last
// maxStderrTailLines lines. The returned channel is closed when stderr hits EOF;
// callers must receive from it before cmd.Wait (StderrPipe contract). The
// returned slice pointer is safe to read only after that receive.
func scanStderr(stderr io.ReadCloser) (*[]string, <-chan struct{}) {
	var lines []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			lines = append(lines, sc.Text())
			if len(lines) > maxStderrTailLines {
				lines = lines[1:]
			}
		}
	}()
	return &lines, done
}

// scanProgress reads yt-dlp's stdout line by line, updating item progress and
// emitting progress events, and returns the resolved destination path (empty if
// yt-dlp never reported one). It blocks until stdout hits EOF.
func (d *Downloader) scanProgress(item *Item, stdout io.ReadCloser) string {
	var finalPath string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		upd, path := d.parser.parseLine(scanner.Text())
		switch {
		case upd != nil:
			d.mu.Lock()
			item.Progress = upd.Progress
			item.Speed = upd.Speed
			item.ETA = upd.ETA
			d.mu.Unlock()
			d.emit(Event{
				Kind: EventProgress, VideoID: item.Video.ID,
				Progress: upd.Progress, Speed: upd.Speed, ETA: upd.ETA,
			})
		case path != "":
			finalPath = path
		}
	}
	return finalPath
}

// waitError builds the failure error from cmd.Wait's error and the tail of
// stderr, capped at maxErrMsgLen.
func waitError(err error, stderrLines []string) error {
	tail := strings.TrimSpace(strings.Join(stderrLines, "\n"))
	errMsg := fmt.Sprintf("yt-dlp: %v", err)
	if tail != "" {
		errMsg += ": " + tail
	}
	if len(errMsg) > maxErrMsgLen {
		errMsg = errMsg[:maxErrMsgLen]
	}
	return fmt.Errorf("%s", errMsg)
}

// resolveFinalPath returns finalPath if yt-dlp reported one, otherwise derives a
// fallback path from the video metadata and configured download dir.
func (d *Downloader) resolveFinalPath(item *Item, finalPath string) string {
	if finalPath != "" {
		return finalPath
	}
	ext := "mkv"
	if item.Type == TypeAudio {
		ext = d.cfg.AudioFormat
	}
	name := sanitizeFilename(item.Video.Channel + " - " + item.Video.Title)
	return filepath.Join(d.cfg.DownloadDir, name+"."+ext)
}

// complete marks the item done, persists it to the DB, and emits the completion
// event. AddLocalVideo is load-bearing: the Local tab reads from the DB, so a
// failure there means the file exists on disk but is invisible to the app. All
// three DB writes are logged so any divergence leaves a trace.
func (d *Downloader) complete(item *Item, finalPath string) {
	d.mu.Lock()
	item.Status = StatusComplete
	item.Progress = 100
	item.FilePath = finalPath
	d.mu.Unlock()

	// The three completion writes record a download that already finished on disk,
	// so they deliberately use context.Background(), not the item's (now-canceled
	// at shutdown) run context: a completed file must land in the DB even when the
	// process is tearing down. dl.Stop() joins this goroutine before the DB closes,
	// so these can't race database.Close() (H-1).
	ctx := context.Background()
	if err := d.db.UpsertVideo(ctx,
		item.Video.ID, item.Video.Title, item.Video.Channel, item.Video.ChannelID,
		item.Video.Duration, item.Video.ViewCount, item.Video.UploadDate, item.Video.URL,
	); err != nil {
		debug.Log("downloader: UpsertVideo %s: %v", item.Video.ID, err)
	}
	var fileSize int64
	if info, statErr := os.Stat(finalPath); statErr == nil {
		fileSize = info.Size()
	}
	if err := d.db.AddLocalVideo(ctx, domain.LocalVideo{
		ID:           item.Video.ID,
		Title:        item.Video.Title,
		Channel:      item.Video.Channel,
		Duration:     item.Video.Duration,
		FilePath:     finalPath,
		FileSize:     fileSize,
		DownloadType: string(item.Type),
		DownloadedAt: time.Now(),
		Status:       domain.StatusNew,
	}); err != nil {
		debug.Log("downloader: AddLocalVideo %s (%s): %v", item.Video.ID, finalPath, err)
	}
	if err := d.db.AddHistory(ctx, item.Video.ID, "download "+string(item.Type), ""); err != nil {
		debug.Log("downloader: AddHistory %s: %v", item.Video.ID, err)
	}

	d.emit(Event{Kind: EventComplete, VideoID: item.Video.ID, Type: item.Type, FilePath: finalPath})

	// A completed item is a transient status line; auto-evict it after a grace
	// period so the queue self-cleans. Failed items are deliberately left
	// (see fail): the eviction only ever targets a still-completed item.
	if d.evictAfter > 0 {
		time.AfterFunc(d.evictAfter, func() { d.evictIfComplete(item) })
	}
}

// evictIfComplete removes item from the queue only if it is still the current
// entry for its video ID and still Complete. The identity + status guard means
// a re-queued download (same ID) or a later failure is never evicted by a
// stale timer.
func (d *Downloader) evictIfComplete(item *Item) {
	d.mu.RLock()
	cur, ok := d.items[item.Video.ID]
	stillComplete := ok && cur == item && cur.Status == StatusComplete
	d.mu.RUnlock()
	if stillComplete {
		d.Remove(item.Video.ID)
	}
}

func (d *Downloader) buildArgs(item *Item) []string {
	var args []string

	if item.Type == TypeAudio {
		args = append(args, "-f", "bestaudio",
			"--extract-audio", "--audio-format", d.cfg.AudioFormat,
			"--audio-quality", "0")
	} else {
		args = append(args,
			"-f", "bestvideo[height<=1080]+bestaudio/best",
			"--merge-output-format", "mkv",
		)
		if d.cfg.Subtitles {
			if langs := d.cfg.SubtitleLangsArg(); langs != "" {
				args = append(args,
					"--write-sub", "--write-auto-sub",
					"--sub-langs", langs,
					"--embed-subs",
				)
			}
		}
	}

	// Save the transcript as a sidecar .srt next to the media (independent of the
	// embed-only Subtitles option, and applies to audio downloads too). Falls back
	// to English when no subtitle languages are configured.
	if d.cfg.SaveTranscript {
		langs := d.cfg.SubtitleLangsArg()
		if langs == "" {
			langs = "en.*"
		}
		args = append(args,
			"--write-sub", "--write-auto-sub",
			"--sub-langs", langs,
			"--convert-subs", "srt",
		)
	}

	if sb := d.cfg.SponsorBlockArg(); sb != "" {
		args = append(args, "--sponsorblock-remove", sb)
	}

	args = append(args,
		"-o", filepath.Join(d.cfg.DownloadDir, "%(channel)s - %(title)s.%(ext)s"),
		"--no-playlist",
		"--newline",
		"--no-warnings",
	)
	if d.cfg.Browser != "" {
		args = append(args, "--cookies-from-browser", d.cfg.Browser)
	}
	args = append(args, item.Video.URL)
	return args
}

// sanitizeFilename replaces characters that are invalid in filenames.
func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\x00':
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	result := strings.TrimSpace(b.String())
	if result == "" {
		return "download"
	}
	return result
}

func (d *Downloader) fail(item *Item, err error) {
	d.mu.Lock()
	item.Status = StatusFailed
	item.Err = err
	d.mu.Unlock()
	d.emit(Event{Kind: EventError, VideoID: item.Video.ID, Err: err})
}

// emit publishes an event to every subscriber non-blockingly (drop-newest when
// the buffer is full). Progress events are lossy by nature, and completion/error
// events self-heal via fetchItemsCmd polling. Delegates to the fanout broadcaster.
func (d *Downloader) emit(ev Event) {
	d.events.emit(ev)
}

// Remove cancels and removes a download item by video ID.
func (d *Downloader) Remove(id string) {
	d.mu.Lock()
	item, ok := d.items[id]
	if ok {
		if item.cancel != nil {
			item.cancel()
		}
		delete(d.items, id)
		for i, oid := range d.order {
			if oid == id {
				d.order = append(d.order[:i], d.order[i+1:]...)
				break
			}
		}
	}
	d.mu.Unlock()
}

// Clear dismisses every queued item (cancel-if-active). It is the bulk form of
// Remove and, like Remove, touches only the in-memory queue — never files, the
// DB, or history (the queue is a transient, process-lifetime view). It reuses
// the single-item Remove path per item so the two stay in lockstep.
func (d *Downloader) Clear() {
	d.mu.RLock()
	ids := append([]string(nil), d.order...)
	d.mu.RUnlock()
	for _, id := range ids {
		d.Remove(id)
	}
}

// Items returns a snapshot of all download items in order.
func (d *Downloader) Items() []Item {
	d.mu.RLock()
	defer d.mu.RUnlock()
	result := make([]Item, 0, len(d.order))
	for _, id := range d.order {
		if item, ok := d.items[id]; ok {
			result = append(result, *item)
		}
	}
	return result
}

// IsDownloading returns true if the video is queued or active.
func (d *Downloader) IsDownloading(id string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if item, ok := d.items[id]; ok {
		return item.Status == StatusPending || item.Status == StatusActive
	}
	return false
}

// Stop cancels all in-flight downloads and blocks until their goroutines have
// exited, so daemon shutdown doesn't race yt-dlp subprocess teardown or leave
// completion writes half-applied. mu is released before the join because run
// goroutines take mu themselves.
func (d *Downloader) Stop() {
	d.mu.Lock()
	for _, item := range d.items {
		if item.cancel != nil {
			item.cancel()
		}
	}
	d.mu.Unlock()
	d.wg.Wait()
}
