## 2026-06-02 21:43 - bisect(B7-1): add codesign import step on top of minimal release.yml

**Reasoning:** Test if Apple-Actions/import-codesign-certs@v3 step + its if: condition is the GitHub-rejected element. Minimal version succeeded at run 26858506229; this adds ONLY the codesign step (with full if: expression and ${{ secrets.MACOS_* }} refs) to isolate startup_failure cause.

**Alternatives considered:** Add all suspect steps at once (faster but doesn't isolate), Try hardcoded secret values first then add ${{ secrets }} refs (1 more bisection step but cleaner data)

**Implications:**
- If this succeeds, codesign step is not the cause; move to GoReleaser steps. If startup_failure, this step is the culprit and we'll iterate on it.

---
