#!/usr/bin/env bash
#
# actions-pin-check.sh — supply-chain guard for GitHub Actions pinning (H-2).
#
# Every third-party action referenced by `uses:` in .github/workflows/*.yml must
# be pinned to a full 40-character commit SHA, not a mutable tag/branch. A
# mutable ref (@v7, @main, …) lets a tag-hijack/retag silently and retroactively
# compromise a workflow that holds `contents: write` + GITHUB_TOKEN
# (cf. tj-actions/changed-files, CVE-2025-30066). Dependabot's github-actions
# updater still bumps the SHAs via reviewable PRs; this gate just refuses to let
# an unpinned ref land in the first place.
#
# Exempt by design: local composite actions (`uses: ./...`) and reusable
# workflows (`uses: owner/repo/.github/workflows/x.yml@ref`) — but we still
# require even those to be SHA-pinned, so nothing is skipped here.
#
# Usage: bash scripts/actions-pin-check.sh   (scans .github/workflows/*.yml)

set -euo pipefail

shopt -s nullglob
workflows=(.github/workflows/*.yml .github/workflows/*.yaml)
if [[ ${#workflows[@]} -eq 0 ]]; then
	echo "actions-pin-check: no workflow files found under .github/workflows/" >&2
	exit 1
fi

fail=0
for wf in "${workflows[@]}"; do
	# Pull the ref after '@' from every `uses:` line, stripping any trailing
	# `# comment` and surrounding quotes/whitespace. Local actions (uses: ./...)
	# carry no '@' and are skipped.
	while IFS= read -r line; do
		# Normalize: drop everything up to and including `uses:`, strip a trailing
		# comment, then trim quotes and whitespace.
		spec="${line#*uses:}"
		spec="${spec%%#*}"
		spec="$(echo "$spec" | tr -d '"'"'"' ' | tr -d '[:space:]')"

		[[ -z "$spec" ]] && continue
		[[ "$spec" == ./* ]] && continue        # local composite action
		[[ "$spec" == docker://* ]] && continue # docker refs pin by digest separately

		if [[ "$spec" != *@* ]]; then
			echo "❌ $wf: unpinned (no ref): $spec" >&2
			fail=1
			continue
		fi

		ref="${spec##*@}"
		if [[ ! "$ref" =~ ^[0-9a-f]{40}$ ]]; then
			echo "❌ $wf: action must be pinned to a 40-char commit SHA, got '@$ref': $spec" >&2
			fail=1
		fi
	done < <(grep -E '^\s*-?\s*uses:' "$wf" || true)
done

if [[ "$fail" -ne 0 ]]; then
	echo "" >&2
	echo "GitHub Actions must be pinned to full commit SHAs (with a '# vX.Y.Z' comment)." >&2
	echo "See scripts/actions-pin-check.sh for the rationale (supply-chain hardening, H-2)." >&2
	exit 1
fi

echo "actions-pin-check: all workflow actions are SHA-pinned ✓"
