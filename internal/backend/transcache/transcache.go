// Package transcache is a client-local cache of display-ready transcript text,
// keyed by video ID. It is the transcript counterpart to backend/thumbs: a plain
// directory of files a remote client fills with the transcripts it views, so
// re-opening one is instant and works offline. It deliberately stores only the
// finished display text (not the .srt/markdown the daemon's transcript store
// manages) — turning a transcript into text is the daemon's job; keeping a copy
// of the result is this cache's.
package transcache

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Store is a directory of transcript text files keyed by video ID. The zero
// value is unusable; construct with NewStore. Methods are safe for concurrent
// use (the filesystem provides the atomicity we need).
type Store struct {
	dir string
}

// NewStore creates (mkdir -p) the cache directory and returns a Store.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("transcache: mkdir %s: %w", dir, err)
	}
	return &Store{dir: dir}, nil
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

func (s *Store) path(id string) string { return filepath.Join(s.dir, id+".txt") }

// Get returns the cached transcript text for id, or ("", false) if none is
// cached (or id is unsafe). A read error is treated as a miss — the caller just
// re-fetches through the backend.
func (s *Store) Get(id string) (string, bool) {
	if !safeID(id) {
		return "", false
	}
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return "", false
	}
	return string(data), true
}

// Put writes text for id, replacing any existing file atomically (temp+rename).
func (s *Store) Put(id, text string) error {
	if !safeID(id) {
		return fmt.Errorf("transcache: unsafe id %q", id)
	}
	tmp, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("transcache: temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(text); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("transcache: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("transcache: close: %w", err)
	}
	if err := os.Rename(tmpName, s.path(id)); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("transcache: rename: %w", err)
	}
	return nil
}

// RetainNewest evicts all but the newest max cached transcripts by mtime,
// bounding the reactive "cache everything you view" set on the client (there is
// no DB-driven eligibility to prune against, as the daemon has). max <= 0 keeps
// everything. Returns the number of files removed.
func (s *Store) RetainNewest(max int) (int, error) {
	if max <= 0 {
		return 0, nil
	}
	return retainNewest(s.dir, ".txt", max)
}

// retainNewest is the shared newest-N-by-mtime sweep for client caches: it keeps
// the max most-recently-modified files with the given suffix in dir and removes
// the rest. Temp files and unrelated entries are ignored.
func retainNewest(dir, suffix string, max int) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("transcache: readdir: %w", err)
	}
	type item struct {
		name    string
		modUnix int64
	}
	var files []item
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue // vanished between ReadDir and Info; skip
		}
		files = append(files, item{name: e.Name(), modUnix: info.ModTime().UnixNano()})
	}
	if len(files) <= max {
		return 0, nil
	}
	// Newest first, then remove everything past the cap.
	sort.Slice(files, func(i, j int) bool { return files[i].modUnix > files[j].modUnix })
	removed := 0
	for _, f := range files[max:] {
		if err := os.Remove(filepath.Join(dir, f.name)); err != nil && !os.IsNotExist(err) {
			return removed, fmt.Errorf("transcache: retain remove %s: %w", f.name, err)
		}
		removed++
	}
	return removed, nil
}
