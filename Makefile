.PHONY: build test lint vuln check

build:
	go build ./...

test:
	go test -race ./...

lint:
	golangci-lint run ./...

vuln:
	govulncheck ./...

# Run all quality gates locally before pushing.
check: build test lint vuln
