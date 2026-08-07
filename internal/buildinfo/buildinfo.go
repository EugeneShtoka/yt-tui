// Package buildinfo exposes version metadata stamped into the binary.
//
// Release builds (GoReleaser) inject the values via the linker:
//
//	-ldflags "-X github.com/EugeneShtoka/yt-tui/internal/buildinfo.Version=v0.1.0 ..."
//
// For plain `go build` / `go install` builds — where nothing is injected — the
// values fall back to the module build info the Go toolchain embeds, so a
// `go install ...@v0.1.0` still reports v0.1.0 and a local build reports the
// VCS revision. When nothing is available the version reads "dev".
package buildinfo

import "runtime/debug"

// Injected at release build time via -ldflags -X. Leave empty otherwise; the
// accessors below fall back to the embedded module build info.
var (
	Version = ""
	Commit  = ""
	Date    = ""
)

// version returns the release version: the ldflags-injected value if present,
// otherwise the module version the toolchain embedded (e.g. from
// `go install ...@v0.1.0`), otherwise "dev".
func version() string {
	if Version != "" {
		return Version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

// commit returns the short VCS revision: the ldflags-injected value if present,
// otherwise the revision the toolchain embedded for VCS-aware builds.
func commit() string {
	if Commit != "" {
		return Commit
	}
	return vcsSetting("vcs.revision")
}

// date returns the build date: the ldflags-injected value if present, otherwise
// the commit time the toolchain embedded for VCS-aware builds.
func date() string {
	if Date != "" {
		return Date
	}
	return vcsSetting("vcs.time")
}

// vcsSetting reads one of the vcs.* keys the Go toolchain records in the build
// info for VCS-aware builds. Returns "" when unavailable (e.g. GOFLAGS=-buildvcs=false).
func vcsSetting(key string) string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range bi.Settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}

// goVersion returns the Go toolchain version the binary was built with.
func goVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		return bi.GoVersion
	}
	return ""
}

// String renders a multi-line version report for `<name> --version`. Fields
// that resolve to empty (e.g. commit/date on a toolchain build with VCS info
// stripped) are omitted.
func String(name string) string {
	out := name + " " + version()
	if c := commit(); c != "" {
		out += "\n  commit: " + c
	}
	if d := date(); d != "" {
		out += "\n  built:  " + d
	}
	if g := goVersion(); g != "" {
		out += "\n  go:     " + g
	}
	return out
}
