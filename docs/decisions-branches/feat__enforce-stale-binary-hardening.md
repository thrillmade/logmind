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

## 2026-07-16 16:30 - Dogfood: refresh AGENTS.md to v9-pointer + install Claude Code PreToolUse guard in this repo

**Reasoning:** Verify the branch binary's doctor --fix end to end in the repo that ships it: AGENTS.md was still on the pre-migration v3-slim block and .claude/settings.json did not exist yet, so this repo itself was not exercising the enforcement it ships

**Alternatives considered:** skip dogfooding and rely on the automated test suite alone

**Implications:**
- AGENTS.md logmind block refreshed v3-slim to v9-pointer (matchingTemplate's unrecognized-marker fallback to the slim default, since v3-slim predates the marker scheme this PR's ordering guard covers)
- .claude/settings.json created fresh with the PreToolUse guard-commit entry (Bash(git *) matcher, timeout 10)
- doctor --fix also backfilled the deterministic first-decision-title marker into 133 pre-existing markerless docs/decisions-branches/*.md files (unconditional per backfillBranchSummaries doc comment); included here as part of the dogfood rather than reverted
- deliberately did NOT commit .gitattributes or .github/workflows/logmind-self-update.yml that the same doctor --fix run also created; both are genuine pre-existing repo gaps but unrelated to this PR's stale-binary-hardening/AGENTS-bump scope, left as local untracked artifacts for a separate change

---

