← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-14-collapse-the-decision-layout-main-is-a-branch-the-timeline-g -->
- **2026-08-14** — Collapse the decision layout: main is a branch, the timeline gets the cap
<!-- logmind-entry-end -->

## 2026-08-14 22:14 - Collapse the decision layout: main is a branch, the timeline gets the cap

**Reasoning:** SPEC 3.2 makes decision files append-only and uncapped, and 3.3 moves the bound to the rendering: timeline.md carries the 50 most recent, everything older renders to timeline-archive.md, both from the same sources every time. The old model capped the RECORD and archived by moving, which is why five repos showed 18 main-log entries and zero archive entries: nothing could commit to main directly, so the cap never fired and the archive never filled.

**Alternatives considered:** Work from the deletion list in the issue. Rejected in favour of grepping for the existing pair and each deleted symbol, which found sites the list omits and one defect the list could not contain: git checkout HEAD -- a b c and git add a b c are ALL-OR-NOTHING. With any pathspec untracked they restore or stage nothing, exit 1 and 128, both files left dirty. Naively adding the third path to the shell hooks would have silently switched the L2a restore OFF in every repo that has not yet regenerated on main. Both hook bodies now loop one path at a time.

**Implications:**
- The split cannot become a move by construction: timeline.Generate returns both halves from one collectMarked, neither output is ever an input, and there is no flag to write one without the other. Measured here: 50 recent, 134 archive. Two mutations SURVIVED on the first pass and both were tests on the thing changed rather than on observable output — a config guard placed on LoadAsMap instead of Load, and an integration test that ITERATED the very list it was pinning. Both replaced; twelve mutations now die. docs/decisions.md stays readable because 18 entries exist across five un-migrated repos and dropping the read would lose them the moment a repo upgrades. One consequence needs a ruling and is raised separately: a branch file contributes exactly one timeline row, so main.md 12 entries collapse to one dated at its oldest, and main will accumulate forever behind a row frozen at 2026-05-15.

---

## 2026-08-15 08:31 - Date a markerless row by its newest entry, not its first

**Reasoning:** A branch file with entry-block markers renders one row per block; a markerless file collapses to one synthesized row. That row was dated from entries[0]. Every branch file except one eventually closes — the branch merges, the file stops changing, and first and last sit days apart inside one window of work, so the choice is immaterial. main.md is the one file that never closes. Measured here: 12 migrated entries spanning 2026-05-15 to 2026-07-16 rendered as a single row dated 2026-05-15, sitting at line 544 of the archive, already far past the 50-entry cut. Anything logged on main was therefore invisible in the recent view, and would sink further as other branches accumulated.

**Alternatives considered:** Special-case the default branch and emit one row per entry for it. Rejected because it contradicts the rule the whole issue rests on — main is a branch like any other — and buys a permanent if-branch-is-default in the renderer. The CEO named the same instinct: main.md functions like any other branch file. The asymmetry is not that main is special, it is that main.md never closes.

**Implications:**
- One rule for every file: the synthesized row takes the newest entry, and its title comes from the same entry so the date and the text agree. It only MATTERS for a file that stays open, which is the honest reason to prefer it over branching on the branch name. Verified: main.md moved from 2026-05-15 to 2026-07-16 and floated out of the archive into the recent half. A closed feature branch is unaffected — its first and last entries are the same day. TestGenerateMainCanonical_LegacyMarkerlessFallback asserted the old rule in its own comment and is updated, not weakened: it still pins exactly one row, and a mutation emitting one row per entry now fails two tests.

---

## 2026-08-15 09:05 - timeline: conform to SPEC line 646 — a markerless branch dates by its FIRST decision

**Reasoning:** The panel caught that dating a markerless branch row by its newest decision violates a normative MUST at SPEC line 646: "Where none exists the producer MUST derive the sentence from the branch's first decision title, so a summary always resolves without a model call." I verified the line independently — fence parity 20 (even, so not inside a code fence) and it sits among six other MUSTs. Code conforms to the SPEC as written; the SPEC change is filed at thrillmade/protocol#97, not assumed.

**Alternatives considered:** Ship the newest-entry behaviour anyway, since it is better for main.md — the one branch file that never closes, so its first decision is permanently its date. Rejected: a producer that diverges from a normative MUST makes every other producer's output unpredictable, which is the whole point of a spec. The argument belongs in the issue, not in a silent divergence.

**Implications:**
- main.md keeps dating by its oldest decision until protocol#97 is ruled on. The code comment cites the issue so the divergence is a decision on record rather than a bug someone re-fixes.

---

## 2026-08-15 13:55 - decisions: read the default branch's log and the legacy archive from every branch, not just the one checked out

**Reasoning:** The panel blocked this branch on a regression it introduced. Moving main's decisions into a branch file left search scanning only the legacy top-level log and whichever branch happened to be checked out, so from a feature branch, which is where agents always are, searching main's entire history returned nothing. Measured in a real repository: main's decision matched zero times from a feature branch and once after the fix, with a control confirming the branch's own decision still matched and a token present in no file still matched nothing. The archive had the same shape of defect from a different cause: all four read paths stopped reading docs/decisions-archive.md, and the old default rotated at twenty entries, so any repository that had rotated lost that history from the timeline, from search and from show.

**Alternatives considered:** Migrate the archive's contents into the branch file on upgrade. Rejected: that is a write over an artifact the user owns, which is the exact class two other lanes are fixing this cycle, and SPEC line 1101 forbids it. The archive becomes a legacy read source instead, read by everything and written by nothing. Also rejected: hardcoding main as the default branch, since a repository whose default is trunk must find its own history; resolution goes through the same helper rebase, warp and pulse already share.

**Implications:**
- NonBranchSources becomes the single owner of the two legacy paths so a fifth read path cannot be added while forgetting one. Five mutations, all compiled, all died, including one that removes the archive and turns six tests red across all four read paths. Six existing tests asserted the bug and were inverted. A doctor check announcing a migratable archive is deliberately not built here: with the read paths restored there is no silent loss left to announce, and advising a migration with no safe tool to perform it invites hand edits.

---

## 2026-08-15 15:21 - decisions: one primitive enumerates every source, so the four read paths cannot disagree again

**Reasoning:** Search was the only read path that guessed a branch name instead of enumerating what exists, and the guess collapsed wherever origin/HEAD is unset: a single-branch clone, the shape actions/checkout produces, and any locally created repository. Measured before and after in four clone shapes, each with an in-repo control term that hit in both runs: three of the four returned nothing for a decision logged on the default branch and now return it. A second finding from another lane makes the guess worse than it looked, since init.defaultBranch equal to main ships inside Apple's command line tools gitconfig and a system-scope check misses it, so the resolver's last fallback is unreachable on a macOS machine and a test written against it passes for the wrong reason.

**Alternatives considered:** Harden the branch-name resolver so its answer can be trusted. Rejected: the other three read paths never needed a name, because listing the files that exist answers the question directly, and every additional fallback step is another way for the answer to be confidently wrong. Discovery now consults no resolver at all, which is why the gitconfig finding needed no work rather than a workaround.

**Implications:**
- The timeline archive gains a merge driver registration and stops being derived from the docs directory rather than from the write path, which had it reverting a tracked file while merging a scratch one. A half flag exists because the driver is handed a scratch file at the worktree root, where writing the pair would drop a stray. The no-archive flag keeps its meaning rather than becoming inert, because it already ships in consumers' instructions and cobra errors on an unknown flag, so removing it would break repositories we do not control; the archive stays searched by default and the receipt now reports whether one was actually scanned rather than echoing the flag. Ten mutations, all compiled, all died.

---

