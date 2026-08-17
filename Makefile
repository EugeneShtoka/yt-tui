.PHONY: build run run-daemon build-tui build-daemon test lint fmt fix vuln secrets hooks check coverage coverage-check

# Build both runnable binaries (./yt-tui and ./yt-tuid).
build: build-tui build-daemon

build-tui:
	go build -o yt-tui ./cmd/yt-tui/

build-daemon:
	go build -o yt-tuid ./cmd/yt-tuid/

run:
	go run ./cmd/yt-tui/

run-daemon:
	go run ./cmd/yt-tuid/

test:
	go test -race ./...

# golangci-lint and govulncheck are pinned as Go tool dependencies (the go.mod
# `tool` directive, Go 1.24+) and invoked via `go tool`, so their versions are
# reproducible from go.mod/go.sum with no `go install ...@version` drift.
# goreleaser (huge dep tree) and gitleaks (not `go install`-able under its v8
# module path) stay outside the directive — see the release workflow / `secrets`.
GOLANGCI := go tool golangci-lint
GOVULN   := go tool govulncheck

lint:
	$(GOLANGCI) run ./...

fmt:
	$(GOLANGCI) fmt ./...

# Non-mutating formatting check for the pre-push gate and CI: reports diffs
# instead of rewriting files (unlike `fmt`), so `check` can't silently modify
# the tree while CI asserts formatting.
fmt-check:
	$(GOLANGCI) fmt --diff ./...

# Auto-fix everything that can be fixed without human judgment.
fix:
	go mod tidy
	go fmt ./...
	$(GOLANGCI) run --fix ./...

vuln:
	$(GOVULN) ./...

# Scan the full git history for committed secrets (requires gitleaks).
secrets:
	gitleaks detect --redact --no-banner

# Supply-chain guard: every GitHub Actions `uses:` ref must stay SHA-pinned (H-2).
pin-check:
	bash scripts/actions-pin-check.sh

# Enable the tracked pre-commit secret-scan hook for this clone.
hooks:
	git config core.hooksPath .githooks
	@echo "pre-commit secret scan enabled. Install gitleaks for full coverage:"
	@echo "  https://github.com/gitleaks/gitleaks"

# Test-coverage report: total summary to stdout + browsable HTML in coverage.html.
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1
	go tool cover -html=coverage.out -o coverage.html

# Enforce the coverage floors (overall + per-package); see scripts/coverage-gate.sh.
coverage-check:
	go test -coverprofile=coverage.out ./...
	bash scripts/coverage-gate.sh coverage.out

# Run all quality gates locally before pushing. Uses fmt-check (non-mutating) so
# the gate matches CI and never rewrites the tree; run `make fmt` to auto-format.
check: build test lint fmt-check vuln secrets pin-check
