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

## 2026-08-15 16:15 - search: a source named archive is not the archive unless it is also not a branch

**Reasoning:** Search was the one read path of four deciding what to exclude from a label alone, so a repository with a branch literally named archive had that branch's decisions dropped by the no-archive flag, and the receipt then reported something untrue. Show and repomap already consulted the branch flag in this same change, which is what made it a one-line divergence rather than a design question. The receipt was separately unpinned: a mutation forcing it to claim an archive was scanned survived the entire suite.

**Alternatives considered:** Rename the legacy label so a branch cannot collide with it. Rejected: the collision is not the defect. Deciding identity from a display label rather than from the field that records what a thing is would remain wrong for the next label anyone reuses, and three of the four paths already did it correctly.

**Implications:**
- The three raw writes in the git attributes package are routed through the refusing primitive as well, one of which this branch had added itself while registering the archive merge driver. A dangling symlink there had let init and doctor --fix write outside the repository while printing that the block was added and reporting it written, so the regression asserts on what the user is told as well as on what reached disk. Every mutation compiled and died, and the one reverting the attributes write reproduced the reported symptom including its false success line.

---

## 2026-08-15 17:40 - round 5: the timeline.md link breakage is transient by design, v13 splits the template collision, and two user-owned artifacts stop getting overwritten

**Reasoning:** The panel blocked on 12 broken links in the committed docs/timeline.md, all identical: `docs/timeline.md: missing -> decisions.md`, one per pre-rename entry the old renderer emitted before this branch's rename of docs/decisions.md to docs/decisions-branches/main.md. The branch cannot fix them — check-derived-docs forbids editing any derived doc on a non-default branch, and docs/timeline.md is exactly that — so this is the zero-conflict invariant doing its job, not a regression. Measured: `logmind check-links` on this branch reports the 12 rows above; regenerating in a scratch copy of this same tree (`logmind timeline --write docs/timeline.md`, simulating what regen-on-main does after merge) drops the count to 0 and `logmind check-links` reports "All markdown links resolve and no orphans found" — `grep -c '](decisions.md)' docs/timeline.md docs/timeline-archive.md` is 0/0 in the regenerated copy against 12/0 on-branch. check-links is advisory (#315), not a required check, so nothing blocks red in the meantime. A future rename of a decision file reproduces this exact shape; the fix is always the same regen, never a branch-side edit.

A second, unrelated defect: internal/templates/github/regen-timeline.yml.template's marker went v11 to v12 on this branch (the archive-gate addition) and, independently, v11 to v12 on fix/template-v12 (#314, a credential-chain and default-branch-name rewrite) — two branches, one number, different content. installWorkflowTemplates only compares marker STRINGS; it never compares bytes when they match. Whichever v12 a repo installed first would have kept it forever, silently, even after the other v12 shipped, and doctor would report that repo current throughout. Per ruling, #314 keeps v12; this branch's archive-gate change moves to v13, in both internal/templates/github/regen-timeline.yml.template and the installed .github/workflows/regen-timeline.yml (their shared header/check-derived-docs region stays byte-identical per TestRegenTimelineWorkflow_LockstepWithTemplate — verified after the edit). A new TestWorkflowTemplateMarkers_PinnedToContent in internal/templates/templates_test.go pins v13's SHA256; it is a git-conflict generator, not a runtime check, but it works from inside one repo's tree: the first branch to land a marker's checksum owns that number's content, and a second branch reusing the same marker for different content fails this test on its OWN CI the moment it merges or rebases past the entry, without ever needing to see the other branch's diff. It does not close the narrower case of two branches introducing the same new marker and both merging before either rebases onto the other — git's own textual merge conflict on this table's line covers that remainder, which is the same loud-not-silent outcome. Nothing in this codebase catches that residual case today, and nothing caught the actual v12/v12 collision either — a human review pass did.

Two LOWs, both real: internal/linkcheck/linkcheck.go's DefaultAllowOrphans dropped docs/decisions-archive.md while adding docs/timeline-archive.md, instead of keeping both — it only bites an archive with no parseable `## ` headers, which is exactly an upgrader's half-migrated state. And internal/gitattr/gitattr.go's addMissingLines matched on path pattern with no way to tell "this repo predates the pattern" from "the user deleted it on purpose" — reinstating a line someone removed on purpose is the same class of bug as overwriting a user-owned artifact.

**Alternatives considered:** For the addMissingLines fix — stamp a generation marker into the committed .gitattributes block itself, the same shape as the template markers above. Rejected: the block's byte format is a promise doctor and every consumer repo depend on (testdata/gitattributes-fresh.golden pins the exact 5-line shape), and EnsureBlock/addMissingLines take a bare path, not a repoRoot, so there was no signature-compatible way to hang state there without touching internal/cli/init.go and refresh.go, both owned by #306 this round. Per-clone git config (`logmind.gitattr-offered-lines`, read/written via the existing gitcli.ConfigGet/ConfigSet) needs no signature change — repoRoot is derived from path's own directory — and degrades to the old offer-everything-missing behaviour outside a git repo or with git absent, same failure mode ConfigureMergeDrivers already accepts.

For the panel's separately-noted inconsistency — an unreadable-or-dangling-symlink entry under decisions-branches/ failed loud in search and timeline but was silently skipped by `show --brief`/`--json` — the panel judged the underlying scenario contrived, but the fix was narrow enough to make anyway: decisions.SplitRaw/Iter correctly treat a genuinely-absent optional file (docs/decisions.md not existing in a v2+ repo) as zero-entries-no-error, and that same swallow was firing for a dangling symlink's identical ENOENT. Rather than teach SplitRaw/Iter to distinguish the two — used by many callers reading legitimately-optional files — collectShowEntries' extras loop (paths that came from decisions.ListSources' directory enumeration, so something IS on disk) now reads the file directly first and fails loud on any error, leaving the base-target call (a branch's file legitimately may not exist yet) untouched. Reproduced live before and after: `show --all --brief`/`--json` against a real dangling symlink silently omitted the other branch's decision pre-fix, now errors `read .../broken.md: ... no such file or directory`, matching search/timeline/show's own default text stream against the identical fixture.

**Implications:**
- HIGH-1 also fixed: internal/cli/timeline.go's --help replaced an accurate "Reads docs/decisions.md, docs/decisions-archive.md, and every docs/decisions-branches/*.md" with an inaccurate "Reads every docs/decisions-branches/*.md" — behaviour was unchanged (decisions.ListSources still reads both legacy files via NonBranchSources), only the sentence was wrong. Restored to match search's already-correct wording.
- Mutation-verified, private-copy restore + SHA256 byte-identity confirmed for every guard: TestAddMissingLines_DoesNotReinstateDeliberatelyRemovedLine (gitattr.go), TestCheck_DefaultAllowlistSkipsDecisionsArchive (linkcheck.go), TestShow_All_DanglingSymlinkFailsLoud (show.go), TestWorkflowTemplateMarkers_PinnedToContent (templates_test.go's own pin, mutated by editing the template body under the same v13 marker — exactly the collision shape it exists to catch). All four compiled and went red under mutation, all four restored byte-identical.
- Files touched: internal/cli/timeline.go, internal/linkcheck/linkcheck.go (+ linkcheck_test.go), internal/gitattr/gitattr.go (+ gitattr_test.go), internal/cli/show.go (+ show_test.go), internal/templates/github/regen-timeline.yml.template, .github/workflows/regen-timeline.yml, internal/templates/templates_test.go.

---

## 2026-08-15 19:32 - templates: move to v13 and pin every marker to the content it claims to describe

**Reasoning:** Two branches had independently moved the same template from eleven to twelve with different bodies. The installer rewrites only when the marker differs, so whichever landed first would have left every repository taking it permanently unable to receive the other, while doctor reported the file current. Nothing in the suite could see it, because a single repository cannot observe a sibling branch. Each marker is now pinned to a digest of the body it labels, which makes a same-branch recurrence impossible and turns a genuine two-branch race into a merge conflict on the pinned entry rather than a silent divergence.

**Alternatives considered:** Keep twelve here and let the other branch move. Rejected: the other branch is the template lane and its twelve carries the credential chain the fleet is waiting on, so it lands first by sequence rather than by preference. Also rejected: a test that reads sibling branches, which would make the suite depend on the state of a remote it does not control.

**Implications:**
- The residual case, where both branches merge before either rebases, is covered by nothing and the log says so rather than implying the guard is complete. The help text on the timeline command is restored to name every source it reads, having been narrowed to only the branch files while the behaviour was unchanged. The attributes helper now records which patterns it has already offered, so a line the user deliberately deleted is not reinstated on the next run.

---

## 2026-08-15 19:51 - gitattr: record a pattern as offered only after the write that offered it succeeded

**Reasoning:** The record that stops a deliberately deleted line from being reinstated was written from a deferred call, so it also fired on the error return. A write that never happened was recorded as offered, and the pattern was then skipped forever. The trigger is the symlink refusal this same branch added, and nothing surfaces it: the doctor probe checks only that the block sentinel is present, never that the patterns it should contain are, so it reports the file current while the merge driver for the archive is unregistered. The end symptom is conflict markers in the file this branch exists to keep conflict-free.

**Alternatives considered:** Drop the record entirely and accept that a deleted line comes back. Rejected: reinstating a line the user removed on purpose is the same class as overwriting an artifact they own. The record stays and now runs only where a write actually succeeded, or where there was nothing to write.

**Implications:**
- The record lives in the local git configuration, so it does not survive a clone and the line returns once in each new working copy; that limit is now named in the doc comment rather than left to be discovered, alongside the causes it already listed. Whether the doctor probe should verify pattern coverage rather than sentinel presence is a question for the lane that owns it, recorded rather than reached across for.

---

## 2026-08-15 23:05 - gitattr: a shipped driver's command string is frozen — new behaviour gets a new driver name

**Reasoning:** This branch had edited the existing timeline driver's command in place to add a flag, and the released binary cannot run it. Measured against the actual cask: the shipped form exits zero, the edited form exits one with unknown flag. Git swallows a driver failure into an ordinary content conflict, so a repository whose configured driver outran its binary would get conflict markers inside a file the documentation tells people never to hand-edit, with nothing naming the cause. Reproduced end to end on a two-branch merge with the released binary on the path: the edited form produced a conflicted state, the restored form auto-merged cleanly.

**Alternatives considered:** Keep the flag and require everyone to upgrade first. Rejected: the configuration is written by whichever binary last ran the fix and read by whichever is on the path at merge time, so the two can differ on one machine, and the failure is silent. The flag was never needed for correctness anyway; it suppressed an untracked file appearing beside git's scratch file, which is litter rather than damage.

**Implications:**
- The archive gets its own driver name carrying the flag, and an undefined driver name was measured to fall back to an ordinary text merge, so a repository configured by an older binary is degraded rather than broken. A test now freezes every shipped driver command, because the rule that makes this safe is not that the current string is right but that changing one is how the skew happens. The registration this branch added is also installed in this repository's own attributes file, with a test that fails when the defaults and the committed file disagree — shipping a merge driver the project does not itself use is the dogfooding failure it exists to avoid.

---

## 2026-08-15 23:57 - timeline: --write writes the file it was given and nothing beside it

**Reasoning:** My previous ruling called the stray archive beside git's scratch file untracked litter, and that was wrong twice over. The next log commits it, because staging everything is the default, and it then reaches every clone; and where a tracked file of that name already exists at the repository root, the merge replaces its contents and says only that it regenerated something. Freezing the driver command was right, so the fix belongs in the behaviour rather than the string: writing to a path now produces that path and nothing else, and which half of the split lands there is chosen by a flag.

**Alternatives considered:** Put the flag back on the timeline driver so the pair write is suppressed. Rejected: the released binary cannot run a command carrying that flag, and git turns the failure into ordinary conflict markers inside a file people are told never to edit by hand, with nothing naming the cause.

**Implications:**
- The pair write, the flag and the archive are all new in this change, so the frozen command means the same thing on the released binary as on this one, and no shipped hook or workflow ever wrote an archive that could regress. The merge test asserts the fixture actually diverges before checking, so the driver is known to have fired rather than assumed. Correcting the template turned up a sixth copy of the abolished routing rule and a neighbouring claim that log regenerates the derived documents when it restores them.

---

## 2026-08-16 00:42 - decisions: an unborn HEAD has a branch — the fallback is detachment, not the absence of a commit

**Reasoning:** My previous correction said a detached or unborn HEAD routes to the legacy log, and the second half was false. Resolving the symbolic reference does not dereference it to a commit, so it answers with the branch name before any commit exists, and a decision in a fresh repository lands in the branch file. Measured against the real binary in a repository where verifying HEAD fails, with the detached case alongside as the control, since that one genuinely does fall back. The claim had been copied into eight places including two surfaces shipped to consumers, and the root was the branch resolver's own contract, which every other site re-derived it from.

**Alternatives considered:** Correct the seven sites this change introduced and leave the rest. Rejected: two pre-existing sites carried the same falsehood, one of them the resolver's own documentation, and leaving the root would have left the next reader to re-derive it exactly as these did.

**Implications:**
- The three routes to the legacy log are branch-awareness turned off, a directory that is not a repository, and a detached head; the count was right and only one description was wrong. The link-checking template moves a generation because a body-only fix reaches no repository already on the previous one, which a guard I had not been told about independently demanded. The agent configuration files for two editors now name the branch directory and both timeline files first, with the legacy log kept last rather than dropped, and a test drives the real creation path and enumerates the agents from the registry so a future one is covered without being remembered.

---

## 2026-08-16 01:21 - timeline: render the cap from the constant, so the file cannot assert a number the code disagrees with

**Reasoning:** The header this change added wrote the cap into the prose of every generated file as a literal, beside a constant whose own comment claimed the renderer was its only consumer. Changing the constant to three left the whole package green while every timeline in the fleet would have gone on asserting fifty. That is the same two-owners-of-one-fact failure the workflow fingerprints exist to catch, in a file the tool generates rather than one a person maintains.

**Alternatives considered:** Add a test asserting the header says fifty. Rejected: that is a third copy of the number, and it would go stale in the same motion as the second. The prose renders from the constant, and the test compares the rendered header against it rather than against a literal, so there is nothing left to disagree.

**Implications:**
- Verified by the mutation that previously survived: setting the constant to three now compiles and turns the package red, and restoring it returns to green. The restatements in documentation a person maintains were judged individually rather than templated, because prose written for a reader is not output the tool generates and mechanically deriving it would trade one staleness for a worse one.

---

## 2026-08-16 01:28 - show: rename the legacy JSON source from main to legacy, and correct what the previous entry left out

**Reasoning:** The previous entry described only the cap rendering, while the commit it recorded also renamed a value in a normative schema and reworded three surfaces shipped to consumers. A record that says less than its commit did is the failure this project exists to prevent, so this entry covers the remainder rather than leaving it to be rediscovered from a diff. Before this change the bare source token meant a decision made on the default branch; afterwards that is a branch-prefixed token and the bare one means the legacy file, so a consumer keying on it silently reads the wrong set. The schema is normative but unreleased, so the ambiguity is free to remove now and expensive later.

**Alternatives considered:** Keep the token and document the change of meaning. Rejected: a normative schema whose value quietly changes what it denotes is worse than one that changes shape loudly, because nothing on the consumer's side fails. The label has one owner and the rename propagates from it.

**Implications:**
- A correction to the previous entry: the mutation setting the cap to three does turn the package red, but through the guard that cross-checks the hand-maintained documents against the constant, not through the header. The header now renders from the constant and therefore stays green under that mutation, which is the correct behaviour and was the point of the change; the entry implied the wrong guard was doing the work. The existing tests never caught the source collision because they ran without a repository, so the branch case could not arise; the new test builds a real one with a default branch and a legacy file side by side.

---

## 2026-08-16 02:27 - templates: render the cap into generated files too, and stop citing a spec clause that does not exist

**Reasoning:** My ruling that the remaining restatements were prose a person maintains was wrong for three of them. The configuration file is a verbatim copy of its template, and two workflows are scaffolded from theirs, so all three are output the tool generates and all three would have gone on asserting fifty after the constant moved. Separately, the JSON output has been described in code and in help text as a normative schema from a specification section that defines nothing of the kind: fetched on both branches, the shape it claims appears zero times, against a control term that appears once.

**Alternatives considered:** Bump the two workflow markers so consumers receive the corrected comment. Rejected after checking the merge base: both markers were introduced by this branch and nothing has shipped under them, so re-pinning the content fingerprints is correct and a bump would spend a generation for nothing. The agent-file template keeps its hand-typed number because its marker predates this branch and is installed, so a bump there would rewrite every consumer's file for a comment.

**Implications:**
- The wording fix turned out not to be cosmetic. The repository check cannot distinguish a directory that is not a repository from a git binary it cannot run, so that branch can fire inside a real repository on a real branch, and turning branch awareness off is a policy choice rather than a missing name. Only a detached head is genuinely nameless, and all three sites now say that instead of repeating the retired framing. The schema is described as this tool's own contract, pinned by its own test, and is worth a specification clause so consumers stop relying on something asserted from nowhere.

---

## 2026-08-16 04:24 - init: keep writing the legacy path as a pointer, so the released binary still recognises a repository this one scaffolds

**Reasoning:** This change moved the already-initialised sentinel to the docs directory and the config file, while the released binary still tests for the legacy decision log, which this change stopped creating. Measured with the actual installed binary against a repository scaffolded by this one: it reported creating the config, logged a first decision, and the config lost both enforcement keys. The control, the same binary against a repository scaffolded from the base, reported refresh mode and touched nothing. This branch reasons carefully about mixed versions for the merge driver and did not apply that reasoning here.

**Alternatives considered:** Revert the sentinel. Rejected: it would restore a check for a file the layout no longer has a reason to create, and the released binary would still be the one deciding what counts as initialised. Writing the legacy path as a pointer satisfies the old check honestly, says what the file is, and contributes nothing to any read path because the parser wants headers and a pointer has none, which was measured rather than assumed with a control appending one real header to watch a row appear.

**Implications:**
- One cost the ruling did not predict was found and closed: an always-present legacy source would have opened every fresh repository's show output with an empty banner, so entry-less non-branch sources are now skipped, which is what makes the no-cost claim true. The repository now carries the pointer itself, with a test asserting byte equality rather than existence, because nothing writes that file and any difference is therefore drift. Reporting a missing pointer from the doctor command is deliberately not added: the status type has no way to describe a condition that is reported without being fixed, so adding one would enrol the sentinel in the automatic repair this ruling declined.

---

