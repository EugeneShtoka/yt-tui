package youtube

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
)

// stateCfg returns a config rooted at a temp state dir, with the update check on.
func stateCfg(t *testing.T) *config.Config {
	t.Helper()
	cfg := &config.Config{}
	cfg.StateDir = t.TempDir()
	cfg.YtdlpUpdateCheck = true
	return cfg
}

// TestUpstreamRecordRoundTrip: what the background check writes is what the probe
// reads back.
func TestUpstreamRecordRoundTrip(t *testing.T) {
	cfg := stateCfg(t)
	path := upstreamCachePath(cfg)
	want := upstreamRecord{Version: "2026.08.19", CheckedAt: time.Now().Truncate(time.Second)}
	if err := writeUpstreamRecord(path, want); err != nil {
		t.Fatalf("writeUpstreamRecord: %v", err)
	}
	got, err := readUpstreamRecord(path)
	if err != nil {
		t.Fatalf("readUpstreamRecord: %v", err)
	}
	if got.Version != want.Version || !got.CheckedAt.Equal(want.CheckedAt) {
		t.Errorf("round trip: got %+v, want %+v", got, want)
	}
	ver, ok := CachedLatestVersion(cfg)
	if !ok || ver.Raw != "2026.08.19" {
		t.Errorf("CachedLatestVersion = %q (ok=%v), want 2026.08.19", ver.Raw, ok)
	}
	// The temp file used for the atomic replace must not be left behind.
	entries, err := os.ReadDir(cfg.StateDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("state dir holds %d files, want just the record: %v", len(entries), entries)
	}
}

// TestCachedLatestVersionMissing: no record and no state dir both mean "no
// reference", never an error the caller has to handle.
func TestCachedLatestVersionMissing(t *testing.T) {
	if _, ok := CachedLatestVersion(stateCfg(t)); ok {
		t.Error("an empty state dir must yield no cached version")
	}
	if _, ok := CachedLatestVersion(&config.Config{}); ok {
		t.Error("a config with no state dir must yield no cached version")
	}
	if _, ok := CachedLatestVersion(nil); ok {
		t.Error("a nil config must yield no cached version")
	}
}

// TestCachedLatestVersionCorrupt: a truncated or hand-edited record degrades to
// "no reference" instead of failing the probe.
func TestCachedLatestVersionCorrupt(t *testing.T) {
	cfg := stateCfg(t)
	if err := os.WriteFile(upstreamCachePath(cfg), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, ok := CachedLatestVersion(cfg); ok {
		t.Error("a corrupt record must yield no cached version")
	}
}

// TestRefreshLatestVersionDisabled: with the check off, RefreshLatestVersion must
// make no request and write nothing — it is the switch that keeps the app offline.
func TestRefreshLatestVersionDisabled(t *testing.T) {
	cfg := stateCfg(t)
	cfg.YtdlpUpdateCheck = false
	RefreshLatestVersion(context.Background(), cfg)
	if _, err := os.Stat(upstreamCachePath(cfg)); !os.IsNotExist(err) {
		t.Errorf("disabled check wrote a record (stat err = %v)", err)
	}
}

// TestRefreshLatestVersionSkipsFreshRecord: a record younger than the refresh
// interval is left untouched, so launching repeatedly makes at most one request a
// day. A sentinel version proves the file was not rewritten.
func TestRefreshLatestVersionSkipsFreshRecord(t *testing.T) {
	cfg := stateCfg(t)
	path := upstreamCachePath(cfg)
	sentinel := upstreamRecord{Version: "2026.01.01", CheckedAt: time.Now()}
	if err := writeUpstreamRecord(path, sentinel); err != nil {
		t.Fatalf("writeUpstreamRecord: %v", err)
	}
	RefreshLatestVersion(context.Background(), cfg)
	got, err := readUpstreamRecord(path)
	if err != nil {
		t.Fatalf("readUpstreamRecord: %v", err)
	}
	if got.Version != sentinel.Version {
		t.Errorf("fresh record was replaced: got %q, want %q", got.Version, sentinel.Version)
	}
}

// TestRefreshLatestVersionNoStateDir: nowhere to cache means nothing to do, and
// no panic on a bare config.
func TestRefreshLatestVersionNoStateDir(t *testing.T) {
	cfg := &config.Config{}
	cfg.YtdlpUpdateCheck = true
	RefreshLatestVersion(context.Background(), cfg)
}

// TestUpstreamCachePathUnderStateDir pins where the record lives: StateDir holds
// the app's regenerable per-host state, and this is exactly that.
func TestUpstreamCachePathUnderStateDir(t *testing.T) {
	cfg := stateCfg(t)
	if got, want := upstreamCachePath(cfg), filepath.Join(cfg.StateDir, "ytdlp-latest.json"); got != want {
		t.Errorf("upstreamCachePath = %q, want %q", got, want)
	}
}
