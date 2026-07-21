.PHONY: build run run-daemon build-tui build-daemon test lint fmt fix vuln check

build:
	go build ./...

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
	go fmt ./...
	golangci-lint run --fix ./...

vuln:
	govulncheck ./...

# Run all quality gates locally before pushing.
check: build test lint fmt vuln
