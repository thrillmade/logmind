## 2026-05-27 00:43 - v0.3.2: homebrew-bump auto-merge + nothing-to-commit guard + site/app/page.tsx URL fix

**Reasoning:** (3) Nothing-to-commit guard added to handle workflow re-runs idempotently — exits 0 when formula already at target version instead of erroring on empty commit

**Implications:**
- Next tag push (v0.3.2 itself) dogfoods the auto-merge — tag → PyPI publish → tap PR opened → tap PR auto-merged
- Site change cosmetic for logmind.dev visitors

---
