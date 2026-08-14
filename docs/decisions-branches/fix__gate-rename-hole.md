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

