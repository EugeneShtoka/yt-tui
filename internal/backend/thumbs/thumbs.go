// Package thumbs manages the on-disk cache of video thumbnail images. Images are
// stored as plain files keyed by video ID under a single directory and served
// back to the client over the GetThumbnail RPC, so a remote client renders a
// thumbnail without reaching YouTube's CDN itself. The cache is deliberately
// bounded (the newest-N videos per subscribed channel plus the recommended
// feed); the enricher's Retain sweep evicts anything outside that set.
package thumbs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// maxImageBytes caps a single thumbnail download so a misbehaving URL can't
// exhaust memory or disk. YouTube thumbnails are well under this.
const maxImageBytes = 4 << 20 // 4 MiB

// fetchClient is the shared HTTP client for store-independent fetches (Fetch).
// A Store carries its own identical client; this one serves callers that have
// no Store (a remote client fetching the CDN directly when the daemon's
// thumbnail cache is off).
var fetchClient = &http.Client{Timeout: 30 * time.Second}

// Fetch downloads url and returns cropped image bytes without needing a Store.
// It is the store-independent counterpart to (*Store).Download+Put — used by a
// client that fetches the CDN itself when the daemon serves no thumbnails.
// Bytes are capped at maxImageBytes and letterbox-cropped, so the render layer
// receives a clean image exactly as it would from a Store.
func Fetch(ctx context.Context, url string) ([]byte, error) {
	data, err := download(ctx, fetchClient, url)
	if err != nil {
		return nil, err
	}
	if cropped, ok := CropLetterboxJPEG(data); ok {
		data = cropped
	}
	return data, nil
}

// Store is a directory of thumbnail image files keyed by video ID. The zero
// value is unusable; construct with NewStore. All methods are safe for
// concurrent use (the filesystem provides the atomicity we need).
type Store struct {
	dir    string
	client *http.Client
}

// NewStore creates (mkdir -p) the thumbnail directory and returns a Store.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("thumbs: mkdir %s: %w", dir, err)
	}
	return &Store{dir: dir, client: &http.Client{Timeout: 30 * time.Second}}, nil
}

// safeID reports whether id is a valid YouTube-style identifier safe to use as a
// filename. Restricting to this alphabet also blocks path traversal ("..", "/").
func safeID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

func (s *Store) path(id string) string { return filepath.Join(s.dir, id+".jpg") }

// Path returns the on-disk path a cached thumbnail for id would occupy (whether
// or not it exists). Callers building a markdown image embed need it to compute
// the note→image relative path.
func (s *Store) Path(id string) string { return s.path(id) }

// URLFor returns the predictable CDN URL for a video's thumbnail. hqdefault
// (480×360) exists for every public video, so thumbnails can be fetched without
// a yt-dlp metadata call.
func URLFor(videoID string) string {
	return "https://i.ytimg.com/vi/" + videoID + "/hqdefault.jpg"
}

// Get returns the cached image bytes for a video, or (nil, false, nil) if none
// is cached. A non-nil error means the read itself failed.
func (s *Store) Get(id string) ([]byte, bool, error) {
	if !safeID(id) {
		return nil, false, nil
	}
	data, err := os.ReadFile(s.path(id))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("thumbs: read %s: %w", id, err)
	}
	return data, true, nil
}

// Has reports whether a thumbnail is already cached for id.
func (s *Store) Has(id string) bool {
	if !safeID(id) {
		return false
	}
	_, err := os.Stat(s.path(id))
	return err == nil
}

// Put writes image bytes for id, replacing any existing file atomically. It is
// the single crop choke point: letterbox bars are trimmed here on write, so
// every cached image is clean and the render layer never crops. A non-JPEG or
// already-16:9 image passes through untouched.
func (s *Store) Put(id string, data []byte) error {
	if !safeID(id) {
		return fmt.Errorf("thumbs: unsafe id %q", id)
	}
	if cropped, ok := CropLetterboxJPEG(data); ok {
		data = cropped
	}
	tmp, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("thumbs: temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("thumbs: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("thumbs: close: %w", err)
	}
	if err := os.Rename(tmpName, s.path(id)); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("thumbs: rename: %w", err)
	}
	return nil
}

// Delete removes the cached thumbnail for id. A missing file is not an error.
func (s *Store) Delete(id string) error {
	if !safeID(id) {
		return nil
	}
	if err := os.Remove(s.path(id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("thumbs: delete %s: %w", id, err)
	}
	return nil
}

// Download fetches url and returns the image bytes (capped at maxImageBytes).
func (s *Store) Download(ctx context.Context, url string) ([]byte, error) {
	return download(ctx, s.client, url)
}

// download is the shared fetch body behind (*Store).Download and Fetch: a GET
// capped at maxImageBytes. It does not crop — Store.Put crops on write and
// Fetch crops its result, so each caller crops at its own choke point.
func download(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("thumbs: request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("thumbs: get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("thumbs: get %s: status %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes))
	if err != nil {
		return nil, fmt.Errorf("thumbs: read body: %w", err)
	}
	return data, nil
}

// RetainNewest evicts all but the newest max cached thumbnails by mtime. It is
// the client-side counterpart to Retain: a remote client's cache grows with
// whatever the user views and has no DB-driven eligibility to prune against, so
// it is bounded by a simple newest-N cap instead. max <= 0 keeps everything.
// Returns the number of files removed.
func (s *Store) RetainNewest(max int) (int, error) {
	if max <= 0 {
		return 0, nil
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, fmt.Errorf("thumbs: readdir: %w", err)
	}
	type item struct {
		name    string
		modUnix int64
	}
	var files []item
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jpg") {
			continue // skip temp files and anything not ours
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue // vanished between ReadDir and Info; skip
		}
		files = append(files, item{name: e.Name(), modUnix: info.ModTime().UnixNano()})
	}
	if len(files) <= max {
		return 0, nil
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modUnix > files[j].modUnix })
	removed := 0
	for _, f := range files[max:] {
		if err := os.Remove(filepath.Join(s.dir, f.name)); err != nil && !os.IsNotExist(err) {
			return removed, fmt.Errorf("thumbs: retain-newest remove %s: %w", f.name, err)
		}
		removed++
	}
	return removed, nil
}

// Retain deletes every cached thumbnail whose video ID is not in keep. This is
// the single GC that evicts thumbnails for recommended videos that aged out,
// subscribed videos that fell out of the newest-N window, and unsubscribed
// channels. Returns the number of files removed.
func (s *Store) Retain(keep map[string]bool) (int, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, fmt.Errorf("thumbs: readdir: %w", err)
	}
	removed := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".jpg") {
			continue // skip temp files and anything not ours
		}
		id := strings.TrimSuffix(name, ".jpg")
		if keep[id] {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, name)); err != nil && !os.IsNotExist(err) {
			return removed, fmt.Errorf("thumbs: retain remove %s: %w", name, err)
		}
		removed++
	}
	return removed, nil
}
