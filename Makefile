# Makefile for the logmind Go rewrite (v1.0).
#
# The Python v0.6.x package still lives in src/logmind/ and is shipped to
# PyPI from main. This Makefile only targets the Go binary; Python's
# `pytest` + `pip` commands stay invoked directly from CI / pre-commit
# until the v1.0 cutover.

GO          ?= go
BIN_DIR     ?= bin
BIN_NAME    ?= logmind
BIN         := $(BIN_DIR)/$(BIN_NAME)
# Explicit package list rather than ./... so we skip the unrelated Go
# code that ships inside site/node_modules/flatted/. Future packages
# under cmd/ or internal/ get picked up automatically by the wildcards.
PKG         := ./cmd/... ./internal/...
CMD_PKG     := ./cmd/logmind

# Pass `make GO_TEST_FLAGS='-run TestVersion'` to scope a local run.
GO_TEST_FLAGS ?=

.PHONY: help
help:
	@echo "logmind Go targets (v1-go-rewrite branch):"
	@echo ""
	@echo "  make build           Build $(BIN) from $(CMD_PKG)"
	@echo "  make test            Run all Go tests"
	@echo "  make snapshot        Regenerate testdata/*.golden from current Go output"
	@echo "  make verify-parity   Compare Go binary output to Python v0.6.14 (placeholder)"
	@echo "  make tidy            Run go mod tidy"
	@echo "  make clean           Remove $(BIN_DIR)/"
	@echo ""
	@echo "Python tooling (pytest, pip install -e) is unchanged — see pyproject.toml."

.PHONY: build
build: | $(BIN_DIR)
	$(GO) build -o $(BIN) $(CMD_PKG)

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

.PHONY: test
test:
	$(GO) test $(GO_TEST_FLAGS) $(PKG)

# `make snapshot` rewrites every testdata/*.golden file from the current
# Go output. Use during development after intentionally changing a
# command's stdout shape; commit the regenerated goldens alongside the
# change so CI's `make test` stays green.
#
# Implementation note: `-update` is a custom flag registered by the
# snapshot tests. Passing it to packages that DON'T register the flag
# would fail the test binary with "flag provided but not defined: -update"
# (Go exits 2 on unrecognised flags). So we explicitly enumerate the
# packages that ship golden files. When a future wave adds testdata/
# under a new package, add its import path to SNAPSHOT_PKGS below.
#
# B2 appended hooks and gitattr — both ship goldens for hook bodies
# and the .gitattributes managed-block respectively. linkcheck DOES
# NOT need a golden (its testdata is constructed inline from
# tempdir fixtures), so it stays off this list.
SNAPSHOT_PKGS := ./internal/cli/... ./internal/hooks/... ./internal/gitattr/... ./internal/timeline/... ./internal/tree/...
.PHONY: snapshot
snapshot:
	$(GO) test $(SNAPSHOT_PKGS) -update

# Placeholder for the byte-identical parity gate vs Python v0.6.14.
# Wired up in a later wave once the Go binary covers enough subcommands
# to make a meaningful diff. Today it prints what it WILL check so the
# Makefile entry point is stable for CI to invoke without breaking.
.PHONY: verify-parity
verify-parity:
	@echo "verify-parity: placeholder — wave B1 only ships --version."
	@echo "Later waves will diff Go binary output vs src/logmind v0.6.14"
	@echo "for each command pair (init, log, show, search, ...)."

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: clean
clean:
	rm -rf $(BIN_DIR)
