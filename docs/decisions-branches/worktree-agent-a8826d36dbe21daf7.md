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

## 2026-07-17 14:32 - fix(guard-commit): detect -c bundled with other shell flags (bash -xc, sh -ec, -x -c) (#221 review)

**Reasoning:** the dual-review flagged as MAJOR that the -c unwrap only matched -c as the exact second token, so bash -xc / sh -ec / bash -ic / bash -x -c all defeated detection and a real wrapped git commit slipped past the Layer-1 nudge; now scan the shell's option tokens for the first bundle containing c and recurse on the token after it

**Alternatives considered:** keep the exact words[1] equals -c check (rejected: misses every bundled/preceding-flag form, the MAJOR gap); a full getopt parser (rejected: overkill for a Layer-1 advisory; the scan covers the real forms and Layer-2 git-hook still backstops)

**Implications:**
- recursion still terminates (the command string is strictly shorter); a shell that runs a script file with no -c correctly does not match

---

