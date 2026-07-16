<!-- logmind-entry-start: 2026-06-01-fix-readme-use-full-skill-path-in-skills-sh-badge-url -->
- **2026-06-01** — fix(README): use full skill path in skills.sh badge URL
<!-- logmind-entry-end -->

## 2026-06-01 16:30 - fix(README): use full skill path in skills.sh badge URL

**Reasoning:** README badge rendered as 'resource not found' (red) because the badge image URL was https://skills.sh/b/thrillmade/agent-skills (missing the skill name). skills.sh badge endpoint requires the full /b/<owner>/<repo>/<skill-name> shape — verified that https://www.skills.sh/b/thrillmade/agent-skills/logmind returns 200 SVG. The click-through URL was already correct; only the image URL needed the trailing /logmind segment.

**Alternatives considered:** Remove the skills.sh badge entirely (loses the install-count signal), Use shields.io custom badge wrapper instead (adds redirect latency, more brittle)

**Implications:**
- Consumer-facing first-impression bug — first thing any visitor to the logmind README sees. Closes a low-cost trust gap before Phase D scope.
- Verified the same pattern is NOT present on sister READMEs (clud-bug / agent-skills / reporulez / tokenomics / rezgen) — fix is localized to logmind.

---
