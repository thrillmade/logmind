## 2026-06-02 21:43 - bisect(B7-1): add codesign import step on top of minimal release.yml

**Reasoning:** Test if Apple-Actions/import-codesign-certs@v3 step + its if: condition is the GitHub-rejected element. Minimal version succeeded at run 26858506229; this adds ONLY the codesign step (with full if: expression and ${{ secrets.MACOS_* }} refs) to isolate startup_failure cause.

**Alternatives considered:** Add all suspect steps at once (faster but doesn't isolate), Try hardcoded secret values first then add ${{ secrets }} refs (1 more bisection step but cleaner data)

**Implications:**
- If this succeeds, codesign step is not the cause; move to GoReleaser steps. If startup_failure, this step is the culprit and we'll iterate on it.

---
## 2026-06-02 21:45 - bisect(B7-2): codesign step minimal — no if:, no secret refs, placeholder values

**Reasoning:** bisect-1 (codesign step with full if: + secret refs) → startup_failure. Narrow: is it the action ref itself, the if: expression referencing github.event.inputs.dry_run, or the secret refs? bisect-2 isolates the action ref by replacing if:+secrets with hardcoded placeholders.

**Alternatives considered:** Skip and add GoReleaser snapshot step instead, Test only the if: expression with the original echo step

**Implications:**
- If startup_failure persists, the bug is in the action ref or its with: schema. If success, the cause is the if: expression OR secret refs — test those next.

---
## 2026-06-02 21:47 - bisect(B7-3): codesign step with if: false (forced skip)

**Reasoning:** bisect-2 (codesign with NO if: + NO secrets) → startup_failure. Adding if: false to verify parser fails on STEP DEFINITION (uses:/with: schema), not on runtime resolution.

**Alternatives considered:** Try different action ref tag (e.g. v3.0.0 vs v3)

**Implications:**
- If startup_failure persists, GitHub rejects the step definition outright. If success, step DEFINITION is fine but runtime resolution fails — narrow to specific input.

---
