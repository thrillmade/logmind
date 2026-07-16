<!-- logmind-entry-start: 2026-05-29-0-0-t-logmind-side-rtk-inspired-fail-safe-warn-not-silent-on -->
- **2026-05-29** — 0.0.T (logmind side): RTK-inspired fail-safe → warn-not-silent on parser; orphan-cleanup on atomic_write
<!-- logmind-entry-end -->

## 2026-05-29 09:18 - 0.0.T (logmind side): RTK-inspired fail-safe → warn-not-silent on parser; orphan-cleanup on atomic_write

**Reasoning:** Two RTK-inspired patterns applied to logmind. (1) src/logmind/core/parser.py iter_decisions: previously caught ValueError on malformed date/time in headers (regex matched but date impossible like '2026-13-45 25:99 - title') and silent-passed. Now emits stderr warning naming file + lineno + the parse error before skipping the entry. The Phase 0 hindsight bug — Phase B's brief-elision test on PR #72 wouldn't have caught this if the input were corrupt. (2) src/logmind/core/atomic_io.py atomic_write_text: previously left an orphaned .tmp sibling behind if Path.write_text raised mid-write. Now catches BaseException (covers KeyboardInterrupt), unlinks the orphan with missing_ok=True, then re-raises. Cleanup is best-effort — if unlink ALSO fails, the original exception still propagates (cleanup never masks the load-bearing error). +6 tests in tests/test_fail_safe_0_0_T.py: warns on malformed date, no warning on clean file, no warning on missing file; cleans up tmp on write failure, never masks original exception, happy path unchanged. 608 pass (full suite).

**Implications:**
- Pattern shape ports cleanly to clud-bug — same try/except/best-effort-cleanup applies to lib/skills.js GraphQL parse fallbacks. Will ship clud-bug side as a separate PR (different repo) after PR #109 (golden gate) lands so the prompt-changes are gated.

---
