<!-- logmind-entry-start: 2026-05-15-v0-2-0-derived-file-architecture-replaces-per-merge-aggregat -->
- **2026-05-15** — v0.2.0: derived-file architecture replaces per-merge aggregator
<!-- logmind-entry-end -->

## 2026-05-15 15:08 - v0.2.0: derived-file architecture replaces per-merge aggregator

**Reasoning:** User flagged the per-merge aggregator (one bookkeeping PR per feature merge) as unsustainable. The whole class of v0.1.4 LOGMIND_BOT_PAT/GH_TOKEN-trigger-downstream/protected-branch friction stems from that design. Replaced with derived-file architecture: docs/timeline.md (and docs/file-structure.md) are now auto-regenerated from sources (per-branch logs + decisions.md + git log) on every PR commit by the new regen-timeline.yml workflow. Two PRs in flight regenerate to byte-identical output (deterministic, same inputs → same output), so the derived files cannot merge-conflict. Pushes go to the PR's own feature branch (unprotected) via GITHUB_TOKEN — no PAT needed, no branch-protection bypass.

**Alternatives considered:** Keep the aggregator but auto-merge its PR — still needs LOGMIND_BOT_PAT, still one bookkeeping PR per merge, Squash-time hook on merge queue — needs merge queue enabled, more setup; deferred, Push to a long-running orphan branch instead of main — deviates from 'main is the truth' convention

**Implications:**
- New CLI: logmind timeline [--write PATH] [--check] computes the chronological view; regen-timeline.yml runs it on every PR push
- Deleted: logmind-aggregate.yml.template, actions/aggregate.py, the dogfood workflow, test_action_aggregate.py
- Migration: existing v0.1.x installs delete their .github/workflows/logmind-aggregate.yml, re-run logmind init to get regen-timeline.yml, optionally drop LOGMIND_BOT_PAT secret
- Required settings documented in README + CHANGELOG: workflow read+write permissions, strict-required-status-checks on main
- AGENTS.md bumped to v3 / v3-slim; agents update will refresh existing installs
- All 482 pytest pass; full test suite green

---
