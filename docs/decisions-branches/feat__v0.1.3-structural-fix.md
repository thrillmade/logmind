<!-- logmind-entry-start: 2026-05-15-v0-1-3-kill-file-structure-conflicts-fix-agents-md-drift-aft -->
- **2026-05-15** — v0.1.3: kill file-structure conflicts + fix AGENTS.md drift after logmind log
<!-- logmind-entry-end -->

## 2026-05-15 10:56 - v0.1.3: kill file-structure conflicts + fix AGENTS.md drift after logmind log

**Reasoning:** Two structural bugs surfaced from PR #24's CI: (1) docs/file-structure.md regeneration during 'logmind log' on a feature branch guarantees a merge conflict against main — PR #24 hit this against PR #23; (2) sync_agent_files_from_config ran AFTER log's commit, leaving refreshed AGENTS.md as dirty working-tree changes (the v0.1.2 PR commit, the AGENTS.md refresh commit, and a chore commit all hit this in sequence). Fix: skip file-structure regen on non-default branches and let the aggregator workflow regen on main after PR merge; move sync to BEFORE the commit and pass modified agent files into log's scoped staging.

**Alternatives considered:** Gitignore docs/file-structure.md entirely — but loses GitHub rendering and the AGENTS.md install pointer that says 'read docs/file-structure.md', Always regenerate per-PR and let users resolve conflicts — current pain, demonstrated to fail, Keep AGENTS.md sync after commit, accept dirty tree — known footgun, just fixed

**Implications:**
- On feature branches, 'logmind log' no longer touches docs/file-structure.md → no per-PR conflicts
- Aggregator action regenerates file-structure.md on main as part of the merge-aggregation commit, so main always reflects its own tree
- AGENTS.md / CLAUDE.md / .cursorrules refreshes from sync_agent_files_from_config now ride inside the same commit as the decision log — no post-commit dirty tree
- Adds _changed_agent_files helper to cli.py + extra_scoped_paths param to log() — both used by 'logmind log' but available for any caller

---
