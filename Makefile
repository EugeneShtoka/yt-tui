.PHONY: build run run-daemon build-tui build-daemon test lint fmt fix vuln secrets hooks check coverage

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

lint:
	golangci-lint run ./...

fmt:
	golangci-lint fmt ./...

# Auto-fix everything that can be fixed without human judgment.
fix:
	go mod tidy
	go fmt ./...
	golangci-lint run --fix ./...

vuln:
	govulncheck ./...

# Scan the full git history for committed secrets (requires gitleaks).
secrets:
	gitleaks detect --redact --no-banner

# Enable the tracked pre-commit secret-scan hook for this clone.
hooks:
	git config core.hooksPath .githooks
	@echo "pre-commit secret scan enabled. Install gitleaks for full coverage:"
	@echo "  https://github.com/gitleaks/gitleaks"

# Generate a test-coverage report as lcov for Repowise. Go only emits its native
# coverprofile, so scripts/cov2lcov.py converts it to coverage/lcov.info (which
# `repowise coverage add` auto-discovers). Fold it into the health scores with:
#   make coverage && repowise coverage add && repowise update
coverage:
	go test -coverprofile=coverage.out ./...
	python3 scripts/cov2lcov.py

# Run all quality gates locally before pushing.
check: build test lint fmt vuln secrets
