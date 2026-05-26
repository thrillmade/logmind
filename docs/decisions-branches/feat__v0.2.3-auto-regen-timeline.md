## 2026-05-26 11:21 - feat: v0.2.3 — logmind log auto-regenerates docs/timeline.md

**Reasoning:** PR #42 (v0.2.2 paths-filter fix) stalled because docs/timeline.md was stale. logmind log writes a new decision file but didn't regen the derived timeline.md index, so every decision PR needed an extra 'logmind timeline --write' + push before check-derived-docs would pass. The fix calls write_timeline() in logger.py's log() function after archival and adds timeline.md to the scoped-staging list — mirroring the existing companion-file pattern for file-structure.md and decisions-archive.md. Regen runs on every branch (not just default) because the CI gate runs on PR branches and timeline merges three-way trivially.

**Alternatives considered:** Pre-commit hook, CI auto-commits via PAT

**Implications:**
- logmind log now produces a self-consistent commit on every branch — no manual timeline regen step needed
- Behavior-only change; no template change; no logmind init needed in installed repos
- This commit itself proves the fix: docs/timeline.md should be auto-staged alongside the decision file

---
