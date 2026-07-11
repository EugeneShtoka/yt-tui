package downloader

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
	tea "github.com/charmbracelet/bubbletea"
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
	Video     youtube.Video
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

// EventMsg wraps Event as a bubbletea message.
type EventMsg Event

type Downloader struct {
	cfg       *config.Config
	db        *db.DB
	mu        sync.RWMutex
	items     map[string]*Item
	order     []string
	eventCh   chan Event
	semaphore chan struct{}
}

func New(cfg *config.Config, database *db.DB) *Downloader {
	max := cfg.MaxDownloads
	if max <= 0 {
		max = 3
	}
	return &Downloader{
		cfg:       cfg,
		db:        database,
		items:     make(map[string]*Item),
		eventCh:   make(chan Event, 64),
		semaphore: make(chan struct{}, max),
	}
}

var progressRe = regexp.MustCompile(`\[download\]\s+(\d+\.?\d*)%\s+of\s+~?\S+\s+at\s+(\S+)\s+ETA\s+(\S+)`)
var destRe = regexp.MustCompile(`\[download\] Destination: (.+)`)
var mergerRe = regexp.MustCompile(`\[Merger\] Merging formats into "(.+)"`)

// Start enqueues a video download. Idempotent if already queued.
func (d *Downloader) Start(video youtube.Video, dlType DownloadType) {
	d.mu.Lock()
	if _, ok := d.items[video.ID]; ok {
		d.mu.Unlock()
		return
	}
	item := &Item{
		Video:     video,
		Type:      dlType,
		Status:    StatusPending,
		StartedAt: time.Now(),
	}
	d.items[video.ID] = item
	d.order = append(d.order, video.ID)
	d.mu.Unlock()

	go d.run(item)
}

func (d *Downloader) run(item *Item) {
	d.semaphore <- struct{}{}
	defer func() { <-d.semaphore }()

	ctx, cancel := context.WithCancel(context.Background())
	d.mu.Lock()
	item.Status = StatusActive
	item.cancel = cancel
	d.mu.Unlock()
	defer cancel()

	args := d.buildArgs(item)
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		d.fail(item, err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		d.fail(item, err)
		return
	}
	if err := cmd.Start(); err != nil {
		d.fail(item, err)
		return
	}

	var stderrLines []string
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			stderrLines = append(stderrLines, sc.Text())
			if len(stderrLines) > 20 {
				stderrLines = stderrLines[1:]
			}
		}
	}()

	var finalPath string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if m := progressRe.FindStringSubmatch(line); len(m) == 4 {
			pct, _ := strconv.ParseFloat(m[1], 64)
			d.mu.Lock()
			item.Progress = pct
			item.Speed = m[2]
			item.ETA = m[3]
			d.mu.Unlock()
			d.eventCh <- Event{
				Kind: EventProgress, VideoID: item.Video.ID,
				Progress: pct, Speed: m[2], ETA: m[3],
			}
		} else if m := mergerRe.FindStringSubmatch(line); len(m) == 2 {
			finalPath = strings.TrimSpace(m[1])
		} else if m := destRe.FindStringSubmatch(line); len(m) == 2 {
			finalPath = strings.TrimSpace(m[1])
		}
	}

	<-stderrDone // drain stderr before Wait (StderrPipe contract)
	if err := cmd.Wait(); err != nil {
		tail := strings.TrimSpace(strings.Join(stderrLines, "\n"))
		errMsg := fmt.Sprintf("yt-dlp: %v", err)
		if tail != "" {
			errMsg += ": " + tail
		}
		if len(errMsg) > 500 {
			errMsg = errMsg[:500]
		}
		d.fail(item, fmt.Errorf("%s", errMsg))
		return
	}

	if finalPath == "" {
		ext := "mkv"
		if item.Type == TypeAudio {
			ext = d.cfg.AudioFormat
		}
		name := sanitizeFilename(item.Video.Channel + " - " + item.Video.Title)
		finalPath = filepath.Join(d.cfg.DownloadDir, name+"."+ext)
	}

	d.mu.Lock()
	item.Status = StatusComplete
	item.Progress = 100
	item.FilePath = finalPath
	d.mu.Unlock()

	// Persist to DB
	_ = d.db.UpsertVideo(
		item.Video.ID, item.Video.Title, item.Video.Channel, item.Video.ChannelID,
		item.Video.Duration, item.Video.ViewCount, item.Video.UploadDate, item.Video.URL,
	)
	_ = d.db.AddLocalVideo(db.LocalVideo{
		ID:           item.Video.ID,
		Title:        item.Video.Title,
		Channel:      item.Video.Channel,
		Duration:     item.Video.Duration,
		FilePath:     finalPath,
		DownloadType: string(item.Type),
		DownloadedAt: time.Now(),
		Status:       db.StatusNew,
	})
	_ = d.db.AddHistory(item.Video.ID, "download "+string(item.Type), "")

	d.eventCh <- Event{Kind: EventComplete, VideoID: item.Video.ID, Type: item.Type, FilePath: finalPath}
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
	d.eventCh <- Event{Kind: EventError, VideoID: item.Video.ID, Err: err}
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

// WaitForEvent returns a Cmd that blocks until the next download event.
func (d *Downloader) WaitForEvent() tea.Cmd {
	return func() tea.Msg {
		return EventMsg(<-d.eventCh)
	}
}
