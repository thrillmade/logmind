## 2026-05-15 10:28 - v0.1.2: upstream bug fixes from clud-bug PR #21 + skill repo restructure to thrillmot/agent-skills collection

**Reasoning:** Six bugs surfaced when clud-bug installed logmind v0.1.1: (1) Python-only ignore_patterns flooded Node/Next.js trees with 280+ lines of build cache; (2) check-decisions.yml advertised [skip-logmind] PR-title override but if: never checked title; (3) THRESHOLD env var was dead — gate hardcoded 20; (4) logmind-aggregate.yml's git push fails under branch protection; (5) git diff --numstat without --no-renames miscounts src→docs renames; (6) auto_push: true + git_add_all in logger silently published unrelated working-tree changes alongside the decision. Plus skills.sh page was bare because we used the two-level user/skill URL form instead of three-level collection layout.

**Alternatives considered:** Ship A–C only, leave D–F for v0.1.3 — but D/E are the highest-impact fixes per clud-bug review, no reason to split, Default auto_push to false instead of scoping git add — but breaks the existing convenience contract, scope is cleaner, Skip the skill repo restructure, fix logmind bugs only — but the restructure is a one-time URL change that compounds with the install-count bundling we already had in init

**Implications:**
- Templates ship with .next/.vercel/.turbo/out/coverage/*.tsbuildinfo/.DS_Store/*.log excluded by default — Node projects no longer flood file-structure.md
- tree_gen now does path-aware fnmatch instead of basename-only, so a root .gitignore entry like site/.next/ correctly skips the nested cache
- logmind log --stage scoped (new default) stages only the decision file + file-structure + archive_if_rotated. --stage all reinstates v0.1.1 behavior as an opt-in
- AGENTS.md install URL is npx skills add -g https://github.com/thrillmot/agent-skills --skill logmind (old thrillmot/logmind-skill URL auto-redirects via GitHub, so v0.1.1 AGENTS.md files keep working during the transition)
- logmind agents update will refresh existing v0.1.1 AGENTS.md blocks to the v2 marker version (new URL) on next run

---
