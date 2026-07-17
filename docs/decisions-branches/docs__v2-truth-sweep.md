← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-16-v2-0-0-truth-sweep-retired-python-era-doc-examples-taught-th -->
- **2026-07-16** — v2.0.0 truth sweep: retired Python-era doc examples, taught the real v2 CLI surface (context/repomap/doctor --fix/headline/enforcement/the pulse) across README, SECURITY, ai-agent-files, plan, skill, and release tooling docs
<!-- logmind-entry-end -->

## 2026-07-16 23:00 - v2.0.0 truth sweep: retire Python-era doc examples, teach the real v2 CLI surface

**Reasoning:** Pre-tag sweep found docs still teaching v1/Python-era commands (log --template, stats, aggregate, pip install) that fail with unknown command in the real v2 binary, plus other stale references (SECURITY.md version table, custom-integrations.md BaseIntegration, config.yml.template's inverted --stage default, goreleaser changelog filters that miss scoped conventional commits) ahead of the v2.0.0 tag

**Alternatives considered:** Leave the docs stale until users hit confused post-tag issues, Patch only the MUST-FIX items and skip the cheap nice-to-haves

**Implications:**
- README Quick Start, SECURITY.md, docs/ai-agent-files.md, docs/plan.md, config.yml.template, and skill/SKILL.md now match the real v2 CLI surface (context, repomap, doctor --fix, headline, LOGMIND_QUIET, enforcement, the pulse)
- goreleaser changelog filters are now scope-aware and the release footer no longer claims Linux Homebrew support

---

