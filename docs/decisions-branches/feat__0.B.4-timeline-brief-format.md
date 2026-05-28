## 2026-05-28 08:46 - v0.5.4: docs/timeline.md ships brief format on disk (Phase 0.B.4)

**Reasoning:** Default render_markdown to brief=True so every consuming repo's docs/timeline.md ships compact on next regen. Brief format: per month, header carries (N decisions) count; ≥3-entry months render newest + '... N-2 more decisions ...' + oldest; ≤2-entry months show all entries (nothing to compress). Threaded brief through generate_timeline + write_timeline. CLI gains --full flag for legacy format. Smoke-tested on logmind's own docs/timeline.md: 71 lines → 20 lines (~72% reduction). +10 tests cover brief-is-default, brief shorter than full, month-count header, first+last+elision for ≥3 entries, all-entries for ≤2 entries, --full opt-in, CLI surface, deterministic render preservation.

**Implications:**
- Future timeline-readers (clud-bug review prompt, agent boots) ingest ~70% fewer bytes from this file in every consuming repo. Legacy full format remains opt-in via logmind timeline --full.

---
## 2026-05-28 09:01 - PR #72 fix: tighten brief-1-entry assertion + add singular-form test for n=3 elision

**Reasoning:** clud-bug review caught: the 'or' clause on the 1-entry assertion made the test permissive enough to pass the very regression it guarded against (substring shadowing via .replace). Fix: drop the 'or' clause, add negative lockdown assert that '## 2025-01 (' never appears. Separately: the n=3 case (elision = '1 more decision' singular) was uncovered — added test_render_markdown_brief_n3_uses_singular_decision to lock down that branch. Pre-existing pluralization fix in timeline.py was correct; just needed test coverage.

---
