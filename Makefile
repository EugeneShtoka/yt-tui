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

# Test-coverage report: total summary to stdout + browsable HTML in coverage.html.
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1
	go tool cover -html=coverage.out -o coverage.html

# Enforce the coverage floors (overall + per-package); see scripts/coverage-gate.sh.
coverage-check:
	go test -coverprofile=coverage.out ./...
	bash scripts/coverage-gate.sh coverage.out

# Run all quality gates locally before pushing.
check: build test lint fmt vuln secrets
