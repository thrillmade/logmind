## 2026-05-27 07:52 - feat(v0.3.4): check-derived-docs auto-fixes when LOGMIND_AUTO_REGEN_PAT configured

**Reasoning:** Forked PRs ALWAYS run in fail-fast mode — can't push to a fork's head ref. Three-way branching: internal+PAT → auto-fix; internal+no-PAT → fail-fast with opt-in hint; fork → fail-fast

**Alternatives considered:** Auto-commit via GITHUB_TOKEN — rejected because it doesn't re-trigger downstream checks; merge gate stays stuck on 'Expected', Bot-PR-opener instead of push-back — more ceremony, same UX cost as today's manual path. The push-back is the happy path

**Implications:**
- Workflow template-version bumped v2 → v3. logmind init refresh-mode rewrites existing installs on next invocation. Backwards-compatible without setup: identical to v0.3.3's behavior until LOGMIND_AUTO_REGEN_PAT is added
- Required permission bumped from contents:read to contents:write (needed for the auto-push path)

---
