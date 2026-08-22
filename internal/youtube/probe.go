package youtube

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
)

// staleLagTolerance is how far behind a newer known release the host's yt-dlp may
// fall before the probe warns. Some lag is normal and harmless — distro packagers
// cut a release days after upstream — but around a month behind is where
// YouTube's rotating playback signatures start rejecting an old extractor, which
// the user experiences as playback that silently does nothing.
const staleLagTolerance = 30 * 24 * time.Hour

// Probe runs cheap, bounded, strictly-local checks for YouTube readiness and
// returns any problems as non-fatal config issues (empty when all is well). It
// never touches the network: it verifies that the yt-dlp binary is on PATH, that
// nothing older is shadowing the version the host's package manager provides,
// that the newest version the host can reach is not far behind the latest
// release, and that the configured cookie source resolves. The latest-release
// number comes from a cache that RefreshLatestVersion fills in the background, so
// the probe stays instant and offline-safe. The daemon-side counterpart (a health
// RPC for remote mode) is a planned follow-up; today remote clients simply skip
// the probe.
func Probe(ctx context.Context, cfg *config.Config) []config.ConfigIssue {
	return availabilityProbe{
		cfg:       cfg,
		lookPath:  exec.LookPath,
		installed: func() (Version, bool) { return InstalledVersion(ctx) },
		system:    func() (Version, string, bool) { return SystemVersion(ctx) },
		latest:    func() (Version, bool) { return CachedLatestVersion(cfg) },
	}.run()
}

// availabilityProbe holds the probe inputs. Every environment lookup is injected
// (real implementations in production) so the checks are unit-testable without a
// real yt-dlp install, a package manager, or a wall clock. A nil lookup disables
// the check that needs it — used by tests that only exercise the other paths.
type availabilityProbe struct {
	cfg       *config.Config
	lookPath  func(string) (string, error)
	installed func() (Version, bool)         // yt-dlp --version
	system    func() (Version, string, bool) // version + name of the host package manager
	latest    func() (Version, bool)         // newest release, from the cache
}

func (p availabilityProbe) run() []config.ConfigIssue {
	if !p.cfg.YouTubeEnabled {
		return nil // user opted out — there is nothing to warn about
	}
	// Without yt-dlp nothing YouTube-related works; report and stop (the version
	// and cookie checks below would only add noise on top of the root cause).
	path, err := p.lookPath("yt-dlp")
	if err != nil {
		return []config.ConfigIssue{{
			Severity: config.SeverityError,
			Message:  "yt-dlp not found on PATH — YouTube features (feed, search, playback, downloads) are unavailable; install yt-dlp",
		}}
	}
	var issues []config.ConfigIssue
	issues = append(issues, p.checkYtdlpVersion(path)...)
	issues = append(issues, p.checkCookieSource()...)
	return issues
}

// checkYtdlpVersion compares the yt-dlp that will actually run against the two
// references the host can offer: what its package manager provides, and the
// newest release recorded by the background update check. Each comparison is only
// made when its reference exists — no reference means no warning, never a guess
// from the version's age alone, because how long a release stays good is not
// something a date can tell us.
func (p availabilityProbe) checkYtdlpVersion(path string) []config.ConfigIssue {
	if p.installed == nil {
		return nil
	}
	installed, ok := p.installed()
	if !ok {
		debug.Log("probe: yt-dlp version unreadable or unrecognized; skipping version checks")
		return nil
	}
	system, manager, haveSystem := p.systemVersion()
	var issues []config.ConfigIssue
	if haveSystem && installed.Older(system) {
		issues = append(issues, shadowedIssue(installed, system, manager, path))
	}
	// The lag check judges the best yt-dlp this host can currently get: if the
	// packaged one is newer, that is what an update would install.
	best, fromSystem := installed, false
	if haveSystem && installed.Older(system) {
		best, fromSystem = system, true
	} else if haveSystem && !system.Older(installed) {
		fromSystem = true // package manager offers exactly what is installed
	}
	if iss, lagging := p.lagIssue(best, manager, fromSystem); lagging {
		issues = append(issues, iss)
	}
	return issues
}

// systemVersion asks the host package manager what it provides, tolerating a nil
// lookup (tests) as "no package manager knows yt-dlp".
func (p availabilityProbe) systemVersion() (Version, string, bool) {
	if p.system == nil {
		return Version{}, "", false
	}
	return p.system()
}

// shadowedIssue reports a yt-dlp on PATH that is older than the one the host's
// package manager has — almost always a stale copy in an earlier PATH entry
// (/usr/local/bin, ~/.local/bin) quietly winning over the packaged binary, which
// makes "I already updated yt-dlp" and "playback is broken" true at the same time.
func shadowedIssue(installed, system Version, manager, path string) config.ConfigIssue {
	return config.ConfigIssue{
		Severity: config.SeverityWarning,
		Message: fmt.Sprintf(
			"yt-dlp %s at %s is older than the %s %s provides — an out-of-date copy earlier on PATH is shadowing the packaged one; remove it or fix PATH, or playback and downloads will keep failing",
			installed.Raw, path, system.Raw, manager),
	}
}

// lagIssue reports how far the best yt-dlp this host can reach is behind the
// latest release, once that gap passes staleLagTolerance. When the best version
// is the packaged one, updating cannot fix it — the advice has to be to go around
// the package manager — so the two cases say different things.
func (p availabilityProbe) lagIssue(best Version, manager string, fromSystem bool) (config.ConfigIssue, bool) {
	if p.latest == nil {
		return config.ConfigIssue{}, false
	}
	latest, ok := p.latest()
	if !ok {
		return config.ConfigIssue{}, false // no update check has succeeded yet
	}
	behind := best.Behind(latest)
	if behind < staleLagTolerance {
		return config.ConfigIssue{}, false
	}
	days := int(behind.Hours() / 24)
	msg := fmt.Sprintf(
		"yt-dlp %s is %d days behind the latest release (%s) — YouTube rejects playback from an outdated extractor; update it ('yt-dlp -U', pip/pipx, or your package manager)",
		best.Raw, days, latest.Raw)
	if fromSystem {
		msg = fmt.Sprintf(
			"the newest yt-dlp %s offers (%s) is %d days behind the latest release (%s) — your distribution's packaging is lagging, so updating through it will not catch you up; install yt-dlp from upstream (pip/pipx or the standalone build) if playback starts failing",
			manager, best.Raw, days, latest.Raw)
	}
	return config.ConfigIssue{Severity: config.SeverityWarning, Message: msg}, true
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
