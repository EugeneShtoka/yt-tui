#!/usr/bin/env bash
#
# coverage-gate.sh — enforce test-coverage floors, locally and in CI.
#
# Two gates:
#   1. Overall coverage must be >= TOTAL_MIN (default 40%).
#   2. Every non-exempt package must be >= PKG_MIN (default 35%).
#
# Reads an existing coverage profile (arg 1, default coverage.out). Generate it
# with `go test -coverprofile=coverage.out ./...`, or just run `make
# coverage-check`, which does both.
#
# A non-exempt package with no coverage data (i.e. no test files) counts as 0%
# and fails the gate — so a newly added package must either carry tests or be
# added to the exempt list below with a reason.

set -euo pipefail

MODULE="github.com/EugeneShtoka/yt-tui"
PROFILE="${1:-coverage.out}"
TOTAL_MIN="${TOTAL_MIN:-40}"
PKG_MIN="${PKG_MIN:-35}"

# Packages exempt from the per-package floor, with the reason each is exempt.
# These carry no meaningfully unit-testable logic; keep the list short and
# justified. (There is intentionally no "coverage debt" tier — Phase 0 of
# docs/ARCH-REVIEW-2026-08-07.md raised every real package above the floor.)
exempt=(
	internal/api/backend/v1                   # generated protobuf
	internal/api/backend/v1/backendv1connect  # generated Connect stubs
	internal/api/apitest                      # test-only fakes
	internal/theme                            # declarative styles
	internal/tui/keymap                       # declarative key tables
	internal/tui/component                    # thin view widgets
	internal/tui                              # Program glue / message types
	internal/domain/portability               # pure data types (no executable stmts; JSON contract tested)
	internal/buildinfo                        # version vars
	internal/debug                            # debug logging
	internal/procexec                         # exec wrapper
	cmd/yt-tui                                # main wiring
	cmd/yt-tuid                               # main wiring
)

is_exempt() {
	local p="$1" e
	for e in "${exempt[@]}"; do [[ "$p" == "$e" ]] && return 0; done
	return 1
}

[[ -f "$PROFILE" ]] || { echo "coverage-gate: profile not found: $PROFILE" >&2; exit 2; }

# Aggregate per-package covered/total statements straight from the profile.
# Lines: <import/path/file.go>:<s>.<c>,<e>.<c> <numstmts> <count>
declare -A total covered
while IFS= read -r line; do
	[[ "$line" == mode:* ]] && continue
	path="${line%%:*}"
	rest="${line#*:}"
	pkg="${path%/*}"          # drop filename → package import path
	pkg="${pkg#"$MODULE"/}"   # make module-relative
	read -r _pos stmts cnt <<<"$rest"
	total["$pkg"]=$(( ${total["$pkg"]:-0} + stmts ))
	(( cnt > 0 )) && covered["$pkg"]=$(( ${covered["$pkg"]:-0} + stmts ))
done < "$PROFILE"

fail=0
printf '%-48s %8s\n' "PACKAGE" "COVER"
printf '%-48s %8s\n' "-------" "-----"
while IFS= read -r fq; do
	pkg="${fq#"$MODULE"/}"
	if is_exempt "$pkg"; then
		printf '%-48s %8s\n' "$pkg" "exempt"
		continue
	fi
	t=${total["$pkg"]:-0}
	if (( t == 0 )); then
		printf '%-48s %8s  FAIL (no tests)\n' "$pkg" "0.0%"
		fail=1
		continue
	fi
	c=${covered["$pkg"]:-0}
	pct=$(awk "BEGIN{printf \"%.1f\", 100*$c/$t}")
	if awk "BEGIN{exit !($pct < $PKG_MIN)}"; then
		printf '%-48s %7s%%  FAIL (< %s%%)\n' "$pkg" "$pct" "$PKG_MIN"
		fail=1
	else
		printf '%-48s %7s%%\n' "$pkg" "$pct"
	fi
done < <(go list ./...)

tot=$(go tool cover -func="$PROFILE" | awk '/^total:/{sub(/%/,"",$NF); print $NF}')
echo
echo "overall: ${tot}% (min ${TOTAL_MIN}%), per-package floor ${PKG_MIN}%"
if awk "BEGIN{exit !($tot < $TOTAL_MIN)}"; then
	echo "FAIL: overall coverage ${tot}% is below ${TOTAL_MIN}%"
	fail=1
fi

if (( fail )); then
	echo "coverage-gate: FAILED" >&2
	exit 1
fi
echo "coverage-gate: PASSED"
