## 2026-05-26 16:06 - feat: v0.2.7 — --stage all becomes the default; logmind log is the commit primitive

**Reasoning:** logmind exists to be ONE command that handles add+commit+push for automated agents. The previous default (--stage scoped) forced agents into a manual two-step git workflow that defeated the design intent — users (correctly) kept asking 'why isn't logmind log doing this already?'. Default flip closes that gap; explicit --stage scoped remains as escape hatch for users with unrelated WIP.

**Alternatives considered:** Drop --stage flag entirely — too restrictive; some users genuinely want decision-only commits when staging a long-running refactor separately, Major version bump to v0.3.0 — not strictly breaking; --stage scoped opt-in preserves prior behavior, and the new default is more aligned with the README/skill intent than the prior 'feature'

**Implications:**
- Same logmind log command now produces a single commit with decision + code + any working-tree changes; downstream agent workflows simplify from add+commit→log to just log
- AGENTS.md.slim.template (v3-slim → v4-slim) and AGENTS.md.template (v3 → v4) rewritten to lead with 'logmind log is the commit primitive that replaces git add + git commit + git push'. Refresh fires automatically on next logmind init in installed repos
- Two regression tests pin the new default behavior; explicit --stage scoped test pins backwards-compatibility
- Companion update to canonical skill on agent-skills follows in a separate PR (notify-agent-skills.yml shipped in v0.2.6 will fire on this tag and open the prompt automatically)

---
