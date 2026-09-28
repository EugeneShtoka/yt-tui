#!/usr/bin/env bash
#
# deadcode-check.sh — no function may be reachable only from tests: tests build their
# own fixtures, and code no binary runs is not code. The reasoned exceptions live in
# scripts/deadcode-allow.txt; an entry deadcode no longer reports fails too, so the
# list cannot rot.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

allow="$(sed -E 's/#.*//; s/[[:space:]]+$//; /^$/d' scripts/deadcode-allow.txt | sort -u)"
out="$(CGO_ENABLED=0 go tool deadcode ./cmd/...)"
found="$(sed -nE 's/.*unreachable func: (.*)$/\1/p' <<<"$out" | sort -u)"

status=0
unexpected="$(comm -23 <(echo "$found") <(echo "$allow") | sed '/^$/d')"
if [[ -n "$unexpected" ]]; then
	echo "deadcode-check: functions only tests reach (move them into a _test.go file, or delete them):" >&2
	# The full deadcode lines (with positions) for just the unexpected functions.
	while IFS= read -r fn; do
		grep -F "unreachable func: $fn" <<<"$out" | sed 's/^/  /' >&2
	done <<<"$unexpected"
	status=1
fi
stale="$(comm -13 <(echo "$found") <(echo "$allow") | sed '/^$/d')"
if [[ -n "$stale" ]]; then
	echo "deadcode-check: scripts/deadcode-allow.txt names what deadcode no longer reports; drop:" >&2
	sed 's/^/  /' <<<"$stale" >&2
	status=1
fi
((status == 0)) && echo "deadcode-check: nothing is reachable only from tests (beyond $(wc -l <<<"$allow") allowed) ✓"
exit $status
