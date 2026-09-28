# Contributing to yt-tui

Thanks for your interest in improving yt-tui! This guide covers how to build,
test, and submit changes.

## Prerequisites

- **Go 1.26+** (the module pins the exact toolchain in `go.mod`)
- **[yt-dlp](https://github.com/yt-dlp/yt-dlp)** and a media player (`mpv`
  recommended) to run the app end to end
- **golangci-lint**, **govulncheck**, **deadcode** and the **protoc plugins**
  are pinned as Go tool dependencies (the `go.mod` `tool` directive) and invoked
  via `go tool`: no separate install needed
- **goreleaser**, **gitleaks**, **buf** and **markdownlint-cli2** are pinned in
  `tools/versions.env` (and `tools/markdownlint`'s lock) and installed into
  `.tools/` by `scripts/ensure-tools.sh`, checksum-verified, the first time a gate
  needs them. markdownlint needs node and npm
- Every gate runs with go.mod's `toolchain` and no `go env` file, the same as CI,
  so a local pass means a CI pass

## First-time setup

Enable the git hooks (recommended — git hooks aren't cloned, so this is opt-in
per checkout):

```sh
make hooks   # sets core.hooksPath to .githooks
```

- **pre-commit**: secret scan of the staged changes, then `fmt-check` and
  `go vet` over the staged snapshot when Go files are staged.
- **pre-push**: secret scan, then the whole `make check CHECK_STRICT=1`, so a
  push does not fail in CI for something this machine could have said.

Both fall back to a basic pattern scan without
[gitleaks](https://github.com/gitleaks/gitleaks). The same scan runs in CI on
every push and PR, so a bypassed local hook still gets caught.

## Building and running

The `Makefile` wraps the common tasks:

```sh
make build        # build both ./yt-tui and ./yt-tuid (CGO_ENABLED=0, as released)
make run          # go run the TUI client
make run-daemon   # go run the headless daemon
make install      # build + copy both binaries to ~/.local/bin (no gates)
make deploy       # make check, then make install — the one command
make undeploy     # remove the installed binaries (config and data are kept)
```

`make deploy PREFIX=/usr/local` installs machine-wide (sudo only when the target
isn't writable).

There are two binaries: **`yt-tui`** (the TUI client) and **`yt-tuid`** (the
optional headless daemon). Both build the same in-process backend at their core
— see [ARCHITECTURE.md](ARCHITECTURE.md) for the full map of the codebase before
making non-trivial changes.

## Before you open a PR

Run the full local gate — it must pass, and it's exactly what CI runs:

```sh
make check        # every CI gate, with CI's toolchain and tools (non-mutating)
```

`scripts/ci-parity-check.sh` (the `ci-parity` gate) keeps `make check` and the CI
workflow running exactly the same targets. Individual gates:

```sh
make mod-check      # go.mod/go.sum tidy and verified
make compile        # every package builds as released
make coverage-check # go test -race + coverage floors (scripts/coverage-gate.sh)
make cross          # the release platforms
make lint           # golangci-lint run (make fmt formats in place)
make fmt-check      # formatting diff, non-mutating
make vuln           # govulncheck + npm audit of markdownlint's lock
make secrets        # gitleaks: full history + working tree
make pin-check      # every GitHub Actions ref is SHA-pinned
make arch-check     # depguard names every package, deps-check, deadcode
make ci-parity      # make check == CI
make script-check   # the gate scripts' own tests
make release-check  # goreleaser check
make proto-check    # buf lint + generated code in sync (make proto regenerates)
make docs-check     # markdownlint
make fix            # auto-fix: go mod tidy + gofmt + golangci-lint --fix
make pins-outdated  # hand-pinned tools and the bubbletea fork vs. upstream (network)
```

`arch-check` fails on a function only tests reach (`go tool deadcode`): move it
into a `_test.go` file or a `*test` package, delete it, or add it to
`scripts/deadcode-allow.txt` with the reason.

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
