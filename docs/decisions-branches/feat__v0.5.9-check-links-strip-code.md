<!-- logmind-entry-start: 2026-05-29-feat-v0-5-9-fix-60-strip-code-regions-before-scanning-for-li -->
- **2026-05-29** — feat(v0.5.9): fix #60 — strip code regions before scanning for links
<!-- logmind-entry-end -->

## 2026-05-29 23:53 - feat(v0.5.9): fix #60 — strip code regions before scanning for links

**Reasoning:** Mentioning markdown link syntax inside backticks (e.g. discussing the `[text](path)` pattern in prose) used to trip check-links because the link-extraction regex didn't strip code regions first. Recursion: writing a decision-log entry to fix this trips on the same fix-attempting example. Other markdown linters (markdown-link-check, lychee) skip code regions; we now match. `_strip_code_regions()` replaces fenced blocks and inline-code spans with whitespace of equivalent length so line numbers + byte offsets stay correct for any broken-link error messages.

**Implications:**
- Bumps to v0.5.9. 4 new tests covering inline-code + fenced-block + regression-guard (real broken link in prose alongside code example STILL gets detected) + line-number preservation. 624/624 tests pass. Independent of v0.5.8 which is in parallel review.

---
