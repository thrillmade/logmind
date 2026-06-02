## 2026-06-02 17:51 - B2 carry-forward: Go post-merge hook body absorbs v0.6.16 HEAD-vs-origin skip

**Reasoning:** Python v0.6.16 replaced v0.6.15's blanket default-branch skip with a HEAD-vs-origin check (skip only on fast-forward pull-up). Local merges that introduce new commits MUST regen for the multi-branch self-heal case. Go body kept byte-identical via the existing TestPostMergeBody_ByteIdenticalToPython parity contract — drop in the same nine-line shell block verbatim. Updated testdata/post-merge.golden via go test -update.

**Alternatives considered:** Extract the HEAD-vs-origin block into a Go constant — rejected because the Python helper uses one inline concatenation, the existing Go body matches that shape line-for-line, and extracting would diverge the two implementations cosmetically without behavior change, Wait until v0.7.x to land — rejected because B6 (PR #125) already carries-forward the v0.6.16 commit-msg hook and PATH probe; deferring B2/B3/B4 would leave a half-shipped v0.6.16 in Go

**Implications:**
- Existing v0.6.15-style installed hooks (blanket default-branch skip) will be overwritten on the next logmind log because the body bytes differ. That's the install-time refresh contract — no extra work.
- Also overwrote src/logmind/core/gitattributes.py with the v0.6.16 Python source on this branch so the parity test can compare against the same shape. Without this the byte-identical-vs-Python test fails — the parity helper shells to in-tree src/logmind.

---
