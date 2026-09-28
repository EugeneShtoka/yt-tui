.PHONY: build build-tui build-daemon run run-daemon install deploy undeploy \
	test lint fmt fmt-check fix vuln secrets pin-check arch-check mod-check release-check \
	proto-check docs-check hooks check coverage coverage-check tools pins-outdated \
	compile cross ci-parity ensure-tools vet script-check proto

# ── The same environment as CI ───────────────────────────────────────────────
# Every gate runs what CI runs, with what CI runs it with:
#  - Go: exactly go.mod's `toolchain` line (downloaded once), not whatever go is on
#    PATH, which may be newer. GOTOOLCHAIN=local opts out, for offline work.
#  - No user `go env` file: CI has none, and one can carry GOEXPERIMENT or GOFLAGS
#    that change the build.
#  - golangci-lint, govulncheck, deadcode and the protoc plugins: the go.mod `tool`
#    directive.
#  - goreleaser, gitleaks, buf and markdownlint-cli2: tools/versions.env and
#    tools/markdownlint's lock, installed into .tools by scripts/ensure-tools.sh,
#    which CI runs too.
# scripts/ci-parity-check.sh fails when a `check` gate is not a CI step or back.
GO_TOOLCHAIN := $(shell awk '$$1 == "toolchain" {print $$2}' go.mod)
export GOTOOLCHAIN := $(GO_TOOLCHAIN)
export GOENV := off
export PATH := $(CURDIR)/.tools/bin:$(PATH)
ENSURE   := bash scripts/ensure-tools.sh
GOLANGCI := go tool golangci-lint
GOVULN   := go tool govulncheck

# CHECK_STRICT=1 (the pre-push hook) fails a gate that would otherwise be skipped
# here, which is only the history secret scan outside the project's own repository.
CHECK_STRICT ?=

# ── Build ────────────────────────────────────────────────────────────────────
# Built as released: CGO_ENABLED=0 (modernc.org/sqlite is pure Go). `test` keeps
# cgo: -race needs it. Outside its own repository go would stamp that repository's
# commit into the binaries, so vcs stamping is off there.
OWN_REPO := $(shell [ "$$(git rev-parse --show-toplevel 2>/dev/null)" = "$(CURDIR)" ] && echo yes)
BUILDVCS := $(if $(OWN_REPO),,-buildvcs=false)
GOBUILD := CGO_ENABLED=0 go build $(BUILDVCS)
GORUN   := CGO_ENABLED=0 go run $(BUILDVCS)

# Every package, as released (CI's Build step).
compile:
	$(GOBUILD) ./...

# The release targets (.goreleaser.yaml). No windows: the player package uses the
# Unix-only syscall.SysProcAttr.Setsid. CI builds one target per matrix leg by
# setting CROSS_TARGETS=goos/goarch in the environment.
CROSS_TARGETS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
cross:
	@set -e; for t in $(CROSS_TARGETS); do \
		echo "cross: $$t"; GOOS=$${t%/*} GOARCH=$${t#*/} $(GOBUILD) ./...; \
	done

# Both runnable binaries (./yt-tui and ./yt-tuid).
build: build-tui build-daemon

build-tui:
	$(GOBUILD) -o yt-tui ./cmd/yt-tui/

build-daemon:
	$(GOBUILD) -o yt-tuid ./cmd/yt-tuid/

# ── Run from source ──────────────────────────────────────────────────────────
run:
	$(GORUN) ./cmd/yt-tui/

run-daemon:
	$(GORUN) ./cmd/yt-tuid/

# One coverage profile per make invocation, so concurrent runs (agents, hooks, a
# developer) never write the same file. Pass COVERPROFILE=path to choose one.
# Named by make's PID: unique per invocation, and nothing is created until go test
# writes it.
ifndef COVERPROFILE
COVERPROFILE := $(or $(TMPDIR),/tmp)/yt-tui-cover-$(shell echo $$PPID).out
endif

# -count=1: never serve cached results. The profile feeds coverage-check.
test:
	go test -race -count=1 -coverprofile=$(COVERPROFILE) ./...

# --allow-serial-runners: a second concurrent run waits for the first instead of
# failing with "parallel golangci-lint is running".
lint:
	$(GOLANGCI) run --allow-serial-runners ./...

fmt:
	$(GOLANGCI) fmt ./...

# Reports diffs without rewriting (for check/CI).
fmt-check:
	$(GOLANGCI) fmt --diff ./...

# go vet alone (the pre-commit hook; lint runs it too, with more).
vet:
	go vet ./...

# Auto-fix everything that can be fixed without human judgment.
fix:
	go mod tidy
	go fmt ./...
	$(GOLANGCI) run --fix ./...

# Go modules, and the one npm tree the gates run (markdownlint-cli2's lock).
vuln:
	$(GOVULN) ./...
	cd tools/markdownlint && npm audit --audit-level=low --omit=dev

# Regenerate api/proto codegen (needs buf). Output is committed.
proto:
	@$(ENSURE) buf
	buf lint
	buf generate

# Scan history (only when this dir is its own repo root) and the working tree.
secrets:
	@$(ENSURE) gitleaks
	@if [ -n "$(OWN_REPO)" ]; then \
		gitleaks git --redact --no-banner; \
	elif [ -n "$(CHECK_STRICT)" ]; then \
		echo "secrets: FAILED — no history to scan: $(CURDIR) is not the root of its own git repository." >&2; \
		exit 1; \
	else \
		echo "secrets: history scan SKIPPED — $(CURDIR) is not the root of its own git repository."; \
	fi
	gitleaks dir --redact --no-banner

# Supply-chain guard: every GitHub Actions `uses:` ref must stay SHA-pinned.
pin-check:
	bash scripts/actions-pin-check.sh

# The tools go.mod cannot pin, at tools/versions.env's versions (cheap when current).
ensure-tools:
	@$(ENSURE)

proto-check:
	@$(ENSURE) buf
	PROTO_CHECK_REQUIRED=1 bash scripts/proto-check.sh

# .goreleaser.yaml is valid, checked by the goreleaser the release runs.
release-check:
	@$(ENSURE) goreleaser
	goreleaser check

# docs/ is gitignored (internal planning notes), so only the published Markdown.
docs-check:
	@$(ENSURE) markdownlint
	markdownlint-cli2 "*.md" ".github/**/*.md"

# go.mod/go.sum are tidy and every module matches its checksum.
mod-check:
	go mod tidy -diff
	go mod verify

# Every `check` gate is a CI step, and every CI make target is a `check` gate.
ci-parity:
	bash scripts/ci-parity-check.sh

# The gate scripts' own tests (ensure-tools, ci-parity), offline.
script-check:
	bash scripts/test/tools-test.sh

# Every package must be named by a depguard rule, the binaries must link only what
# their layer allows, and no function may be reachable only from tests (tests build
# their own fixtures; scripts/deadcode-allow.txt lists the reasoned exceptions).
arch-check:
	bash scripts/depguard-coverage-check.sh
	bash scripts/deps-check.sh
	bash scripts/deadcode-check.sh

# Enable the tracked pre-commit/pre-push hooks for this clone.
hooks:
	git config core.hooksPath .githooks
	@echo "hooks enabled: secrets + fmt + vet before a commit, secrets + make check before a push."
	@echo "Install gitleaks for full secret-scan coverage:"
	@echo "  https://github.com/gitleaks/gitleaks"

# The tools the workflows pin by hand (Dependabot does not bump them) against their
# latest releases, and the bubbletea fork against upstream. Needs network.
pins-outdated:
	bash scripts/pinned-tools.sh

# Print the pinned versions of every tool (sanity check).
tools: ensure-tools
	@go version
	@go tool golangci-lint version
	@go tool govulncheck -version | head -2
	@goreleaser --version | grep -m1 GitVersion
	@echo "gitleaks $$(gitleaks version)"
	@echo "buf $$(buf --version)"
	@markdownlint-cli2 --help | head -1

# Test-coverage report: total summary to stdout + browsable HTML in coverage.html.
coverage:
	go test -count=1 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1
	go tool cover -html=coverage.out -o coverage.html

# Floors in scripts/coverage-gate.sh; reuses the profile from `test`.
coverage-check: test
	bash scripts/coverage-gate.sh $(COVERPROFILE); status=$$?; rm -f $(COVERPROFILE); exit $$status

# Every CI gate, with CI's tools (ci-parity-check.sh holds the two lists equal).
# Non-mutating. CHECK_STRICT=1 also fails what would be skipped here.
check: mod-check compile coverage-check cross lint fmt-check vuln secrets pin-check \
	arch-check ci-parity script-check release-check proto-check docs-check

# ── Deploy ───────────────────────────────────────────────────────────────────
# Default install is per-user ($HOME/.local/bin); `make deploy PREFIX=/usr/local`
# for machine-wide (sudo only if BINDIR is not writable).
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
BINARIES := yt-tui yt-tuid

# Build and copy the binaries into BINDIR. `install` replaces each file (unlink +
# create), so a yt-tui that is running keeps its old binary.
install: build
	@set -eu; \
	if { [ -d "$(BINDIR)" ] && [ -w "$(BINDIR)" ]; } || mkdir -p "$(BINDIR)" 2>/dev/null; then SUDO=""; \
	else SUDO="sudo"; echo "==> $(BINDIR) is not writable — installing the binaries with sudo"; sudo mkdir -p "$(BINDIR)"; fi; \
	for b in $(BINARIES); do $$SUDO install -m755 "$$b" "$(BINDIR)/$$b"; done; \
	echo "==> binaries -> $(foreach b,$(BINARIES),$(BINDIR)/$(b))"; \
	case ":$$PATH:" in *":$(BINDIR):"*) ;; *) echo "!!  $(BINDIR) is not on PATH" ;; esac; \
	"$(BINDIR)/yt-tui" --version

# Build and install, with no gates (as in mx-tui): the pre-push hook and CI run
# `make check`. There is no local daemon unit to restart — yt-tui runs its backend
# in-process, and yt-tuid is deployed as a system service (deploy/yt-tuid.service).
deploy: install

# Remove what `install` wrote. Config, database and caches are left in place.
undeploy:
	@set -eu; \
	if [ -w "$(BINDIR)" ]; then SUDO=""; else SUDO="sudo"; fi; \
	for b in $(BINARIES); do \
	  if [ -e "$(BINDIR)/$$b" ]; then $$SUDO rm -f "$(BINDIR)/$$b"; echo "==> removed $(BINDIR)/$$b"; fi; \
	done; \
	echo "    config, database and caches were left in place"
