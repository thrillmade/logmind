## 2026-06-01 12:08 - v0.6.7: post-merge hook leaves derived docs unstaged (downstream bug fix)

**Reasoning:** Downstream agent reported: post-merge hook auto-stages docs/timeline.md + docs/file-structure.md after merge, blocking git checkout main on every PR cycle. Workaround git reset HEAD + git checkout -- was required every time, hitting every contributor every PR. Root cause: _POST_MERGE_HOOK_BODY had a git add line that's correct inside logmind log (regens bundle into decision commit) but wrong from post-merge (no commit being constructed). Removing the line fixes the bug. Auto-propagates via v0.5.12+ self-install on every logmind log.

**Alternatives considered:** Add --no-stage flag to timeline/file-structure write subcommands + call with --no-stage from hook (rejected: changes CLI surface for no benefit; logmind timeline --write doesn't stage anyway, only the hook's separate git add did), Hook runs write then git reset HEAD before exit (rejected: extra subprocess call for no benefit over just not staging in the first place)

**Implications:**
- Regression guard test asserts no anchored 'git add docs/timeline.md' line in installed hook body. Prevents future revert
- No propagation cycle needed — every consumer's next logmind log auto-rewrites the post-merge hook via existing v0.5.12 drift-detection logic. Downstream agent should see the fix on their next log invocation after pip-upgrade to v0.6.7

---
