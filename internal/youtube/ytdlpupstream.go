package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
)

const (
	// upstreamCacheFile records the last upstream lookup under StateDir, so the
	// probe can name the newest yt-dlp release without ever making a request of
	// its own.
	upstreamCacheFile = "ytdlp-latest.json"
	// upstreamRefreshInterval is how often the background lookup re-asks GitHub.
	// yt-dlp ships every week or two, so a daily check is already finer-grained
	// than the releases it tracks.
	upstreamRefreshInterval = 24 * time.Hour
	// upstreamLatestURL returns the newest tagged yt-dlp release. Unauthenticated
	// GitHub API access is rate-limited per IP, which once a day never approaches.
	upstreamLatestURL = "https://api.github.com/repos/yt-dlp/yt-dlp/releases/latest"
	// upstreamBodyLimit caps how much of the response is read — the one field
	// needed is near the top and the endpoint is not ours to trust unbounded.
	upstreamBodyLimit = 1 << 20
	// upstreamTimeout bounds the request. It runs off the startup path, so this
	// is generous: it exists to stop a hung connection, not to keep anyone waiting.
	upstreamTimeout = 15 * time.Second
)

// upstreamRecord is the on-disk cache: the newest release seen and when it was
// looked up.
type upstreamRecord struct {
	Version   string    `json:"version"`
	CheckedAt time.Time `json:"checked_at"`
}

// CachedLatestVersion returns the newest yt-dlp release recorded by the last
// background lookup. The record is used however old it is: it names a release
// that really happened, so an outdated one can only understate how far behind the
// host has fallen — it can never invent a version that does not exist. ok=false
// means no lookup has succeeded yet (a first run, or the check is turned off),
// and the probe then simply has no upstream reference to compare against.
func CachedLatestVersion(cfg *config.Config) (Version, bool) {
	path := upstreamCachePath(cfg)
	if path == "" {
		return Version{}, false
	}
	rec, err := readUpstreamRecord(path)
	if err != nil {
		return Version{}, false
	}
	return ParseVersion(rec.Version)
}

// RefreshLatestVersion looks up the newest yt-dlp release and caches it for the
// next probe, unless the check is disabled or the cached record is still fresh.
// This is the only part of the yt-dlp checks that touches the network, which is
// why it is a separate call the caller runs in the background — Probe itself only
// ever reads the cache, keeping startup local and instant.
func RefreshLatestVersion(ctx context.Context, cfg *config.Config) {
	if !cfg.YtdlpUpdateCheck {
		return
	}
	path := upstreamCachePath(cfg)
	if path == "" {
		return
	}
	if rec, err := readUpstreamRecord(path); err == nil && time.Since(rec.CheckedAt) < upstreamRefreshInterval {
		return // still fresh; nothing to ask
	}
	tag, err := fetchLatestTag(ctx)
	if err != nil {
		debug.Log("ytdlp update check: %v", err)
		return
	}
	if _, ok := ParseVersion(tag); !ok {
		debug.Log("ytdlp update check: unrecognized release tag %q", tag)
		return
	}
	if err := writeUpstreamRecord(path, upstreamRecord{Version: tag, CheckedAt: time.Now()}); err != nil {
		debug.Log("ytdlp update check: %v", err)
		return
	}
	debug.Log("ytdlp update check: latest release is %s", tag)
}

// upstreamCachePath is where the record lives, or "" when there is no state
// directory to keep it in (a zero config in tests, for instance).
func upstreamCachePath(cfg *config.Config) string {
	if cfg == nil || cfg.StateDir == "" {
		return ""
	}
	return filepath.Join(cfg.StateDir, upstreamCacheFile)
}

func readUpstreamRecord(path string) (upstreamRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return upstreamRecord{}, fmt.Errorf("readUpstreamRecord: %w", err)
	}
	var rec upstreamRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return upstreamRecord{}, fmt.Errorf("readUpstreamRecord %s: %w", path, err)
	}
	return rec, nil
}

// writeUpstreamRecord replaces the record atomically, so a probe reading the file
// concurrently sees either the old record or the new one, never a partial write.
func writeUpstreamRecord(path string, rec upstreamRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("writeUpstreamRecord: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), upstreamCacheFile+".*")
	if err != nil {
		return fmt.Errorf("writeUpstreamRecord: %w", err)
	}
	// Best-effort cleanup of the staging file; a no-op once the rename succeeded.
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writeUpstreamRecord: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writeUpstreamRecord: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("writeUpstreamRecord: %w", err)
	}
	return nil
}

// fetchLatestTag reads the tag name of yt-dlp's newest GitHub release.
func fetchLatestTag(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstreamLatestURL, nil)
	if err != nil {
		return "", fmt.Errorf("fetchLatestTag: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "yt-tui")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetchLatestTag: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetchLatestTag: %s", resp.Status)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, upstreamBodyLimit)).Decode(&release); err != nil {
		return "", fmt.Errorf("fetchLatestTag: %w", err)
	}
	if release.TagName == "" {
		return "", errors.New("fetchLatestTag: release has no tag_name")
	}
	return release.TagName, nil
}
