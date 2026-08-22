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

// probeVersion runs the probe with yt-dlp present at a fixed path and a clean
// browser cookie source, so the only issue can come from the version checks.
// installed/system/latest are the three references, each nil-able to model
// "unknown".
func probeVersion(installed, system, latest string, manager string) []config.ConfigIssue {
	ver := func(raw string) func() (Version, bool) {
		return func() (Version, bool) { return ParseVersion(raw) }
	}
	p := availabilityProbe{
		cfg:       enabledBrowserCfg(),
		lookPath:  func(string) (string, error) { return "/usr/local/bin/yt-dlp", nil },
		installed: ver(installed),
		latest:    ver(latest),
	}
	if system != "" {
		p.system = func() (Version, string, bool) {
			v, ok := ParseVersion(system)
			return v, manager, ok
		}
	}
	return p.run()
}

func enabledBrowserCfg() *config.Config {
	cfg := &config.Config{}
	cfg.YouTubeEnabled = true
	cfg.Browser = "vivaldi+gnomekeyring"
	return cfg
}

// TestProbeYtdlpShadowed: a yt-dlp on PATH older than the one the package manager
// provides is the classic stale-copy-earlier-in-PATH setup, and the warning must
// name both versions and the path so the user can find the offending binary.
func TestProbeYtdlpShadowed(t *testing.T) {
	issues := probeVersion("2026.03.31", "2026.08.19", "2026.08.19", "pacman")
	if !hasSeverity(issues, config.SeverityWarning, "shadowing") {
		t.Fatalf("shadowed yt-dlp not reported: %+v", issues)
	}
	for _, want := range []string{"2026.03.31", "2026.08.19", "/usr/local/bin/yt-dlp", "pacman"} {
		if !hasSeverity(issues, config.SeverityWarning, want) {
			t.Errorf("shadow warning omits %q: %+v", want, issues)
		}
	}
}

// TestProbeSystemLagging: when the newest version the package manager offers is
// itself far behind the latest release, updating through it cannot help — the
// warning has to say so rather than just "update yt-dlp".
func TestProbeSystemLagging(t *testing.T) {
	issues := probeVersion("2026.05.01", "2026.05.01", "2026.08.19", "apt")
	if !hasSeverity(issues, config.SeverityWarning, "lagging") {
		t.Fatalf("lagging distro package not reported: %+v", issues)
	}
	if !hasSeverity(issues, config.SeverityWarning, "apt") {
		t.Errorf("lag warning must name the package manager: %+v", issues)
	}
	if hasSeverity(issues, config.SeverityWarning, "shadowing") {
		t.Errorf("nothing is shadowing anything here: %+v", issues)
	}
}

// TestProbeBothWarnings: a shadowed binary AND a lagging package manager are two
// separate problems with two separate fixes, so both are reported.
func TestProbeBothWarnings(t *testing.T) {
	issues := probeVersion("2026.01.10", "2026.05.01", "2026.08.19", "pacman")
	if !hasSeverity(issues, config.SeverityWarning, "shadowing") ||
		!hasSeverity(issues, config.SeverityWarning, "lagging") {
		t.Errorf("expected both a shadow and a lag warning: %+v", issues)
	}
}

// TestProbeInstalledBehindUpstream: with no package manager entry (a pip or
// standalone install), the installed version is compared against the latest
// release directly and the advice is the ordinary "update it".
func TestProbeInstalledBehindUpstream(t *testing.T) {
	issues := probeVersion("2026.05.01", "", "2026.08.19", "")
	if !hasSeverity(issues, config.SeverityWarning, "behind the latest release") {
		t.Fatalf("outdated yt-dlp not reported: %+v", issues)
	}
	if hasSeverity(issues, config.SeverityWarning, "lagging") {
		t.Errorf("with no package manager there is no packaging to blame: %+v", issues)
	}
}

// TestProbeInstalledNewerThanPackage: a self-updated yt-dlp ahead of the repo is
// exactly right — neither the shadow nor the lag check may fire.
func TestProbeInstalledNewerThanPackage(t *testing.T) {
	if issues := probeVersion("2026.08.19", "2026.05.01", "2026.08.19", "pacman"); len(issues) != 0 {
		t.Errorf("a yt-dlp newer than the packaged one produced issues: %+v", issues)
	}
}

// TestProbeNoReference: the age of a version says nothing on its own, so with
// neither a package manager nor a cached release to compare against, the probe
// stays silent no matter how old the install looks.
func TestProbeNoReference(t *testing.T) {
	if issues := probeVersion("2024.01.01", "", "", ""); len(issues) != 0 {
		t.Errorf("probe warned with no reference version: %+v", issues)
	}
}

// TestProbeVersionUnreadable: an unreadable or unrecognizable installed version
// is skipped, not surfaced — presence is already confirmed, and startup must
// neither block nor manufacture a warning.
func TestProbeVersionUnreadable(t *testing.T) {
	if issues := probeVersion("not-a-version", "2026.08.19", "2026.08.19", "pacman"); len(issues) != 0 {
		t.Errorf("unreadable version produced issues: %+v", issues)
	}
}

// TestProbeWithinTolerance: a gap smaller than staleLagTolerance is normal
// packaging lag and must stay quiet.
func TestProbeWithinTolerance(t *testing.T) {
	if issues := probeVersion("2026.08.05", "2026.08.05", "2026.08.19", "pacman"); len(issues) != 0 {
		t.Errorf("a two-week-old yt-dlp produced issues: %+v", issues)
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
