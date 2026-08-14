← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-14-close-the-rename-hole-the-shared-evaluation-change-opened -->
- **2026-08-14** — Close the rename hole the shared-evaluation change opened
<!-- logmind-entry-end -->

## 2026-08-14 17:17 - Close the rename hole the shared-evaluation change opened

**Reasoning:** A retrospective panel on merged dev found the gate passing 550 lines of new Go. Unifying the numstat flag lists in #287 dropped --no-renames, which the old bash template carried with a comment saying exactly why. With rename detection on, git renders a cross-directory rename as ONE row whose path field is 'old => new', and IsExcludedPath prefix-matched that whole string — so 'docs/notes.md => src/payload.go' was excluded as docs/ and the count came out zero. Reproduced independently: git mv docs/notes.md src/payload.go plus 150 appended lines, gate says '✓ 0 lines changed (below 20-line threshold)', exit 0. The first repro attempt did NOT trigger it, because appending 150 lines to a 1-line file drops similarity below git's 50% rename threshold; it needs a file large enough to still be detected as a rename.

**Alternatives considered:** Teach IsExcludedPath to parse the rename rendering and test the destination path. Rejected: git has at least two renderings — 'old => new' and the compact '{docs => src}/sub/file' — so a parser owes both plus whatever git adds later, and a parser that falls behind fails OPEN. Refusing to exclude anything carrying ' => ' fails CLOSED; the worst case is asking for a decision that was not owed.

**Implications:**
- Two layers. gitcli.numstatFlags is now the one list every count runs with, carrying --no-renames, so the three call sites cannot drift apart again — drift is what caused this. IsExcludedPath additionally refuses to exclude any path containing ' => ' as a second line of defence, because the first line was removed once already. Verified with four cases at true exit codes: rename out of docs/ blocks (1), plain 550 lines blocks (1), docs-only passes (0), under-threshold passes (0). Also closes --threshold as a fourth gate-clearer in range mode — worse than --no-fail because it reads as configuration rather than an escape; §3.4 pins the gate's threshold to git.commit_line_threshold and nothing else.

---

## 2026-08-14 17:57 - Pin the rename fix on git's real output, not on synthetic rows

**Reasoning:** An adversarial panel found the fix's own test was worthless as a guard. It reverted numstatFlags to just --numstat — reproducing #287 verbatim — and the full suite stayed GREEN, because the test asserted on synthetic NumstatLine values and never invoked git. I confirmed it independently before fixing. That is the exact trap: a test on the helper you fixed passes its own mutation and still goes green when the bug ships again. The PR comment even said 'the first line of defence was removed once already' while shipping nothing that would notice a third time.

**Alternatives considered:** Assert on more synthetic rows. Rejected — the bug is in what git EMITS, not in how we classify what we are handed, so no amount of hand-written rows can catch the flag going missing.

**Implications:**
- The new test drives a real git repository through a real rename and asserts the numstat output carries no ' => ' rendering. Mutation-verified in both directions: removing --no-renames now fails with 'numstat still carries a rename rendering (1 rows)', and removing the IsExcludedPath guard fails the consumer-side test. The source file has to be large enough to stay above git's ~50% rename-similarity threshold or the test passes vacuously, so it builds 400 lines and appends 150 — a guard the test states and checks. Also fixes the panel's other findings: --threshold's range-mode refusal shipped untested and now has one, and both flag-combination guards move ABOVE the not-a-repo check, because an invalid flag combination is an argument error that should not depend on where the process is standing. The config template's comment claiming the threshold is 'shared with check-decisions --threshold' now says what §3.4 actually requires.

---

