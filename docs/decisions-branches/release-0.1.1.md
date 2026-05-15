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
