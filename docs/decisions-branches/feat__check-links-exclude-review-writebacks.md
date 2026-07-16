<!-- logmind-entry-start: 2026-06-05-check-links-exclude-docs-reviews-pr-md-spec-6-2-review-write -->
- **2026-06-05** — check-links: exclude docs/reviews/PR-*.md (SPEC §6.2 review-writeback path)
<!-- logmind-entry-end -->

## 2026-06-05 14:50 - check-links: exclude docs/reviews/PR-*.md (SPEC §6.2 review-writeback path)

**Reasoning:** clud-bug-app writes docs/reviews/PR-<n>.md per SPEC §6.2 as append-only review telemetry; these files are never cross-linked by design, so logmind check-links flags them as orphan markdown and fails CI on every PR that picks up a review. Ship the fix structurally in logmind (per-repo .logmindignore would tax every consumer; we already encode this path convention via internal/skill/sync.go ParseReview).

**Alternatives considered:** Per-repo .logmindignore allowlist entry: rejected — friction tax replicated across every consumer; logmind has authoritative knowledge of the SPEC §6.2 path, Change clud-bug-app to stop writing the file: rejected — breaks SPEC §6.2 contract, and the file is the load-bearing input for logmind sync (skill PROVENANCE.md updates), Glob-pattern allowlist (PR-*.md): rejected — over-engineers the isAllowedOrphan mechanism (which is exact-match + dir-prefix); the entire docs/reviews/ directory is convention-owned by the App + sync pipeline

**Implications:**
- Every repo on logmind v1.0.x+ gets the fix free on next bump; clud-bug#147 unblocks; future PRs across the org stop failing check-links from review-writebacks
- Mirror change wanted in the Python source (legacy parity) if/when it gets touched — currently the Go binary is the only authoritative implementation post-cutover (commit 5979eef)

---
