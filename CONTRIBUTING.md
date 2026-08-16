# Contributing to logmind

Thanks for considering a contribution! logmind is small and the bar to land
useful improvements is low. This file covers the dev loop, the conventions
we keep, and how to ship a release.

logmind is a Go binary. The Python v0.6.x source tree was removed
post-cutover ([#132](https://github.com/thrillmade/logmind/pull/132)); see
[`docs/changelog-python.md`](docs/changelog-python.md) for the v0.6.x
history.

## Quick start

```bash
git clone https://github.com/thrillmade/logmind
cd logmind
make build                 # → ./bin/logmind
make test                  # full Go test suite
./bin/logmind check-links  # docs link integrity (also runs in CI)
```

Requires Go 1.22+ (matches `go.mod`'s floor).

## Repo layout

- `cmd/logmind/` — binary entry point (thin shim into `internal/cli`)
- `internal/cli/` — cobra command wiring; one file per top-level command
- `internal/` — core modules (`hooks/`, `gitattr/`, `timeline/`, `tree/`,
  `linkcheck/`, `skill/`, `config/`, `version/`, …)
- `internal/*/testdata/*.golden` — snapshot fixtures; regenerate via
  `make snapshot` after intentional output changes
- `installer/` — `install.sh` curl|sh installer + Homebrew cask template
- `scripts/sign-macos.sh` — release-time codesign + notarize helper
- `.goreleaser.yaml` — release pipeline (cross-compile, sign, brew bump)
- `docs/` — project documentation (regenerated artifacts under
  `docs/timeline.md`, `docs/file-structure.md`, …)
- `skill/` — source for the `logmind` skill published in the
  thrillmade/agent-skills catalog (skills.sh); not shipped in the binary
- `site/` — marketing site (Next.js, deploys via Vercel)

## Conventions

### Decision logging

We dogfood logmind. Open a feature branch, then for any non-trivial
choice run:

```bash
logmind log "Short summary" -r "why" -a "alt 1" -i "implication"
```

Branch-aware, with one path rule and no exception to it: every entry writes
to `docs/decisions-branches/<branch>.md` for the branch it was made on, the
default branch included (`main.md`). `docs/decisions.md` is a legacy source —
read where it exists, and never written **on the branch-aware path**.
`resolveDecisionsPath` still falls back to it in exactly three cases, all of
them "there is no branch to route by": `decisions.branch_aware: false`, a
non-git directory, and a detached or unborn HEAD. In those three it is
created and appended to.

### Tests

- Prefer table-driven tests over many one-shot functions.
- For anything that touches branches, init with `git init -b main` so
  the test is independent of `init.defaultBranch` config.
- Snapshot tests live alongside the package; goldens under
  `<pkg>/testdata/*.golden`. Regenerate with `make snapshot` after a
  deliberate output change, and commit the new goldens.

### Style

`gofmt`/`goimports` are canonical. `go vet` and `go test ./...` are the
floor; CI runs both via `make test`.

The `.pre-commit-config.yaml` wires up universal hooks (trailing
whitespace, YAML lint, merge-conflict markers) plus the local
`logmind check-links` hook. Install with:

```bash
pip install pre-commit && pre-commit install
```

(pre-commit itself is a Python tool; that's the only Python dep left in
the dev loop.)

### Markdown links

`logmind check-links` runs in CI on every PR touching `*.md` files.
Broken links and orphaned `docs/*.md` files fail the build. Keep
relative links relative; allowlist intentional orphans via
`linkcheck.allow_orphans` in `.logmind/config.yml`.

## Pull request checklist

- [ ] Tests added or updated (`make test` is green locally)
- [ ] `./bin/logmind check-links` is green
- [ ] A decision logged (`logmind log "..."`) for any architectural change
- [ ] No committed `bin/`, `*.test`, `*.out`, or other generated artefacts

## Release process

logmind releases are tag-driven. To cut `vX.Y.Z`:

1. Bump `internal/version/version.go` `Version` constant (or rely on the
   ldflags injection from `.goreleaser.yaml`).
2. Commit and merge to `main`.
3. Tag and push:
   ```bash
   git tag -a vX.Y.Z -m "vX.Y.Z"
   git push origin vX.Y.Z
   ```
4. The `release.yml` workflow runs GoReleaser to cross-compile binaries,
   sign + notarize the macOS build, create the GitHub Release with the
   artefacts, and bump the Homebrew tap (`thrillmade/homebrew-tap`).

## Reporting issues / proposing features

Use the issue templates in `.github/ISSUE_TEMPLATE/`. For security
vulnerabilities please follow `SECURITY.md` (private disclosure first).

## Branch protection

`main` requires:
- The `test` workflow to pass (Go matrix aggregator — see
  `.github/workflows/test.yml`)
- The `logmind / check-links` workflow to pass
- At least one approving review
