<!-- logmind-entry-start: 2026-05-30-feat-v0-5-10-warn-loudly-when-stage-scoped-leaves-tracked-mo -->
- **2026-05-30** — feat(v0.5.10): warn loudly when --stage scoped leaves tracked modifications unstaged (#59)
<!-- logmind-entry-end -->

## 2026-05-30 08:43 - feat(v0.5.10): warn loudly when --stage scoped leaves tracked modifications unstaged (#59)

**Reasoning:** Pre-fix silent-failure: logmind log "<title>" --stage scoped stages only logmind-owned files (decisions.md, file-structure.md, etc.); when a user forgot git add before running it, the commit shipped ONLY the decision-log entry. The intended file change stayed unstaged; PR diff did not match its description; CI reviewed unchanged code; reviewer flagged "PR does not match description"; user had to push a follow-up commit. Hit live in clud-bug PR #87 and reporulez PR #20 in the 2026-05-27 wrap-up session — repeating pattern flagged as a real bug.

**Alternatives considered:** fail loud (error + exit non-zero when unstaged tracked changes present + --stage scoped — rejected because users have legitimate WIP scenarios), interactive prompt (Continue anyway? [y/N] — rejected because logmind is used heavily in agent automation flows that have no stdin), intent-matching heuristic (substring match unstaged filenames against title/reasoning — rejected because too many false positives on common words like fix/update, and filenames don't always appear in commit messages)

**Implications:**
- Warn-not-block per Q6 invariant: commit still proceeds (the user may legitimately have unrelated tracked WIP they want unstaged); the warning appears on stderr after git_add and before git_commit so the user sees what is about to be left out.
- Untracked files (scratch/debug artifacts) do NOT trigger the warning — users opting into --stage scoped typically have intentional untracked WIP; only tracked-but-modified files surface the warning (the actual silent-failure scenario).
- New reusable helper git_handler.unstaged_tracked_modifications(path) — can be used by future scoped-stage tooling without duplicating the git diff --name-only invocation.

---
