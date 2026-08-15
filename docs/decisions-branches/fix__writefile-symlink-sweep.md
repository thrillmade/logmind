← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-15-writeaudit-route-every-managed-write-through-the-symlink-ref -->
- **2026-08-15** — writeaudit: route every managed write through the symlink-refusing primitive, and fail CI on the next raw one
<!-- logmind-entry-end -->

## 2026-08-15 14:51 - writeaudit: route every managed write through the symlink-refusing primitive, and fail CI on the next raw one

**Reasoning:** os.WriteFile follows symlinks, so any site that asks whether a file exists, reads not-exist as safe to create, and then writes, can be redirected outside the repository by a planted dangling link. logmind runs inside repositories its user did not write. My own search found twenty-six sites across thirteen files rather than the twenty-three across twelve I was handed, and the difference was found by planting a call in a scratch file and confirming the pattern returned it before trusting where it returned nothing. Reproduced through real command paths rather than helpers: before the change a skill file, a provenance file, an executable pre-commit hook, a rendered file structure, a settings file and a draft were all written outside the repository, with sizes recorded; after it every one of those targets is absent and the tool path is a regular file.

**Alternatives considered:** Convert every site mechanically. Rejected: one site is reached only after a read already succeeded, so a dangling link cannot arrive there, and a deliberately symlinked pre-commit hook is a normal arrangement that the atomic rename would detach while also forcing the mode to executable, breaking a documented contract. That one is kept with the argument recorded rather than converted for consistency.

**Implications:**
- Fixing twenty-six sites without a guard only means the twenty-seventh arrives later, so the change adds an audit that parses the syntax tree rather than grepping, because a dozen comments mention the function in prose. It fails on an unlisted file, on a higher count in a listed file, and on a stale entry, so the entries covering other lanes must be deleted as those land instead of decaying into blanket permission. Two sites in dependabot.go are vulnerable and belong to another lane; they are listed as an explicit gap rather than silently permitted. Nine mutations, all compiled, all died, including one proving the guard catches a second raw call in a file the allowlist permits once.

---

## 2026-08-15 15:08 - writeaudit: the stale-entry check fired on its first real handoff, exactly as intended

**Reasoning:** CI went red on this branch while the same suite was green in the worktree it was built in, because CI tests the merge with the default branch and that branch had moved. The 300 lane routed one of init.go's writes through the primitive, taking that file from five raw calls to four, and the allowlist still claimed five. The audit refuses a listed file whose count has dropped as well as one whose count has risen, so it reported the entry as stale rather than quietly permitting a number that no longer described the file.

**Alternatives considered:** Make the allowlist a ceiling rather than an exact count, so a file getting safer never fails. Rejected: an entry that keeps passing after the reason for it disappears is how a temporary exemption becomes permanent permission, which is the specific decay this audit exists to prevent. The lane-handoff entries are supposed to be deleted as their branches land, and the only thing that reliably forces a deletion is a failure.

**Implications:**
- The count is lowered to four rather than removed, because the remaining calls are still real and still belong to another lane. When that lane lands the entry must disappear entirely, and the audit will say so by failing rather than by staying quiet. Full suite after merging the default branch: twenty-three packages passing, none failing.

---

