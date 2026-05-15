## 2026-05-15 02:39 - Release v0.1.1 (Phase 13 polish)

**Reasoning:** Bundles agents update CLI, prominent init prompt, check-decisions workflow + branch-aware bug fix, skills.sh badge, clud-bug swap, reporulez ruleset, adaptive AGENTS.md slim/full template, and check-doc-links always-run fix. Version bump 0.1.0 → 0.1.1 in pyproject.toml; CHANGELOG section rotated.

**Implications:**
- publish.yml will ship to real PyPI on tag push
- Homebrew formula needs separate update with new url + sha256

---
## 2026-05-15 02:43 - Fix two missed 0.1.0 version strings + always-run clud-bug

**Reasoning:** clud-bug review on PR #21 caught a release blocker: src/logmind/__init__.py and src/logmind/cli.py had hardcoded '0.1.0' strings the bump missed. Users who pip install --upgrade and check logmind.__version__ or 'logmind --version' would see 0.1.0 even though importlib.metadata reports 0.1.1. Fixed both. Separately: user asked clud-bug should never be skipped — reverted the if: github.actor != 'dependabot[bot]' filter so clud-bug runs on every PR regardless of author.

**Implications:**
- Single source of truth for the version stays in pyproject.toml; the two hardcoded copies are unfortunate but live across the duration of the release cycle until I migrate cli.py to importlib.metadata
- clud-bug now runs on Dependabot/Renovate PRs too; expect a no-comment exit when the fork-PR-secrets-missing path triggers, which clud-bug handles gracefully

---
## 2026-05-15 02:45 - Docs: be explicit that agents should use logmind log not git commit

**Reasoning:** User asked whether our docs make it clear agents should invoke logmind log instead of git add/commit. The existing AGENTS.md template said 'logging is part of the work' but didn't explicitly tell agents not to bypass via git directly — a subtle but consequential gap. Adds an explicit do-not-bypass note to: AGENTS.md.template (full variant), AGENTS.md.slim.template, and SKILL.md (Don'ts section).

**Implications:**
- Both AGENTS.md variants and the published SKILL.md now carry the no-bypass guidance
- logmind-skill repo gets the updated SKILL.md as a follow-up commit

---
## 2026-05-15 02:46 - Site: by thrllmt attribution + locally-hosted logo from thrillmot.com

**Reasoning:** User asked for a by-thrllmt mark linking to thrillmot.com using the thrillmot logo. Pulled the SVG (thrllmt.svg) from thrillmot.com and hosted it locally in site/public/ to avoid hot-link dependency. Added a small attribution line in the footer with the logo + link.

**Implications:**
- Footer now has logmind. wordmark + version + by thrllmt logo + skills.sh badge stacked
- Vercel auto-deploys on push

---
