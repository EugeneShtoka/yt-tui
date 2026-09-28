#!/usr/bin/env bash
#
# deps-check.sh — what each binary and core package links, transitively. depguard
# sees one file's direct imports; this sees the whole graph, so a package two hops
# away cannot carry the TUI into the headless daemon, or infrastructure into the
# pure layers.
#
# The TUI is deliberately absent: in single-binary mode it reaches the in-process
# backend (api.InProc -> db, youtube, downloader) through the seam, so a transitive
# rule would be false. depguard's tui-no-infra holds its direct imports instead.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

mod=github.com/EugeneShtoka/yt-tui
# What draws a terminal UI: Charm's libraries, and the packages built on them.
drawing="^(charm\.land/|github\.com/charmbracelet/|github\.com/evertras/|$mod/internal/(tui|theme)(/|$))"
# Persistence, the yt-dlp adapter and the download queue.
infra="^($mod/internal/(db|youtube|downloader|backend)|modernc\.org/sqlite)(/|$)"

status=0
check() {
	local pkg=$1 pattern=$2 why=$3 found
	found="$(CGO_ENABLED=0 go list -deps "$pkg" | grep -E "$pattern" || true)"
	if [[ -n $found ]]; then
		echo "deps-check: $pkg links what it must not ($why):" >&2
		sed 's/^/  /' <<<"$found" >&2
		status=1
	fi
}

check ./cmd/yt-tuid "$drawing" "the daemon is headless: it draws nothing"
check ./internal/backend/... "$drawing" "daemon-side services run headless"
check ./internal/domain/... "$infra" "domain is pure logic, no I/O"
check ./internal/domain/... "$drawing" "domain is pure logic, no UI"
check ./internal/api/backend/v1/... "$infra" "the wire types map to domain only"

((status == 0)) && echo "deps-check: OK"
exit $status
