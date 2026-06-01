## 2026-06-01 19:15 - feat(v0.6.11): single-quoted pin regex + self-update template uses PAT for workflow refreshes

**Reasoning:** Two follow-on fixes to v0.6.9 propagation issues hit today. (1) inserter.py + doctor.py pin regex didn't match reporulez-style single-quoted pins; widened to all three quote styles AND preserve exact style on rewrite. (2) logmind-self-update.yml.template v5 now uses LOGMIND_AUTO_REGEN_PAT for checkout + push (mirror of regen-timeline.yml v3) so workflow-file refreshes can propagate; falls back to GITHUB_TOKEN with clear error if PAT missing AND workflows changed.

**Alternatives considered:** Wait until needed; current workarounds (manual sed for reporulez + admin merges for workflow-PR failures) work but cost time on every propagation cycle, Drop the GitHub-token-write-to-workflow-file constraint entirely by switching to a fully PAT-based bot identity — bigger architectural change, deferred to D.10 orchestrator App

**Implications:**
- Resolves the 3-of-3 propagation blocker on tokenomics + agent-skills + clud-bug from the v0.6.9 cycle today
- Resolves the silent miss on reporulez where logmind agents update --apply returned nothing despite stale pins
- Next ship is v0.6.12 IF auto-resolve flow ships — or proceed to D.0.h tokenomics outreach with v0.6.10 + v0.6.11 both available

---
