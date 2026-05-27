## 2026-05-27 11:42 - fix(vercel.json): guard ignoreCommand against bad VERCEL_GIT_PREVIOUS_SHA

**Reasoning:** Fix: gate git diff with 'git cat-file -e $SHA' — if the object exists, do the diff (skip when site/ unchanged); if not, fall through to 'else false' (treat as 'do build'). Same defensive behavior we had for empty SHA, now extended to missing-object

**Implications:**
- Same change shipped to clud-bug/vercel.json as a separate PR — both had the same bug

---
