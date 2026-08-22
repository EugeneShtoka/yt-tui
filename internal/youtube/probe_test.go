package youtube

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
)

// probeWith runs the availability probe against cfg with a stubbed PATH lookup:
// found controls whether yt-dlp is reported present.
func probeWith(cfg *config.Config, found bool) []config.ConfigIssue {
	look := func(string) (string, error) {
		if found {
			return "/usr/bin/yt-dlp", nil
		}
		return "", errors.New("not found")
	}
	return availabilityProbe{cfg: cfg, lookPath: look}.run()
}

func hasSeverity(issues []config.ConfigIssue, sev config.Severity, substr string) bool {
	for _, iss := range issues {
		if iss.Severity == sev && strings.Contains(iss.Message, substr) {
			return true
		}
	}
	return false
}

// TestProbeDisabledSkips: with youtube_enabled=false the probe stays silent even
// when yt-dlp is missing — the user opted out of YouTube.
func TestProbeDisabledSkips(t *testing.T) {
	cfg := &config.Config{}
	cfg.YouTubeEnabled = false
	if issues := probeWith(cfg, false); len(issues) != 0 {
		t.Errorf("disabled probe returned issues: %+v", issues)
	}
}

// TestProbeMissingBinary: yt-dlp absent is a hard capability error.
func TestProbeMissingBinary(t *testing.T) {
	cfg := &config.Config{}
	cfg.YouTubeEnabled = true
	cfg.Browser = "vivaldi+gnomekeyring"
	issues := probeWith(cfg, false)
	if !hasSeverity(issues, config.SeverityError, "yt-dlp") {
		t.Errorf("missing yt-dlp not reported as an error: %+v", issues)
	}
}

// TestProbeNoCookieSource: binary present but neither browser nor cookies_file
// configured — YouTube can't authenticate, so warn.
func TestProbeNoCookieSource(t *testing.T) {
	cfg := &config.Config{}
	cfg.YouTubeEnabled = true
	issues := probeWith(cfg, true)
	if !hasSeverity(issues, config.SeverityWarning, "cookie") {
		t.Errorf("missing cookie source not reported: %+v", issues)
	}
}

// TestProbeBrowserSourceClean: yt-dlp present + a browser cookie source is all
// we can cheaply verify locally, so no issue is raised.
func TestProbeBrowserSourceClean(t *testing.T) {
	cfg := &config.Config{}
	cfg.YouTubeEnabled = true
	cfg.Browser = "vivaldi+gnomekeyring"
	if issues := probeWith(cfg, true); len(issues) != 0 {
		t.Errorf("clean browser source produced issues: %+v", issues)
	}
}

// TestProbeCookiesFileMissing: a configured cookies_file that isn't on disk is
// an error the user must fix.
func TestProbeCookiesFileMissing(t *testing.T) {
	cfg := &config.Config{}
	cfg.YouTubeEnabled = true
	cfg.CookiesFile = filepath.Join(t.TempDir(), "nope.txt")
	issues := probeWith(cfg, true)
	if !hasSeverity(issues, config.SeverityError, "cookies_file") {
		t.Errorf("missing cookies_file not reported: %+v", issues)
	}
}

// TestProbeCookiesFileNoSAPISID: a cookie file that parses but carries no
// SAPISID means YouTube is "allowed but unavailable".
func TestProbeCookiesFileNoSAPISID(t *testing.T) {
	path := writeCookieFile(t, "youtube.com\tTRUE\t/\tTRUE\t0\tPREF\tsomevalue\n")
	cfg := &config.Config{}
	cfg.YouTubeEnabled = true
	cfg.CookiesFile = path
	issues := probeWith(cfg, true)
	if !hasSeverity(issues, config.SeverityError, "SAPISID") {
		t.Errorf("cookies_file without SAPISID not reported: %+v", issues)
	}
}

// TestProbeCookiesFileValid: a cookie file with a SAPISID resolves cleanly.
func TestProbeCookiesFileValid(t *testing.T) {
	path := writeCookieFile(t, "youtube.com\tTRUE\t/\tTRUE\t0\tSAPISID\tabc123\n")
	cfg := &config.Config{}
	cfg.YouTubeEnabled = true
	cfg.CookiesFile = path
	if issues := probeWith(cfg, true); len(issues) != 0 {
		t.Errorf("valid cookies_file produced issues: %+v", issues)
	}
}

// probeVersion runs the probe with yt-dlp present, a clean browser cookie
// source (so the only issue can come from the version check), and injected
// version/clock stubs.
func probeVersion(cfg *config.Config, version string, versionErr error, now time.Time) []config.ConfigIssue {
	return availabilityProbe{
		cfg:        cfg,
		lookPath:   func(string) (string, error) { return "/usr/bin/yt-dlp", nil },
		runVersion: func() (string, error) { return version, versionErr },
		now:        func() time.Time { return now },
	}.run()
}

func enabledBrowserCfg() *config.Config {
	cfg := &config.Config{}
	cfg.YouTubeEnabled = true
	cfg.Browser = "vivaldi+gnomekeyring"
	return cfg
}

// TestProbeYtdlpStale: a yt-dlp older than ytdlpStaleAfter is reported as a
// warning telling the user to update.
func TestProbeYtdlpStale(t *testing.T) {
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	issues := probeVersion(enabledBrowserCfg(), "2026.06.09\n", nil, now)
	if !hasSeverity(issues, config.SeverityWarning, "outdated extractor") {
		t.Errorf("stale yt-dlp not reported: %+v", issues)
	}
}

// TestProbeYtdlpFresh: a recent yt-dlp raises no issue.
func TestProbeYtdlpFresh(t *testing.T) {
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	if issues := probeVersion(enabledBrowserCfg(), "2026.08.15", nil, now); len(issues) != 0 {
		t.Errorf("fresh yt-dlp produced issues: %+v", issues)
	}
}

// TestProbeYtdlpVersionUnreadable: a --version read failure is skipped, not
// surfaced — presence is already confirmed and startup must not block.
func TestProbeYtdlpVersionUnreadable(t *testing.T) {
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	if issues := probeVersion(enabledBrowserCfg(), "", errors.New("boom"), now); len(issues) != 0 {
		t.Errorf("unreadable version produced issues: %+v", issues)
	}
}

// TestProbeYtdlpVersionUnparseable: an unrecognized version string is skipped
// rather than turned into a bogus staleness warning.
func TestProbeYtdlpVersionUnparseable(t *testing.T) {
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	if issues := probeVersion(enabledBrowserCfg(), "unknown-build", nil, now); len(issues) != 0 {
		t.Errorf("unparseable version produced issues: %+v", issues)
	}
}

func TestParseYtdlpDate(t *testing.T) {
	tests := []struct {
		in     string
		wantOK bool
	}{
		{"2026.06.09", true},
		{"2026.06.09.123456", true}, // nightly build with a trailing time component
		{"2026.6.9", true},
		{"2026.06", false},
		{"", false},
		{"1999.06.09", false}, // implausibly old year
		{"2026.13.09", false}, // invalid month
		{"2026.06.40", false}, // invalid day
		{"stable", false},
	}
	for _, tt := range tests {
		if _, ok := parseYtdlpDate(tt.in); ok != tt.wantOK {
			t.Errorf("parseYtdlpDate(%q): ok=%v, want %v", tt.in, ok, tt.wantOK)
		}
	}
}

func writeCookieFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cookies.txt")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write cookie file: %v", err)
	}
	return path
}
