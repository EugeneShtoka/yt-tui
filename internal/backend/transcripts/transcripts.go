// Package transcripts manages the on-disk store of video transcripts (captions
// converted to .srt), fetched by yt-dlp during enrichment and when video info is
// requested. Files are named <video_id>.<lang>.srt so a video's transcript(s)
// can be found, deleted, and garbage-collected by ID. Like the thumbnail cache
// it is bounded to the eligible set (newest-N per subscribed channel plus the
// recommended feed); the enricher's Retain sweep evicts the rest.
package transcripts

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Store is a directory of transcript .srt files keyed by video ID. Construct
// with NewStore. An optional second directory (enabled via EnableMarkdown) holds
// the canonical <id>.md notes served to the viewer and to Obsidian; unlike the
// bounded .srt archive it is never swept by Retain.
type Store struct {
	dir   string
	mdDir string // markdown notes dir; "" disables the markdown methods
}

// NewStore creates (mkdir -p) the transcript directory and returns a Store.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("transcripts: mkdir %s: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

// safeID reports whether id is a valid YouTube-style identifier safe to use in a
// filename/glob. Restricting the alphabet also blocks path traversal and glob
// metacharacters (id never contains '*', '?', '[', '.').
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

// OutputTemplate returns the yt-dlp -o template that writes a video's transcript
// files into this store as <dir>/<id>.<lang>.srt (yt-dlp fills %(id)s/%(ext)s).
func (s *Store) OutputTemplate() string {
	return filepath.Join(s.dir, "%(id)s.%(ext)s")
}

// matches returns the on-disk .srt transcript files for a video ID.
func (s *Store) matches(id string) []string {
	if !safeID(id) {
		return nil
	}
	srt, _ := filepath.Glob(filepath.Join(s.dir, id+".*.srt"))
	return srt
}

// Has reports whether at least one transcript file is cached for id.
func (s *Store) Has(id string) bool { return len(s.matches(id)) > 0 }

// Read returns the transcript for id as display-ready plain text, converting a
// stored .srt on the fly. Returns ("", false) when nothing is cached.
func (s *Store) Read(id string) (string, bool) {
	if !safeID(id) {
		return "", false
	}
	srt, _ := filepath.Glob(filepath.Join(s.dir, id+".*.srt"))
	for _, f := range srt {
		if data, err := os.ReadFile(f); err == nil {
			return SRTToText(string(data)), true
		}
	}
	return "", false
}

// SelectCues returns the timed cues for id, choosing among the downloaded .srt
// sidecars by language priority: the video's original language wins when it is
// itself one of the acceptable languages, otherwise the first acceptable
// language (in order) that was downloaded. Acceptable entries may be yt-dlp-style
// globs ("en.*"); regional variants ("en-US") match their base. Returns
// (nil, false) when no sidecar matches (e.g. only a timestamp-less .txt exists) —
// callers that need chapter bucketing fall back to flat text in that case.
func (s *Store) SelectCues(id, original string, acceptable []string) ([]Cue, bool) {
	files := s.srtLangs(id)
	if len(files) == 0 {
		return nil, false
	}
	// Priority: the original language first (only when it is itself acceptable),
	// then the acceptable list in its configured order.
	selectors := make([]string, 0, len(acceptable)+1)
	if original != "" && anyLangMatches(acceptable, original) {
		selectors = append(selectors, original)
	}
	selectors = append(selectors, acceptable...)
	for _, sel := range selectors {
		if path := matchLangFile(files, sel); path != "" {
			if data, err := os.ReadFile(path); err == nil {
				return SRTToCues(string(data)), true
			}
		}
	}
	return nil, false
}

// srtLangs maps each downloaded sidecar's language code (the "<lang>" in
// "<id>.<lang>.srt") to its file path.
func (s *Store) srtLangs(id string) map[string]string {
	out := map[string]string{}
	if !safeID(id) {
		return out
	}
	srt, _ := filepath.Glob(filepath.Join(s.dir, id+".*.srt"))
	for _, f := range srt {
		lang := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), id+"."), ".srt")
		if lang != "" {
			out[lang] = f
		}
	}
	return out
}

// matchLangFile returns the path of the sidecar whose language satisfies selector,
// scanning language codes in sorted order so the choice is deterministic when a
// glob matches several regional variants. Returns "" when none match.
func matchLangFile(files map[string]string, selector string) string {
	langs := make([]string, 0, len(files))
	for lang := range files {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	for _, lang := range langs {
		if langMatches(selector, lang) {
			return files[lang]
		}
	}
	return ""
}

// langBase normalizes a language selector for matching: lowercased with a trailing
// yt-dlp glob suffix (".*") removed, so "en.*" and "en" both reduce to "en".
func langBase(s string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".*")
}

// langMatches reports whether a concrete language code (e.g. "en-US") satisfies a
// selector (e.g. "en.*" or "en"): an exact base match or a "base-…" variant.
func langMatches(selector, code string) bool {
	base := langBase(selector)
	code = strings.ToLower(code)
	return base != "" && (code == base || strings.HasPrefix(code, base+"-"))
}

// anyLangMatches reports whether code satisfies any of the selectors.
func anyLangMatches(selectors []string, code string) bool {
	for _, sel := range selectors {
		if langMatches(sel, code) {
			return true
		}
	}
	return false
}

// Delete removes every transcript file for id. Missing files are not an error.
func (s *Store) Delete(id string) error {
	for _, f := range s.matches(id) {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("transcripts: delete %s: %w", f, err)
		}
	}
	return nil
}

// Retain deletes every transcript whose video ID is not in keep, returning the
// number of files removed. The single GC for transcripts left behind by aged-out
// recommended videos, evicted newest-N entries, and unsubscribed channels.
func (s *Store) Retain(keep map[string]bool) (int, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, fmt.Errorf("transcripts: readdir: %w", err)
	}
	removed := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".srt") {
			continue
		}
		id := strings.SplitN(name, ".", 2)[0] // <id>.<lang>.srt — id has no dots
		if keep[id] {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, name)); err != nil && !os.IsNotExist(err) {
			return removed, fmt.Errorf("transcripts: retain remove %s: %w", name, err)
		}
		removed++
	}
	return removed, nil
}
