<!-- logmind-entry-start: 2026-06-02-chore-bump-version-0-6-14-0-6-16-on-v1-go-rewrite -->
- **2026-06-02** — chore: bump __version__ 0.6.14 → 0.6.16 on v1-go-rewrite
<!-- logmind-entry-end -->

## 2026-06-02 17:15 - chore: bump __version__ 0.6.14 → 0.6.16 on v1-go-rewrite

**Reasoning:** Python tests on v1-go-rewrite fail because workflow-template tests substitute __version__ into expected pip-install pin (test_v0_2_1_audit_fixes.py:41 + :365 + :414). main is at 0.6.16; branching back is unblocks pytest for B5 PR #124 + B6 PR #125 + every subsequent Go PR until v1.0 cutover. test_v0_2_1_audit_fixes.py: 20/20 pass locally with the bump

**Alternatives considered:** skip the tests on v1-go-rewrite via pytest skipif marker — rejected: invasive across multiple tests, more code change than the bump, remove pytest from required checks on v1-go-rewrite branch protection — rejected: loses safety net for the Python source that B5/B6 still reuse for parity tests

**Implications:**
- every Go PR opened against v1-go-rewrite now gets clean pytest CI
- v0.6.16 features (commit-msg hook, multi-branch tests, PATH probe, AGENTS.md v6/v8) still NOT on v1-go-rewrite — those land via the v0.6.16 carry-forward follow-up PR

---
