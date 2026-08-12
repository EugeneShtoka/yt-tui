package youtube

import (
	"os"
	"os/exec"

	"github.com/EugeneShtoka/yt-tui/internal/config"
)

// Probe runs cheap, bounded, strictly-local checks for YouTube readiness and
// returns any problems as non-fatal config issues (empty when all is well). It
// never touches the network — it only verifies that the yt-dlp binary is on
// PATH and that the configured cookie source resolves, so it is safe on the
// startup path. The daemon-side counterpart (a health RPC for remote mode) is a
// planned follow-up; today remote clients simply skip the probe.
func Probe(cfg *config.Config) []config.ConfigIssue {
	return availabilityProbe{cfg: cfg, lookPath: exec.LookPath}.run()
}

// availabilityProbe holds the probe inputs. lookPath is injected (exec.LookPath
// in production) so the binary-presence check is unit-testable without a real
// yt-dlp install.
type availabilityProbe struct {
	cfg      *config.Config
	lookPath func(string) (string, error)
}

func (p availabilityProbe) run() []config.ConfigIssue {
	if !p.cfg.YouTubeEnabled {
		return nil // user opted out — there is nothing to warn about
	}
	// Without yt-dlp nothing YouTube-related works; report and stop (the cookie
	// checks below would only add noise on top of the root cause).
	if _, err := p.lookPath("yt-dlp"); err != nil {
		return []config.ConfigIssue{{
			Severity: config.SeverityError,
			Message:  "yt-dlp not found on PATH — YouTube features (feed, search, playback, downloads) are unavailable; install yt-dlp",
		}}
	}
	return p.checkCookieSource()
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
