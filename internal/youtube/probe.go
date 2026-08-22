package youtube

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
)

// ytdlpStaleAfter is how old an installed yt-dlp may be before the probe warns.
// yt-dlp is date-versioned and YouTube routinely rotates its playback signing
// and throttles stale extractors, so a build much older than this typically
// still resolves metadata but then fails to fetch the actual stream (HTTP 403) —
// which surfaces to the user as playback that silently does nothing.
const ytdlpStaleAfter = 45 * 24 * time.Hour

// Probe runs cheap, bounded, strictly-local checks for YouTube readiness and
// returns any problems as non-fatal config issues (empty when all is well). It
// never touches the network — it only verifies that the yt-dlp binary is on
// PATH, is not badly outdated, and that the configured cookie source resolves,
// so it is safe on the startup path. The daemon-side counterpart (a health RPC
// for remote mode) is a planned follow-up; today remote clients simply skip the
// probe.
func Probe(cfg *config.Config) []config.ConfigIssue {
	return availabilityProbe{
		cfg:        cfg,
		lookPath:   exec.LookPath,
		runVersion: defaultYtdlpVersion,
		now:        time.Now,
	}.run()
}

// availabilityProbe holds the probe inputs. lookPath, runVersion and now are
// injected (real implementations in production) so the binary-presence and
// staleness checks are unit-testable without a real yt-dlp install or a wall
// clock. A nil runVersion or now disables the version check — used by tests that
// only exercise the presence/cookie paths.
type availabilityProbe struct {
	cfg        *config.Config
	lookPath   func(string) (string, error)
	runVersion func() (string, error)
	now        func() time.Time
}

func (p availabilityProbe) run() []config.ConfigIssue {
	if !p.cfg.YouTubeEnabled {
		return nil // user opted out — there is nothing to warn about
	}
	// Without yt-dlp nothing YouTube-related works; report and stop (the version
	// and cookie checks below would only add noise on top of the root cause).
	if _, err := p.lookPath("yt-dlp"); err != nil {
		return []config.ConfigIssue{{
			Severity: config.SeverityError,
			Message:  "yt-dlp not found on PATH — YouTube features (feed, search, playback, downloads) are unavailable; install yt-dlp",
		}}
	}
	var issues []config.ConfigIssue
	issues = append(issues, p.checkYtdlpVersion()...)
	issues = append(issues, p.checkCookieSource()...)
	return issues
}

// checkYtdlpVersion warns when the installed yt-dlp is old enough that YouTube
// is likely to reject its stream URLs. It reads the version with a bounded local
// exec; a read or parse failure is logged and skipped rather than reported —
// presence is already confirmed, so an unreadable version must not block startup
// or manufacture a bogus warning.
func (p availabilityProbe) checkYtdlpVersion() []config.ConfigIssue {
	if p.runVersion == nil || p.now == nil {
		return nil
	}
	out, err := p.runVersion()
	if err != nil {
		debug.Log("probe: yt-dlp --version: %v", err)
		return nil
	}
	ver := strings.TrimSpace(out)
	released, ok := parseYtdlpDate(ver)
	if !ok {
		debug.Log("probe: unrecognized yt-dlp version %q", ver)
		return nil
	}
	age := p.now().Sub(released)
	if age < ytdlpStaleAfter {
		return nil
	}
	return []config.ConfigIssue{{
		Severity: config.SeverityWarning,
		Message: fmt.Sprintf(
			"yt-dlp %s is %d days old — YouTube may reject playback and downloads with an outdated extractor; update it (e.g. 'yt-dlp -U' or via your package manager)",
			ver, int(age.Hours()/24)),
	}}
}

// parseYtdlpDate parses yt-dlp's date-based version string (YYYY.MM.DD, with an
// optional trailing time/suffix component on nightly builds) into its release
// date. It returns ok=false for any string it doesn't recognize so an unusual
// build never yields a spurious staleness warning.
func parseYtdlpDate(ver string) (time.Time, bool) {
	parts := strings.Split(ver, ".")
	if len(parts) < 3 {
		return time.Time{}, false
	}
	y, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	d, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return time.Time{}, false
	}
	if y < 2000 || m < 1 || m > 12 || d < 1 || d > 31 {
		return time.Time{}, false
	}
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC), true
}

// defaultYtdlpVersion returns the installed yt-dlp's version string via a
// bounded local exec. The timeout guards the startup path against a wedged
// binary — the probe must stay fast and never hang.
func defaultYtdlpVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "yt-dlp", "--version").Output()
	if err != nil {
		return "", fmt.Errorf("defaultYtdlpVersion: %w", err)
	}
	return string(out), nil
}

// checkCookieSource validates the resolved cookie source with local-only I/O.
// cookies_file takes priority over browser (mirroring runner.buildArgs); a
// browser source can't be cheaply verified beyond yt-dlp's presence, so it is
// accepted here and any keyring failure surfaces lazily at first fetch.
func (p availabilityProbe) checkCookieSource() []config.ConfigIssue {
	const prefix = "YouTube access is allowed but unavailable: "
	switch {
	case p.cfg.CookiesFile != "":
		if _, err := os.Stat(p.cfg.CookiesFile); err != nil {
			return []config.ConfigIssue{{
				Severity: config.SeverityError,
				Message:  prefix + "cookies_file " + p.cfg.CookiesFile + " could not be read (" + err.Error() + ")",
			}}
		}
		if _, sapisid, err := parseCookieFile(p.cfg.CookiesFile); err != nil || sapisid == "" {
			return []config.ConfigIssue{{
				Severity: config.SeverityError,
				Message:  prefix + "cookies_file has no SAPISID cookie — export it while logged in to YouTube",
			}}
		}
		return nil
	case p.cfg.Browser != "":
		return nil // yt-dlp present + a browser source is all we can check locally
	default:
		return []config.ConfigIssue{{
			Severity: config.SeverityWarning,
			Message:  "no cookie source configured (set browser or cookies_file) — YouTube features will be unavailable until one is set",
		}}
	}
}
