## 2026-06-03 13:19 - Port logmind sync to Go: PROVENANCE.md writeback driven by docs/reviews/PR-*.md

**Reasoning:** B5b loop-closer per SPEC §3.9 + §6. clud-bug review files land in docs/reviews/PR-<n>.md (NORMATIVE template §1.8.1). logmind sync parses those files locally (no GitHub API), aggregates citations per skill, and rewrites PROVENANCE.md per §1.11.1. No existing Python sync.py — implementation is greenfield Go using fixtures-derived spec semantics. Place parsing in internal/skill/sync.go (importable by future tooling), CLI wiring in internal/cli/sync.go (cobra cmd). Idempotency: parse current PROVENANCE.md for last-applied review-sha set; re-running with no new PR-*.md SHAs is a no-op.

**Alternatives considered:** Single file in internal/cli/sync.go — rejected: skill audit/scaffold convention puts logic in internal/skill so it stays importable, GitHub API fetch of reviews — rejected: SPEC §6.5 explicitly requires reading the local files, Append to PROVENANCE.md instead of rewriting the YAML block — rejected: SPEC §1.11.1 fixes the schema and history is append-only inside a single template

**Implications:**
- logmind sync command opens path for B5b (write-drafts flag is deferred — this PR is just the provenance writeback)
- Sync stores applied review-shas in PROVENANCE.md so re-runs are idempotent
- Skill names cited in reviews must match an installed .claude/skills/<name>/ directory; unknown citations are skipped with a warning

---
## 2026-06-03 13:33 - Implement logmind sync (B5b / G4.a): port to Go

**Reasoning:** Parses docs/reviews/PR-*.md per SPEC §1.8.1 NORMATIVE template (review-sha + Skills cited block) and increments cited-by-clud-bug + sets last-refined on each cited skill's PROVENANCE.md. Idempotency via applied-review-shas list stored inside the existing YAML block. Implementation split: internal/skill/sync.go holds the parser + writer (importable by future tooling); internal/cli/sync.go is the cobra wiring. No Python sync.py existed — Python v0.6.x closed the loop via .clud-bug.json usage counters; the Go port routes the same signal through the durable in-repo PR review files (survives JSON schema migrations, works for GitHub-App variant, stays grep-friendly).

**Alternatives considered:** Walk repository via GitHub API — rejected: SPEC §6.5 explicitly requires local file read, Track applied SHAs in a sibling .logmind/sync-state.json — rejected: harder to grep, second source of truth competing with PROVENANCE.md, Strict YAML parsing of PROVENANCE.md — rejected: the file is hybrid markdown+YAML and prose below the block is user-edited; full YAML parse would force a lossy round-trip

**Implications:**
- Counter increments only when a new review-sha appears (idempotent re-runs are no-ops). Unknown-skill citations route to stderr warn and the run continues. Re-running after a clean PR (zero citations) is also a no-op (no SHA added to applied set).
- Future SPEC §3.9 surfaces --since and --write-drafts; this PR ships --dry-run only and leaves the others for a follow-on commit so this PR stays focused on the loop-closer write path.
- Tests cover empty repo / single citation / multi-skill / multi-PR / idempotent / dry-run / malformed-file-skipped / unknown-skill-warned / SHA case-insensitivity / non-PR markdown ignored — 14 unit tests in internal/skill, 7 in internal/cli.

---
## 2026-06-03 13:34 - Restore file-structure.md after logmind log regen mis-rooted from worktree

**Reasoning:** Previous logmind log commit ran from /tmp/sync-port-wt and regen'd docs/file-structure.md using that path as root — so the rendered tree said 'sync-port-wt' and omitted untracked-but-existing files (.benchmarks, site/.next, qa screenshots) that are visible from the main worktree. This commit restores the canonical view.

**Alternatives considered:** Skip the fix and let CI's check-derived-docs flag it — rejected: same author should fix what they just broke, Amend HEAD — rejected: AGENTS.md prefers new commits for traceability

**Implications:**
- Underlying issue: logmind file-structure walks the current working tree from CWD; running it from a worktree path with a different basename produces a diff. Worth tracking as a follow-up bug (perhaps anchor on git rev-parse --show-toplevel).

---
## 2026-06-03 14:03 - fix(sync): only count SHA on successful write; mark dry-run on ok line (review #135)

**Reasoning:** Two clud-bug-review threads on PR #135 flagged invariant breaches. Bug 1: appliedReviewSet was mutated before atomicWriteFile, so a write failure inflated summary.ReviewsApplied beyond the count of skills that actually landed on disk. Moving the mutation to after the successful write block fixes the invariant 'ReviewsApplied == count of SHAs whose contributions actually persisted'. Bug 2: the machine-parseable 'ok sync:' line lacked the '(dry-run)' marker while FormatSummary above it did include the marker, so a downstream consumer grepping for 'ok sync: \\d+' would treat a dry-run preview as a real run. Prefixed the line with '(dry-run) ' so the two surfaces agree.

**Alternatives considered:** Bug 1 alt: track a separate 'written-this-run' set and reconcile at the end — rejected as over-engineering for a single contiguous loop body, Bug 1 alt: keep the set mutation early and decrement on failure — rejected because the additive-only set model is simpler and the iteration boundary is the natural rollback point, Bug 2 alt: drop the redundant 'ok sync:' line and rely solely on FormatSummary's prefix — rejected because the line is a documented machine surface (post-merge hooks consume it); we extend the contract rather than break it

**Implications:**
- internal/skill/sync.go gains a package-level 'var atomicWriteFile' test seam so tests can simulate write failures deterministically
- internal/cli/sync.go's 'ok sync:' grammar gains a '(dry-run) ' optional prefix — downstream parsers should accept it or filter on it
- New test TestSync_WriteFailure_DoesNotCountSHA pins the Bug 1 invariant; TestRunSync_DryRun's positive assertion is updated and a negative assertion is added so the bug can't silently regress

---
