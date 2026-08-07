// Package profiles is the daemon-side store of named config profiles. A profile
// is an opaque JSON blob (the client's portable config profile — see
// internal/tui/app.configProfile): the daemon persists it under a stable name
// and never interprets its schema, so the profile format stays single-sourced
// in the client and evolving it needs no daemon change.
//
// Profiles are stored one file per name (<dir>/<name>.json). The store is
// global on the daemon — there is no per-user namespace, matching the daemon's
// single-bearer-token model.
package profiles

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrInvalidName is returned when a profile name is empty or would escape the
// store directory (contains a path separator or is a `.`/`..` traversal
// component). Names must be usable as a bare filename.
var ErrInvalidName = errors.New("invalid profile name")

const ext = ".json"

// Store persists named profiles as JSON files in a single directory.
type Store struct {
	dir string
}

// NewStore creates (mkdir -p) the profiles directory and returns a Store rooted
// at it.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("profiles: create dir: %w", err)
	}
	return &Store{dir: dir}, nil
}

// List returns the names of all stored profiles, sorted lexicographically.
func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("profiles: list: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if name, ok := strings.CutSuffix(e.Name(), ext); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// Get returns the stored profile bytes for name. found is false (with a nil
// error) when no such profile exists; a non-nil error means the read itself
// failed.
func (s *Store) Get(name string) (data []byte, found bool, err error) {
	path, err := s.path(name)
	if err != nil {
		return nil, false, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("profiles: read %q: %w", name, err)
	}
	return b, true, nil
}

// Save writes data as the profile named name, overwriting any existing one.
func (s *Store) Save(name string, data []byte) error {
	path, err := s.path(name)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("profiles: write %q: %w", name, err)
	}
	return nil
}

// path resolves name to its on-disk file, rejecting anything that isn't a bare,
// safe filename so a crafted name can never escape the store directory.
func (s *Store) path(name string) (string, error) {
	if name == "" || name == "." || name == ".." ||
		strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, os.PathSeparator) {
		return "", fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return filepath.Join(s.dir, name+ext), nil
}
