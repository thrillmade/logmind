<!-- logmind-entry-start: 2026-06-03-remove-python-source-tree-post-cutover-g3-a-consumers-migrat -->
- **2026-06-03** — Remove Python source tree post-cutover (G3.a consumers migrated)
<!-- logmind-entry-end -->

## 2026-06-03 14:55 - Remove Python source tree post-cutover (G3.a consumers migrated)

**Reasoning:** All 5 G3.a consumer-repo migration PRs are merged (#110, #68, #143, #48, #24); no consumer pulls from src/logmind/ anymore. The Python v0.6.16 wheel stays frozen on PyPI but is never republished from this repo. Keeping the Python tree in main is dead-code that confuses the active codebase (Go) story and bloats every clone — removing it makes "main = Go" obvious, shrinks the working tree by ~26K lines, and removes the maintenance ambiguity created by the cutover #132 leaving both trees in place. Master-plan task #149.

**Alternatives considered:** Leave src/logmind/ frozen at v0.6.16 for archival (rejected: duplicates what PyPI already preserves; archival lives in git history via the v0.6.16 tag — no need to keep it on main); Move src/logmind/ to a python/ subdirectory and stop maintaining (rejected: same dead-code problem, just renamed); Delete src/ but keep tests/ as historical fixtures (rejected: pytest suite is meaningless without the package; the Go side has its own tests under internal/ and cmd/).

**Implications:**
- src/logmind/, tests/, bench/, MANIFEST.in, .python-version, pyproject.toml all removed. ~26K lines of Python source gone from main.
- Top-level CHANGELOG.md preserved as docs/changelog-python.md (historical v0.6.x release notes); Go release notes now live in GitHub Releases.
- .pre-commit-config.yaml surgically edited: dropped ruff/black/mypy Python tool hooks; kept universal hooks (trailing-whitespace, end-of-file-fixer, check-yaml, check-added-large-files, check-merge-conflict) plus the `logmind check-links` local hook.
- Makefile cleaned of stale "Python still lives in src/" docstrings + the `verify-parity` placeholder target (was a wave-B1 stub for byte-identical comparison against Python v0.6.14 that was never wired up).
- CONTRIBUTING.md rewritten for the Go-only dev loop (make build / make test instead of pip install -e / pytest); release process now points at GoReleaser instead of publish.yml + manual pyproject bump.
- .github/workflows/bench.yml removed (Q7-logmind Python bench harness; companion to bench/).
- .github/workflows/regen-timeline.yml + check-doc-links.yml updated to build the Go binary (`make build` + `./bin/logmind`) instead of `pip install -e .`. They were the two PR-gate workflows that would have hard-failed on this PR otherwise. Mirror updates ship into the consumer templates in a follow-up.
- notify-agent-skills.yml left as-is — it triggers only on tag push (v*) so it doesn't block this PR's checks. It WILL fail on the next tagged release because it imports `logmind.core.changelog` and reads top-level CHANGELOG.md (now under docs/). Follow-up PR needs to port the changelog-section extractor to Go (or inline the slicing in the workflow) and re-point the path. Flagged as a known follow-up.
- docs/file-structure.md regenerated against the cleaned tree via the Go binary's `logmind file-structure --write` command.
- Verification: `go build ./... && go test ./... -race -count=1` pass; `make build` produces a working binary; `./bin/logmind --version` reports `logmind 1.0.0-dev (spec 0.1.0)` (the `-dev` suffix is the local-build default; the GoReleaser pipeline injects the clean tag via ldflags).
