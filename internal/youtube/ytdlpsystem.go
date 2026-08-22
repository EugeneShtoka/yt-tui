package youtube

import (
	"context"
	"os/exec"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/debug"
)

// systemQuery asks one host package manager which yt-dlp version it can provide.
// Every command reads the manager's own on-disk index — none of them refreshes
// over the network — so the whole lookup stays inside the probe's local-only
// contract: pacman reads its sync db, apt-cache its lists, and dnf/zypper are
// pinned to their caches explicitly.
type systemQuery struct {
	manager string // name shown to the user
	bin     string // binary that must be on PATH for this entry to apply
	args    []string
	values  func(out string) []string // every version string the output offers
}

// systemQueries are tried in order; the first manager whose binary exists and
// whose output yields a parseable version wins. pacman appears twice because
// -Si only knows repository packages: an AUR or locally built yt-dlp exists only
// in the local database, which -Qi reports.
var systemQueries = []systemQuery{
	{manager: "pacman", bin: "pacman", args: []string{"-Si", "yt-dlp"}, values: colonField("Version")},
	{manager: "pacman", bin: "pacman", args: []string{"-Qi", "yt-dlp"}, values: colonField("Version")},
	{manager: "apt", bin: "apt-cache", args: []string{"policy", "yt-dlp"}, values: colonField("Candidate")},
	{manager: "dnf", bin: "dnf", args: []string{"--cacheonly", "info", "yt-dlp"}, values: colonField("Version")},
	{manager: "zypper", bin: "zypper", args: []string{"--no-refresh", "info", "yt-dlp"}, values: colonField("Version")},
	{manager: "brew", bin: "brew", args: []string{"info", "--formula", "yt-dlp"}, values: brewStable},
}

// SystemVersion reports the newest yt-dlp the host's package manager offers,
// along with the manager's name. ok=false means no supported manager knows the
// package — an ordinary outcome for a pip, pipx or standalone-binary install,
// where the only reference left is the upstream release.
func SystemVersion(ctx context.Context) (Version, string, bool) {
	for _, q := range systemQueries {
		if _, err := exec.LookPath(q.bin); err != nil {
			continue
		}
		out, err := runLocal(ctx, q.bin, q.args...)
		if err != nil {
			debug.Log("probe: %s %v: %v", q.bin, q.args, err)
			continue
		}
		if best, ok := newestVersion(q.values(out)); ok {
			return best, q.manager, true
		}
	}
	return Version{}, "", false
}

// newestVersion returns the newest parseable version among raws. Managers can
// list several (dnf prints installed and available sections, pacman one entry per
// repository) and the interesting one is always the newest they could install.
func newestVersion(raws []string) (Version, bool) {
	var best Version
	var found bool
	for _, raw := range raws {
		ver, ok := ParseVersion(raw)
		if !ok {
			continue
		}
		if !found || best.Older(ver) {
			best, found = ver, true
		}
	}
	return best, found
}

// colonField returns a parser for the "Label : value" shape pacman, dnf, zypper
// and apt-cache all print, collecting the value of every matching line.
func colonField(label string) func(string) []string {
	return func(out string) []string {
		var vals []string
		for _, line := range strings.Split(out, "\n") {
			name, val, ok := strings.Cut(line, ":")
			if !ok || strings.TrimSpace(name) != label {
				continue
			}
			if v := strings.TrimSpace(val); v != "" {
				vals = append(vals, v)
			}
		}
		return vals
	}
}

// brewStable pulls the version out of brew's header line, which reads
// "==> yt-dlp: stable 2026.08.19 (bottled), HEAD".
func brewStable(out string) []string {
	var vals []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		for i, f := range fields {
			if f == "stable" && i+1 < len(fields) {
				vals = append(vals, strings.Trim(fields[i+1], ","))
			}
		}
	}
	return vals
}
