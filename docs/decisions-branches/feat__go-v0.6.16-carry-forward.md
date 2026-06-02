## 2026-06-02 17:51 - B2 carry-forward: Go post-merge hook body absorbs v0.6.16 HEAD-vs-origin skip

**Reasoning:** Python v0.6.16 replaced v0.6.15's blanket default-branch skip with a HEAD-vs-origin check (skip only on fast-forward pull-up). Local merges that introduce new commits MUST regen for the multi-branch self-heal case. Go body kept byte-identical via the existing TestPostMergeBody_ByteIdenticalToPython parity contract — drop in the same nine-line shell block verbatim. Updated testdata/post-merge.golden via go test -update.

**Alternatives considered:** Extract the HEAD-vs-origin block into a Go constant — rejected because the Python helper uses one inline concatenation, the existing Go body matches that shape line-for-line, and extracting would diverge the two implementations cosmetically without behavior change, Wait until v0.7.x to land — rejected because B6 (PR #125) already carries-forward the v0.6.16 commit-msg hook and PATH probe; deferring B2/B3/B4 would leave a half-shipped v0.6.16 in Go

**Implications:**
- Existing v0.6.15-style installed hooks (blanket default-branch skip) will be overwritten on the next logmind log because the body bytes differ. That's the install-time refresh contract — no extra work.
- Also overwrote src/logmind/core/gitattributes.py with the v0.6.16 Python source on this branch so the parity test can compare against the same shape. Without this the byte-identical-vs-Python test fails — the parity helper shells to in-tree src/logmind.

---
## 2026-06-02 17:52 - B3 carry-forward: multi-branch self-heal regression tests at internal/timeline/merge_driver_test.go

**Reasoning:** v0.6.16 added three regression tests in tests/test_merge_driver.py that exercise the whole dogfood loop end-to-end — two/three/squash concurrent-branch merges with real subprocess git + logmind. Without Go-side equivalents the cross-binary parity gate would silently regress on every Go-only change. The tests use exec.Command (no mocks) because the merge driver itself shells out to logmind.

**Alternatives considered:** Build tag e2e instead of integration — rejected; existing parity workflows use 'integration' naming convention, e2e would imply browser/UI which we don't have, No build tag (always run) — rejected; the tests need logmind on PATH; running them in unit-only contexts (go test ./...) would fail or skip noisily. The tag keeps unit-test runs clean and the integration runs explicit., Mock the merge driver shell-out — rejected; the whole point of these tests is to catch driver-config drift and binary-availability gaps. A mock would defeat the regression signal.

**Implications:**
- All three subprocess invocations (git/logmind init/logmind log/git merge) MUST pass through testEnv() which inherits os.Environ. Without PYENV_VERSION propagating into git's hook env, the pyenv shim resolves logmind to 'system' Python which has an older binary locally — observed 0.3.4 — producing different brief-mode output. Documented in the testEnv() helper's doc comment.
- CI workflow needs 'go install ./cmd/logmind' before running 'go test -tags=integration ./internal/timeline/...'. Documented in the package-level doc comment of merge_driver_test.go.

---
## 2026-06-02 17:53 - B4 carry-forward: AGENTS.md templates bumped to v6 full and v8-pointer slim

**Reasoning:** Python v0.6.16 reframed both AGENTS.md templates with 'REQUIRED for substantive commits' heading and a DO-NOT-git-commit blockquote that pairs with the v0.6.16 commit-msg hook installed by logmind init. The Go embed.FS templates are loaded by inserter at runtime — replacing the file bytes was enough to update the embedded copy. Bumped the version markers in templates.go + templates_test.go assertions + inserter.matchingTemplate to accept both old (v5 / v7-pointer) and new (v6 / v8-pointer) markers so existing repos refresh into the new body.

**Alternatives considered:** Auto-migrate v5 to v8-pointer (slim) — rejected; the version guard against silent full-vs-slim flips is the whole point of matchingTemplate. v5 must map to full-template flavor; v7-pointer must map to slim. The bump is per-flavor only., Drop the v5 / v7-pointer matchers entirely — rejected; downstream repos installed during v0.6.0-v0.6.15 have those markers and the agents update --apply path needs them to map correctly to flavor before drift detection runs.

**Implications:**
- Added two new tests in inserter_test.go that pin the v5→v6 + v7-pointer→v8-pointer refresh paths. Existing tests bumped to v6 / v8-pointer markers since templates.AgentsTemplate() / AgentsSlimTemplate() now embed the new versions.
- Python src/logmind/templates/AGENTS.md.template + AGENTS.md.slim.template on this branch also overwritten with v0.6.16 source so the templates byte-identical-to-Python parity test passes (mirrors what was done for gitattributes.py in the B2 carry-forward).

---
