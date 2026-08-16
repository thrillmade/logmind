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

## 2026-08-15 15:55 - atomicio: an atomic replace swaps the name, it does not write into the inode — one rule, and the earlier entry was wrong

**Reasoning:** Two independent panels found the same thing: converting an existing-file write silently widened its permissions, severed hardlinks and detached a deliberately symlinked target. This branch had argued in one place that exactly those consequences were the reason a write-through must be kept, and then caused them elsewhere with no note. The earlier decision entry on this branch argues that forcing the mode is inherent to the primitive; that is now false and is corrected here rather than left standing. The primitive reproduces the standard call exactly: the permission argument is a create mode, the umask applies to it, and an existing regular file keeps whatever mode it had.

**Alternatives considered:** Exempt the one site that needs write-through and convert everything else. Rejected: an exemption records that a rule was inconvenient, not why it does not apply. The keep now survives on the rule itself, because writing through a deliberately shared hook is the intent and git never checks out into the git directory, so a hostile repository cannot plant the link. Its old justification about mode clobbering was deleted because the rule made it untrue.

**Implications:**
- Sites are identified by the function that encloses them rather than by a count or a line number: a count lets a kept call be laundered into a helper that anything may call, and line numbers churn until people re-baseline the ledger out of habit. The guard catches aliased and dot imports by resolving from the import declarations, and states plainly what it cannot see, which is any write through a file handle already held. Nineteen mutations, all compiled, all died; one initially survived because no probe existed for a create without a truncate, which is the lock file's shape.

---

## 2026-08-15 16:30 - writeaudit: a banned primitive referenced is as dangerous as one called, and a flag whitelist beats a name prefix

**Reasoning:** The guard inspected only the function position of a call, so assigning the banned primitive to a variable and calling that hid it completely. This is not hypothetical: the codebase already contains that idiom with the safe primitive, so someone copying the pattern with the unsafe one is exactly the accident the guard names as its target, and it was not in the documented list of things it cannot see. Separately the open-file check accepted any identifier whose name began with the flag prefix, which contradicted the guard's own doc promising that an unreadable flag is banned rather than waved through.

**Alternatives considered:** Document both as known limits rather than closing them. Rejected: a limit worth stating is one the guard cannot close, and both of these were a few lines. A guard whose stated limits are wider than its real ones teaches people to route around it, and the whole argument for having a source scan here is that no behavioural test can cover this class.

**Implications:**
- Safe open-file flags are now a closed whitelist of the six the standard library defines as non-creating, so a constant merely named like one falls through to banned rather than passing on its spelling. The fsync this change introduced was itself unpinned and its removal survived the suite, which is now fixed by routing it through a replaceable reference a test can observe. The hardlink asymmetry between the two write sites is documented rather than equalised, because a shared hook is an advertised arrangement and a hardlinked agent file is not.

---

## 2026-08-15 16:59 - atomicio: pin the fsync's order, not its presence, and stop banning the flag a safe open needs

**Reasoning:** The test I accepted for the durability promise could not see the thing it promised. Moving the sync to after the rename compiled and the whole suite stayed green, because the file handle reports the name it was opened with, so a check for the temporary suffix still matched and a stat of that name still failed. Deleting the call was the only breakage it could detect, and deleting the call was not the only way to break it. Separately the open-file whitelist banned the flag that opens a path without following a symlink, which is the exact defence this change exists to encourage, so the most careful possible caller would have been flagged and would most likely have answered with an allowlist entry exempting their file forever.

**Alternatives considered:** Widen the whitelist by name shape, since the safe flags share a prefix. Rejected because a constant can be named like a safe flag and not be one: the flag that creates an unnamed inode matches the shape and is now excluded deliberately, with a test asserting it stays excluded so the widening cannot drift into accepting anything that merely looks right.

**Implications:**
- The spy now asserts the destination does not yet exist at the moment the sync fires, which is the ordering signal rather than a proxy for it, and both mutations die: moving the call after the rename and removing it entirely. A bare zero is accepted as read-only, on the same reasoning as the symlink flag, while any other unlabelled number stays banned because an unverifiable constant must not be the quiet way past the audit.

---

