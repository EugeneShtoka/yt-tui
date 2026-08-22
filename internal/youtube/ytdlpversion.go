package youtube

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// versionExecTimeout bounds every local version lookup (yt-dlp itself and the
// package-manager queries). These run on the startup path, so a wedged binary
// must cost a bounded wait rather than hanging the launch.
const versionExecTimeout = 3 * time.Second

// Version is a parsed yt-dlp version. yt-dlp releases are named by date
// (YYYY.MM.DD, plus a time component on nightlies), so the release date is the
// whole ordering — comparing two versions is comparing their dates.
type Version struct {
	Raw  string    // as printed by yt-dlp or by a package manager
	Date time.Time // release date parsed out of Raw
}

// ParseVersion parses a yt-dlp version string into its release date. It accepts
// what yt-dlp prints (2026.08.19, or 2026.08.19.123456 on a nightly) and what
// package managers wrap around it: an epoch prefix (1:2026.08.19) and a
// packaging-release suffix (2026.08.19-1). Anything else returns ok=false, so an
// unusual build yields no comparison rather than a bogus one.
func ParseVersion(s string) (Version, bool) {
	raw := strings.TrimSpace(s)
	core := raw
	if i := strings.LastIndex(core, ":"); i >= 0 {
		core = core[i+1:] // drop a distro epoch
	}
	if i := strings.IndexAny(core, "-+_~"); i >= 0 {
		core = core[:i] // drop a packaging release or local-build suffix
	}
	parts := strings.Split(core, ".")
	if len(parts) < 3 {
		return Version{}, false
	}
	year, errY := strconv.Atoi(parts[0])
	month, errM := strconv.Atoi(parts[1])
	day, errD := strconv.Atoi(parts[2])
	if errY != nil || errM != nil || errD != nil {
		return Version{}, false
	}
	if year < 2000 || month < 1 || month > 12 || day < 1 || day > 31 {
		return Version{}, false
	}
	return Version{Raw: raw, Date: time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)}, true
}

// Age is how long ago v was released, as of now.
func (v Version) Age(now time.Time) time.Duration { return now.Sub(v.Date) }

// Older reports whether v was released before other.
func (v Version) Older(other Version) bool { return v.Date.Before(other.Date) }

// Behind is how far v's release date falls before other's, and 0 when v is at
// least as new as other.
func (v Version) Behind(other Version) time.Duration {
	if !v.Older(other) {
		return 0
	}
	return other.Date.Sub(v.Date)
}

// InstalledVersion reports the version of the yt-dlp on PATH via a bounded local
// exec. It returns ok=false when the binary cannot be run or prints something
// unrecognizable — callers treat that as "no reference", never as a problem.
func InstalledVersion(ctx context.Context) (Version, bool) {
	out, err := runLocal(ctx, "yt-dlp", "--version")
	if err != nil {
		return Version{}, false
	}
	return ParseVersion(out)
}

// runLocal executes a local, read-only version query with a bounded timeout and
// returns its stdout. Stderr is dropped: these commands are probed
// opportunistically, and a missing package is an ordinary outcome, not an error
// worth surfacing.
func runLocal(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, versionExecTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return "", fmt.Errorf("runLocal %s: %w", name, err)
	}
	return string(out), nil
}
