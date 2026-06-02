## 2026-06-01 21:07 - feat(v0.6.12): doctor surfaces LOGMIND_AUTO_REGEN_PAT status proactively

**Reasoning:** Today's v0.6.11 propagation cycle exposed a second-order PAT failure mode: the secret existed at the org level but had insufficient scopes (missing Workflows: write after fine-grained-token migration). Manifested as cryptic 'refusing to allow workflow' 403s at push step. Doctor now proactively detects PAT-dependent workflow bodies and queries gh secret list when possible — surfaces 'missing' / 'present' / 'cannot-verify' status with the required scopes documented inline.

**Alternatives considered:** Wait for D.10 GitHub App which removes PAT requirement entirely — but multi-week build, Add a runtime self-test step in the workflows that ACTIVELY exercises the PAT scope — but adds complexity + scope-checking calls

**Implications:**
- Externally-hosted consumers get a proactive heads-up before their auto-propagation breaks (the v0.6.11 cycle showed this is the dominant failure mode)
- Plan D.0.j captures the three-lever path: v0.6.12 doctor (this) → v0.6.13 smart workflow-skip → D.10 App (removes problem entirely)

---
