← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-17-fix-guard-commit-detect-git-commit-wrapped-in-sh-c-command-a -->
- **2026-07-17** — fix(guard-commit): detect git commit wrapped in sh -c / command / absolute git path (#221)
<!-- logmind-entry-end -->

## 2026-07-17 13:38 - fix(guard-commit): detect git commit wrapped in sh -c / command / absolute git path (#221)

**Reasoning:** The Layer-1 PreToolUse gate only saw bare/env/wrapper-prefixed git commit; agents commonly run commits via sh -c, the command builtin, or an absolute git path, all of which slipped past the pre-commit nudge undetected (#221).

**Alternatives considered:** Leave Layer-1 as-is and rely on the Layer-2 commit-msg hook. Rejected: the nudge should fire at tool-call time on the common wrapper shapes, not only after the commit lands.

**Implications:**
- shellCommands set plus a commandBase basename helper; sh/bash/zsh/dash/ksh -c recurses InvokesGitCommit over the inner command line (terminates as the inner arg is strictly shorter each level); command builtin joins the env-style stripped-prefix set; the git literal check now matches any basename git.

---

