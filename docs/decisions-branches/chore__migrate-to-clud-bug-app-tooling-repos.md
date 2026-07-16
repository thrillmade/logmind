<!-- logmind-entry-start: 2026-06-05-remove-clud-bug-review-yml-workflow-g3-b-extension-app-cover -->
- **2026-06-05** — Remove clud-bug-review.yml workflow (G3.b extension — App covers logmind now)
<!-- logmind-entry-end -->

## 2026-06-05 12:50 - Remove clud-bug-review.yml workflow (G3.b extension — App covers logmind now)

**Reasoning:** clud-bug[bot] GitHub App is installed at thrillmade org scope=all, which means it fires on every PR in this repo IN ADDITION to the per-repo workflow — creating duplicate reviews. G3.b's original 5-consumer sweep (tokenomics #70, agent-skills #117, clud-bug #147, reporulez #50, rezgen #26) excluded logmind + clud-bug-app as 'tooling repos', but App scope=all means tooling repos are not actually excluded from review. Removing the workflow eliminates the duplicate.

**Alternatives considered:** Leave workflow in place — rejected, would keep producing duplicate reviews indefinitely. Restrict App installation to non-tooling repos — rejected, fights scope=all and adds installation drift; App is the strategic primary surface per master plan §End State #7.

**Implications:**
- Future PRs reviewed only by clud-bug[bot] App; no per-repo Anthropic key needed; no consumer-paid CI compute for review.
- AGENTS.md/.cursorrules pointer updated to App source (v2 -> v3-app block).
- Skill manifest .claude/skills/.clud-bug.json preserved so App can still load per-repo skill selection. clud-bug-collaboration skill subdir also preserved.

---
