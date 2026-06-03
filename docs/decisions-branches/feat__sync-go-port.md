## 2026-06-03 13:19 - Port logmind sync to Go: PROVENANCE.md writeback driven by docs/reviews/PR-*.md

**Reasoning:** B5b loop-closer per SPEC §3.9 + §6. clud-bug review files land in docs/reviews/PR-<n>.md (NORMATIVE template §1.8.1). logmind sync parses those files locally (no GitHub API), aggregates citations per skill, and rewrites PROVENANCE.md per §1.11.1. No existing Python sync.py — implementation is greenfield Go using fixtures-derived spec semantics. Place parsing in internal/skill/sync.go (importable by future tooling), CLI wiring in internal/cli/sync.go (cobra cmd). Idempotency: parse current PROVENANCE.md for last-applied review-sha set; re-running with no new PR-*.md SHAs is a no-op.

**Alternatives considered:** Single file in internal/cli/sync.go — rejected: skill audit/scaffold convention puts logic in internal/skill so it stays importable, GitHub API fetch of reviews — rejected: SPEC §6.5 explicitly requires reading the local files, Append to PROVENANCE.md instead of rewriting the YAML block — rejected: SPEC §1.11.1 fixes the schema and history is append-only inside a single template

**Implications:**
- logmind sync command opens path for B5b (write-drafts flag is deferred — this PR is just the provenance writeback)
- Sync stores applied review-shas in PROVENANCE.md so re-runs are idempotent
- Skill names cited in reviews must match an installed .claude/skills/<name>/ directory; unknown citations are skipped with a warning

---
