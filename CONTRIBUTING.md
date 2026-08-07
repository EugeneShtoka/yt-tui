# Contributing to yt-tui

Thanks for your interest in improving yt-tui! This guide covers how to build,
test, and submit changes.

## Prerequisites

- **Go 1.26+** (the module pins the exact toolchain in `go.mod`)
- **[yt-dlp](https://github.com/yt-dlp/yt-dlp)** and a media player (`mpv`
  recommended) to run the app end to end
- **[golangci-lint](https://golangci-lint.run) v2** and
  **[govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck)** for the
  full local gate (CI installs these itself)

## First-time setup

Enable the pre-commit secret-scan hook (recommended — git hooks aren't cloned,
so this is opt-in per checkout):

```sh
make hooks   # sets core.hooksPath to .githooks
```

Install [gitleaks](https://github.com/gitleaks/gitleaks) for full coverage; the
hook falls back to a basic pattern scan without it. The same scan runs in CI on
every push and PR, so a bypassed local hook still gets caught.

## Building and running

The `Makefile` wraps the common tasks:

```sh
make build        # build both ./yt-tui and ./yt-tuid
make run          # go run the TUI client
make run-daemon   # go run the headless daemon
```

There are two binaries: **`yt-tui`** (the TUI client) and **`yt-tuid`** (the
optional headless daemon). Both build the same in-process backend at their core
— see [ARCHITECTURE.md](ARCHITECTURE.md) for the full map of the codebase before
making non-trivial changes.

## Before you open a PR

Run the full local gate — it must pass, and it's exactly what CI runs:

```sh
make check        # build + test (race) + lint + fmt + vuln + secrets
```

Individual steps if you want them separately:

```sh
make test         # go test -race ./...
make lint         # golangci-lint run
make fmt          # golangci-lint fmt (formats in place)
make vuln         # govulncheck ./...
make secrets      # gitleaks full-history secret scan
make fix          # auto-fix: go mod tidy + gofmt + golangci-lint --fix
```

**Add tests for behavioral changes.** The codebase favors table-driven,
behavioral tests (assert on state, not on rendered glyph strings). Pure logic
lives in `internal/domain`, `internal/text`, and `internal/tui/render` and is
expected to stay near-100% covered.

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org). The release
changelog is generated from commit prefixes, so this matters:

- `feat: ...` — a new user-facing capability (grouped under **Features**)
- `fix: ...` — a bug fix (grouped under **Bug fixes**)
- `docs:`, `test:`, `chore:`, `ci:`, `refactor:` — everything else

Keep commits **granular and self-explanatory** — one logical change per commit,
with the *why* in the body when it isn't obvious from the diff. A reviewer (and
future you, via `git blame`) should be able to understand a change without
external context.

## Pull requests

1. Fork and branch off `main`.
2. Make your change; keep it focused (one concern per PR where practical).
3. Ensure `make check` is green and tests cover the change.
4. Open the PR against `main` and fill in the template.

CI (build, race-tests, lint, and a vulnerability scan) runs on every PR and must
pass before merge.

## Releases and versioning

Releases follow [Semantic Versioning](https://semver.org): `vMAJOR.MINOR.PATCH`
(`v`-prefixed). Maintainers cut releases by pushing a `v*` tag, which triggers
the release workflow (GoReleaser builds multi-platform binaries + checksums).
Contributors don't need to bump versions.

## License

By contributing, you agree that your contributions are licensed under the
project's [MIT License](LICENSE).
