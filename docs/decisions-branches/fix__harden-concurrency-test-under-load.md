← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-17-test-log-harden-concurrency-tests-against-acquire-timeout-fl -->
- **2026-07-17** — test(log): harden concurrency tests against acquire-timeout flakes under load
<!-- logmind-entry-end -->

## 2026-07-17 10:18 - test(log): harden concurrency tests against acquire-timeout flakes under load

**Reasoning:** under full-suite saturation a concurrent logmind log can legitimately exceed the 15s repo-lock acquire timeout and fail loud (refusing to write, never losing data); the rigid all-N-succeed assertion then reported a false lost-decisions failure — a CI-reliability landmine that looks like data loss but is not

**Alternatives considered:** raise the product acquire timeout (rejected: a test artifact must not change real-user behavior); a tolerate-fail-loud invariant with no retry (rejected: too lenient, could mask a perf regression where nearly everything times out); retry the timed-out invocation SEQUENTIALLY keeps the strict all-N-land plus N-commits assertion intact

**Implications:**
- a NON-timeout failure (a crash) is never retried, so the suite still fails on the pre-fix rename crash and silent loss — verified by dropping the hardened test onto 041a75a

---

