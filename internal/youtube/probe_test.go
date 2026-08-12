package youtube

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func writeCookieFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cookies.txt")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write cookie file: %v", err)
	}
	return path
}
