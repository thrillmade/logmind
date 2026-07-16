← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-16-harden-git-hook-layer-against-stale-logmind-binaries-bump-ag -->
- **2026-07-16** — Harden git-hook layer against stale logmind binaries + bump AGENTS enforcement prose to v8/v9-pointer
<!-- logmind-entry-end -->

## 2026-07-16 16:30 - Harden git-hook layer against stale logmind binaries + bump AGENTS enforcement prose to v8/v9-pointer

**Reasoning:** Layer 2's commit-msg hook relayed guard-commit's raw exit code via exit $? unconditionally; a stale-but-present logmind on PATH (an old Cobra build that does not know guard-commit, or the frozen Python 0.6.16 argparse CLI) exits nonzero for an unrelated reason and would abort every commit, including logmind log's own internal commit, bricking the tool on any machine with an old release still resolvable on PATH

**Alternatives considered:** leave the hook as a blind exit $? relay and just document the risk, special-case known stale-binary version strings instead of a distinctive exit code

**Implications:**
- guardCommitGitHook now returns exit 65 (EX_DATAERR) on block instead of ErrSilent; the hook checks rc -eq 65 before aborting and fails open on anything else
- Layer 1 (harness, PreToolUse exit 2) is unchanged and knowingly still over-blocks under a stale Python logmind since exit 2 collides with argparse's own error code; accepted, escape hatch exists
- AGENTS.md full template bumped v7 to v8 and slim bumped v8-pointer to v9-pointer with BLOCK enforcement prose + skip-logmind/LOGMIND_ALLOW_GIT_COMMIT/enforce_commits carve-outs; matchingTemplate ordering (pointer checks before bare-version checks) keeps v8 and v8-pointer from colliding

---

