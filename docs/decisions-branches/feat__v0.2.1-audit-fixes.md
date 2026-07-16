<!-- logmind-entry-start: 2026-05-18-v0-2-1-audit-driven-fixes-version-pinning-idempotent-init-at -->
- **2026-05-18** — v0.2.1: audit-driven fixes — version pinning, idempotent init, atomic writes, hardened git helpers
<!-- logmind-entry-end -->

## 2026-05-18 13:04 - v0.2.1: audit-driven fixes — version pinning, idempotent init, atomic writes, hardened git helpers

**Reasoning:** External audit surfaced 8 findings; verified 5 real + 3 false-positives against the actual code. Shipping the 4 that matter: P0 (workflow templates now pin logmind version via __LOGMIND_VERSION__ substitution at install time — kills the silent-downstream-breakage class), P1 (logmind init is now idempotent on already-init'd repos: refresh-mode rewrites stale workflows by # logmind-template-version marker, leaves docs/ + agent files alone — eliminates the mv docs /tmp dance), P3a (is_git_repo + current_branch in git_handler.py now safely swallow OSError/PermissionError; the bare except in logger.py is now unreachable dead code and was removed), P3b (new core/atomic_io.py with temp-file + os.replace pattern wired into all 5 state-file write sites — kills the truncate-on-concurrent-log footgun).

**Alternatives considered:** Ship P2 (the 'stale LOGMIND_BOT_PAT at timeline.py:150') — false positive; that line is a docstring example of accurate historical content. Skipped., Ship P4b (boilerplate dupes across per-branch files) — false; grep -l confirmed 0 files have it. Skipped., Ship P4a (--check defaults to docs/timeline.md) + P4c (path-resolution check) — real but trivial UX vs pathological cases. Deferred.

**Implications:**
- Downstream repos installing logmind today get pinned-version CI workflows instead of unpinned ones. Every breaking release no longer silently breaks their CI weeks later.
- logmind init is now a single-command upgrade path: cd into a v0.2.0-installed repo, run logmind init, get refreshed workflows. Was: rm-then-init dance.
- Concurrent logmind log invocations (multi-agent repos) can no longer race-truncate decisions.md / timeline.md / file-structure.md.
- is_git_repo + current_branch behave defensively on file-system permission errors; downstream callers no longer need to guard with bare except.

---
