<!-- logmind-entry-start: 2026-05-29-0-b-6-trim-agents-md-logmind-block-v5-slim-v6-pointer-69-red -->
- **2026-05-29** — 0.B.6: trim AGENTS.md logmind-block v5-slim → v6-pointer (~69% reduction, data-justified by PR #78 per_session)
<!-- logmind-entry-end -->

## 2026-05-29 13:01 - 0.B.6: trim AGENTS.md logmind-block v5-slim → v6-pointer (~69% reduction, data-justified by PR #78 per_session)

**Reasoning:** Per-session data from PR #78 (just merged) showed agents_md_block_share = 0.51 (51% of AGENTS.md bytes read are the logmind-block) and per_file_share[AGENTS.md] = 0.36 — both decisively above the rubric thresholds (0.30 and 0.20 respectively, per the plan's Step 3). Block compressed from 2526 bytes / 48 lines to 774 bytes / 12 lines: drops the inline 5-step procedure ('That single command: 1. Writes 2. Regenerates 3. git add 4. git commit 5. git push') + 'Required reading' numbered list + the redundant npx-skills-add example, keeps the load-bearing 'logmind log is the commit primitive' rule, the bash example, the skill pointer URL, the brief mention of WIP-handling and required-reading files. Information is a strict subset — everything trimmed is in the skill at agent-skills/skills/logmind which most agent runtimes auto-load. test_get_agents_md_template_returns_slim_when_skills_available asserts v6-pointer marker + commit-primitive rule + git-trio replacement phrase + skill URL + 1500-byte hard cap. test_templates_v0_1_2.py::test_agents_md_install_url_points_at_collection gated on tmpl name (full template keeps inline install command; slim defers). 613 tests pass, 1 skipped (Windows). Block reduction: 1752 bytes / 36 lines per repo per AGENTS.md read. Across 6 consuming repos × per-session AGENTS.md read patterns, compounds to ~10KB+ per session org-wide.

**Alternatives considered:** Trim more aggressively (~400 bytes — just commit-primitive line + skill URL). Rejected: too sparse — agents without the skill auto-loaded would have insufficient context to know the canonical pattern. 774 bytes preserves the load-bearing safety net., Defer 0.B.6 until 0.B.5 ships first. Rejected: 0.B.6 has the higher-leverage data (51% block share, 36% file share) — shipping it first maximizes per-session savings while we evaluate the smaller 0.B.5 trim opportunity post-deployment.

**Implications:**
- v0.5.5 → v0.5.6 release. Composite pin unchanged (no clud-bug changes this release). Consuming repos pick up v6-pointer block on next logmind doctor + refresh cycle via existing inserter._replace_marker_block. v6 is informationally a strict subset of v5; no migration steps needed for agents.
- 0.B.5 (decisions.md per-entry compact) — to be evaluated post-deployment. Per data, decisions.md is 39% of bytes read but per-entry compression is structurally smaller (~5-10% per entry vs 0.B.6's 69%). Will re-measure once v0.5.6 lands in consuming repos and per-session bench picks up the new block size; decide based on incremental signal.

---
