# Makefile for the logmind Go binary (v1.0+).
#
# Post-cutover (#132): the Python v0.6.x package has been removed from
# main. PyPI `logmind` stays frozen at v0.6.16 (never republished from
# this repo). Active codebase is Go-only; this Makefile drives the Go
# build + test loop.

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
	@echo "logmind Go targets:"
	@echo ""
	@echo "  make build           Build $(BIN) from $(CMD_PKG)"
	@echo "  make test            Run all Go tests"
	@echo "  make snapshot        Regenerate testdata/*.golden from current Go output"
	@echo "  make tidy            Run go mod tidy"
	@echo "  make clean           Remove $(BIN_DIR)/"

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

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: clean
clean:
	rm -rf $(BIN_DIR)
