package transcripts

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnableMarkdown points the store at a directory (mkdir -p) for the canonical
// <id>.md notes and turns on the markdown methods. Separate from the .srt dir so
// the notes can live in an Obsidian vault while the raw captions stay in the
// bounded cache. A no-op when dir is empty.
func (s *Store) EnableMarkdown(dir string) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("transcripts: mkdir %s: %w", dir, err)
	}
	s.mdDir = dir
	return nil
}

// MarkdownEnabled reports whether a markdown dir has been configured.
func (s *Store) MarkdownEnabled() bool { return s.mdDir != "" }

// MarkdownDir returns the configured markdown notes directory ("" if disabled).
// Callers building a note's local image embed need it to compute the relative
// path from the note to the cached thumbnail.
func (s *Store) MarkdownDir() string { return s.mdDir }

func (s *Store) mdPath(id string) string { return filepath.Join(s.mdDir, id+".md") }

// RelImageRef returns the note→image path (forward-slashed, for markdown) from
// the markdown dir to imagePath, or "" when markdown is disabled, imagePath is
// empty, or a relative path can't be formed. The caller passes the cached
// thumbnail's path only when the file actually exists, so an absent image simply
// omits the embed.
func (s *Store) RelImageRef(imagePath string) string {
	if s.mdDir == "" || imagePath == "" {
		return ""
	}
	rel, err := filepath.Rel(s.mdDir, imagePath)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

// HasMarkdown reports whether the canonical note for id already exists. Used to
// make the build resumable — an existing note is never refetched or rebuilt.
func (s *Store) HasMarkdown(id string) bool {
	if s.mdDir == "" || !safeID(id) {
		return false
	}
	_, err := os.Stat(s.mdPath(id))
	return err == nil
}

// WriteMarkdown atomically writes a note's content, replacing any existing file.
func (s *Store) WriteMarkdown(id, content string) error {
	if s.mdDir == "" {
		return fmt.Errorf("transcripts: markdown dir not configured")
	}
	if !safeID(id) {
		return fmt.Errorf("transcripts: unsafe id %q", id)
	}
	tmp, err := os.CreateTemp(s.mdDir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("transcripts: md temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("transcripts: md write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("transcripts: md close: %w", err)
	}
	if err := os.Rename(tmpName, s.mdPath(id)); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("transcripts: md rename: %w", err)
	}
	return nil
}

// ReadMarkdown returns the note for id as display-ready text — frontmatter and
// image embed stripped (see stripForDisplay), leaving the Chapters and
// Transcript sections. Returns ("", false) when no note is cached.
func (s *Store) ReadMarkdown(id string) (string, bool) {
	if s.mdDir == "" || !safeID(id) {
		return "", false
	}
	data, err := os.ReadFile(s.mdPath(id))
	if err != nil {
		return "", false
	}
	return stripForDisplay(string(data)), true
}

// DeleteMarkdown removes the canonical note for id. A missing file is not an
// error. Called only on explicit per-video deletion — the GC sweep leaves notes
// alone, since they are the durable export.
func (s *Store) DeleteMarkdown(id string) error {
	if s.mdDir == "" || !safeID(id) {
		return nil
	}
	if err := os.Remove(s.mdPath(id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("transcripts: md delete %s: %w", id, err)
	}
	return nil
}
