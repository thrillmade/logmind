← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-15-roadmap-mark-what-landed-and-hand-the-remaining-work-list-ba -->
- **2026-08-15** — roadmap: mark what landed, and hand the remaining-work list back to git
<!-- logmind-entry-end -->

## 2026-08-15 22:21 - roadmap: mark what landed, and hand the remaining-work list back to git

**Reasoning:** Eleven pull requests merged to the development branch since this file was written, and three of its sections described work as pending that is now built. Two of them were the load-bearing ones: the template reconciliation the fleet migration waits on, and the ignore-source merge that a protocol question was supposed to gate. That question turned out not to gate it at all, because giving the defaults their own source and resolving positionally conforms to the specification as written rather than needing a ruling.

**Alternatives considered:** Rewrite the remaining-work list to name exactly what is left. Rejected on this file's own history: it was blocked four times for carrying a second copy of a fact that git already owned, and a hand-kept list of what remains is precisely that. The reasoning for each item stays, because reasoning does not go stale; the question of whether an item is done is answered by a command.

**Implications:**
- The three issues filed after this section was written are recorded with what they actually were, since two were live paths that lost a user's file: self-update wrote a fragment over the whole of an agents file, and doctor overwrote one it had misjudged as unmarked. The reader is warned that an issue named here may already be built, because the closing keyword only fires on the default branch, so anything merged stays open until the branch reaches it.

---

## 2026-08-15 23:08 - roadmap: a shown command that silently truncates is the defect this file exists to prevent

**Reasoning:** The review found two things, both of them the exact failure shapes this file was rewritten to eliminate, reproduced by the edit meant to eliminate them. A table row still listed a protocol question as gating the ignore-source work, three sections above the paragraph I had just added saying it turned out not to gate it. And the command offered as the replacement for a hand-kept list of open issues drops eighteen of forty-eight, because the tool defaults to thirty and truncates without saying so.

**Alternatives considered:** Fix the one command the review named. Rejected: sweeping the file for the same shape found two more, one unbounded and one bounded at a number that merely happens to exceed today's count. A limit chosen to be larger than the current total is the same defect with a longer fuse.

**Implications:**
- Every listing command now carries an explicit bound, and the open-issue probe is paired with a total from the search endpoint, which cannot truncate, so a reader can tell when the bound has been outgrown. The protocol row records that the question was answered by shipping rather than by a ruling, and stays open for the specification wording it raises rather than being struck.

---

## 2026-08-16 00:16 - roadmap: four numbers were wrong again, including two the fix pass introduced

**Reasoning:** The recheck found the shipped template versions stated as five and eleven when they are six and twelve, and the twelve had landed in the very commit this branch was cut from. It also found the bound I added to the two listing commands lower in the file was not added to the two higher up, so the same silent truncation remained one screen away from its own warning. The consumer count was true only by excluding this repository from a search that includes it, and the symlink class was described as fully built while its last two writes are still open and close only when the sibling branch lands.

**Alternatives considered:** Trust the counts I had already verified once. Rejected: three of the four had been verified at some point and went stale or were verified against the wrong scope. The versions now carry the command that reads them, the search states what it returns rather than what remains after excluding this repository, and the class is described as open with the condition that closes it.

**Implications:**
- The limit is now described as headroom rather than a fact, because a number chosen to exceed today's total is the same defect with a longer fuse, and it is paired with a total from an endpoint that cannot truncate so a reader can tell when it has been outgrown. Both places that list carry the warning now, rather than one of them.

---

## 2026-08-16 01:01 - roadmap: state the unit, correct a version the file contradicted itself on, and add the two repos the fleet table missed

**Reasoning:** Three findings, all mine, and all the two shapes this file keeps failing on. The control beside the consumer count reported rows where the headline reported repositories, so no consistent reading of the sentence was true: the queries return eight repositories in twenty-five rows and two repositories in thirteen. A sentence twenty-five lines above the version table still called the shipped template eleven while the table said twelve. And the fleet table listed six repositories while the search two sections above named eight, omitting two that carry exactly the stale pair the table exists to enumerate.

**Alternatives considered:** Add the two rows and move on. Rejected: the table is what sizes the fleet migration, so an undercount understates the work, and nothing in the file would have caught the next omission either. The command that regenerates the table now sits beside it, so the next reader can rebuild rather than trust it.

**Implications:**
- Both numbers in the consumer sentence now state their unit, and the sentence records that an earlier revision quoted one against the other, because that is the mistake a reader is most likely to repeat. The two added repositories were measured directly rather than inferred from the search: both carry check-decisions at two and regen-timeline at four, matching the four already listed, with a repository the table already contained as the control.

---

## 2026-08-16 01:52 - roadmap: count repositories, never result rows, and let the regeneration command tell absent from unmarked

**Reasoning:** Two findings, both of them a shown command failing to produce the number beside it, which is this file's recurring failure. The row counts I added were not reproducible: the same query returned twenty-five rows for me and twenty-three for the reviewer on the same day, because the search endpoint's row count is not stable, while the repository count was eight for both of us. And the regeneration command I added to fix the previous round could not produce the table it sits under, because it collapsed a missing file and a file carrying no marker into the same word, and the table distinguishes them.

**Alternatives considered:** Re-measure the rows and state the new figures. Rejected: an unstable number does not become stable by being measured again, and writing one invites the next reader to treat a disagreement as a defect rather than as noise. The line now counts repositories and shows the derivation that produces that count rather than a raw listing.

**Implications:**
- The regeneration loop distinguishes three states rather than two, and the text records what it cannot do: it reads default branches only, so the row for the repository whose workflows live on a development branch was measured separately, and this repository is the producer rather than a consumer and is deliberately absent. Verified the shown derivation returns eight and the control returns two, so the command and the prose now agree.

---

## 2026-08-16 03:25 - roadmap: one issue was on both the built list and the after-the-tag list

**Reasoning:** The section this change exists to correct listed an issue as built and on the development branch, while a later section still listed the same issue as work remaining after the tag. Its fix is on that branch, measured directly rather than inferred from the issue state, which stays open because the closing keyword only fires on the default branch. A file whose purpose is recording what landed cannot carry the stale copy it was written to remove.

**Alternatives considered:** Annotate the later list rather than removing the entry. Rejected: two lists that mean different things about the same item is how this file has failed five times, and an annotation is a third statement to keep in agreement rather than one fewer.

**Implications:**
- The symlink class is now stated once as the single exception on that list rather than as a claim and a contradiction two sentences apart, since it genuinely is open and its remaining writes close with the branch still in review. The link-checking template carries the same forward reference the regeneration template already had, because the same branch moves both and only one said so.

---

## 2026-08-16 04:47 - roadmap: a fenced block inside a table cell breaks the table, and the closing paragraph rendered as prose

**Reasoning:** The derivation I added last round to make a count reproducible was placed inside a table cell as a fenced block, which ends the table early. Confirmed against the forge's own renderer rather than by reading the markdown: the table closes before the fence and the cell's final characters render as a stray paragraph, with the sentence truncated mid-clause. The control is a deliberately broken cell of the same shape, which reproduces the stray, against the corrected table which renders four rows and closes once.

**Alternatives considered:** Move the derivation below the table. Rejected: the number it derives lives in that cell, and a command a screen away from its result is the arrangement that let a stale figure sit unnoticed here twice. Inlined as code within the sentence instead, so the claim and its derivation stay adjacent and the table survives.

**Implications:**
- The list of issues built since this section was written no longer implies a one-to-one pairing by position, because it is not one: one pull request closed two of them, another closed most of a third that is still open. The version claims in the architecture document contradict the ones corrected here, and are a second owner of a fact this file already states with its measuring command; that is routed to the branch which holds that file rather than edited across a lane boundary.

---

## 2026-08-16 05:42 - roadmap: stop listing #241 as unbuilt, derive the merge closures instead of naming them, and correct the CI and template-guard claims

**Reasoning:** A panel found four claims false. The worst: #241 was filed under blocked-on-a-person as 'unbuildable as written' while logmind auto is shipped and on dev — verified against current source (root.go registers newAutoCmd, auto.go declares 'auto <profile>', internal/auto ships exactly one profile with retired names refused by name), not against the commit message. Control: grep newPlanCmd → 0, newAutoCmd → 1. A reader sequencing pre-tag work would have re-specified a shipped command. Second: the dev→main step hand-listed eight closures where the range actually carries sixteen — the exact hand-copied-answer defect this file's own rule forbids, so it is now the command that derives them. The comma branch is load-bearing: a first-#N-only probe returns fifteen and silently loses #260, because one commit spells it 'closes #278, #260, #284'.

**Alternatives considered:** Strike the #241 item — rejected; the command shipped and the issue is still open, so the honest correction is to state what is built and let the merge close it. Keep the eight-item list and just add the missing eight — rejected; a hand-kept list of a thing git already owns will drift again the next time a PR lands.

**Implications:**
- Also corrected: 'of the two workflows' claimed two where there are nine, four of which carry a branch-push trigger (all naming main) while two more fire on tags — and test.yml, the Go suite, was invisible to a reader using that section to learn what CI covers. The contradiction with the templates table is resolved by linking to it as the single owner rather than repeating the versions. And the workflow-template guard is TemplateMarker.Writable() inside installWorkflowTemplatesMode, not a symbol named installWorkflowTemplates that does not exist; refusals are reported on stderr, so 'silently left behind' was false while the conclusion it supported was right. Rendering re-verified against GitHub's own renderer: 4 tables, 24 <tr>; control, a fence planted in a middle data row, gives 22.

---

## 2026-08-16 06:11 - roadmap: run every command the file hands the reader, and let one paragraph own the tag boundary

**Reasoning:** A panel found the file's replacement for the stale-SHA line was a command that cannot execute: git rev-parse --short takes exactly one revision, so the two-ref form exits 128 with 'Needed a single revision'. A doc whose thesis is 'state the command, not the answer' had shipped a command nobody ran — worse than the stale number it replaced. Every fenced block and inline command in the file has now been executed and its documented output reproduced. Separately the file asserted the tag boundary in three places that had drifted apart: #310's remaining sites were filed pre-tag as closing when #301 lands, while #265+#257 were paired as post-tag — and #301, which closes #265, is open against dev and lands pre-tag.

**Alternatives considered:** for-each-ref for the two-SHA case — rejected; it exits 0 and prints nothing on a typo'd ref, and a silent wrong answer is the failure this file exists to prevent. Fix the three sequencing statements to agree — rejected; three agreeing copies are still three copies, and they drifted once already.

**Implications:**
- Ruling recorded: #265 is pre-tag and lands in #301; #257 is the single item in the run to the tag that waits for the tag, because setup-logmind installs from /releases/latest and v1.2.0 lacks --base. One paragraph in §2 now decides that boundary and the other sections link to it. Also corrected: skdd is refused generically, not with a reason — 'logmind auto skdd' is byte-identical to 'logmind auto banana', while only night carries a retirement note; the naive closes-probe drops one issue, not two, and #284 survives it by luck rather than coverage; and two transcripts were not verbatim. The lane also caught two errors it introduced mid-pass, including a boundary sentence that contradicted the ten post-tag items in the same file.

---

## 2026-08-16 06:34 - roadmap: correct the marker claim this file introduced last round, and stop asserting the tag boundary in three voices

**Reasoning:** Round 8's own fix stated a falsehood: it said logmind's workflows are all hand-maintained copies carrying no template marker. logmind-self-update.yml carries v11, is byte-identical to its shipped template, and doctor reports it current — so it IS refreshed like any consumer's. Control: check-decisions.yml differs from its template, so the diff distinguishes. A doc correcting a false claim by writing another one is the failure this file exists to prevent, which is why the round that introduced it had to be the round that found it. Two further contradictions: §2 claimed to run after §3 while two of its own three rows are already answered, and it called #257 'the single item' waiting on the tag while §4 named #277 as a second.

**Alternatives considered:** Say 'most workflows are markerless' — rejected; a hedge that avoids naming the exception is how the exception stays invisible. Reconcile the two sequencing sentences so they agree — rejected again; agreeing copies are still copies, and §4's duplicate clause is now deleted rather than corrected.

**Implications:**
- Ruling recorded: #277 is a payload of the fleet migration rather than a second waiter — it closes when #257 does, not on a schedule of its own. §2 owns the sequencing and §4 owns the defect. The lane also chose 'answered' over 'run' for §2's rows, because what was measured is a bypass granted on this repository, not one per consumer — a distinction the earlier wording lost. Method note now carried by both lanes: blanking a middle table row is a USELESS control, since GFM accepts pipeless rows and the counts stay identical; inserting a blank line before a middle row is the working one, dropping table 4 from 9 rows to 4.

---

## 2026-08-16 06:59 - roadmap: scope the #288 bypass to the repository it was granted on, and stop reading per-file facts as per-repository ones

**Reasoning:** I gave the lane a logmind-scoped fact — the steward bypass is granted — and it cleared a fleet-scoped row with it. Measured across every org repo: Integration:3951953 is on org-baseline and org-default-protection everywhere, but a third ruleset, reporulez-default, is active with an EMPTY bypass list on six repos including agent-skills, clud-bug and clud-bug-app. Rulesets aggregate, so one refusing ruleset overrides two that allow. Control: logmind's own two rulesets carry 2 and 3 bypass actors, so the probe distinguishes granted from refused. An operator reading the cleared row would migrate at tag time and put those repos permanently red on every merge to main touching a derived doc — the exact outcome that row exists to prevent, and something #288's own thread and docs/orchestrator-app.md both already said.

**Alternatives considered:** Restate the bypass as simply missing — rejected; it is granted on logmind, and flattening that loses the fact that the mechanism works and only the grants are outstanding. Count the repositories in the prose — rejected; the count rots, so the section carries the per-repo command instead and prints refusals and grants together, which makes the output its own control.

**Implications:**
- The class is now named and swept: a true statement about a narrow thing presented as a conclusion about a wider one. It has produced a finding in three consecutive rounds, twice inside a fix from the round before. The lane's own sweep found a fourth instance — reporulez has seven workflows and its logmind-self-update.yml carries marker v8, so 'reporulez is unreachable by refresh' was true per file and false per repository. Also corrected: #244's entry said 'Nothing to do' while the feature is unbuilt (git grep for LOGMIND_ISSUE on dev returns nothing; control, newAutoCmd resolves for #241, which the same section marks built), and a byte-identity pointer aimed at a template listed below it rather than above.

---

## 2026-08-16 07:21 - roadmap: stop hand-writing the markerless inventory and emit it, then trim #288's archaeology

**Reasoning:** Round 10 narrowed a per-repository claim to per-file and then shipped an incomplete per-file inventory as the resolution — reporulez has three markerless workflows, not two; check-doc-links was missed, and it is a genuine refresh target (ListWorkflowTemplates enumerates all four *.yml.template with no filter, and init.go loops that list). Every other fleet repo has check-doc-links marker-owned — protocol v5, agent-skills v5, clud-bug v4, clud-bug-app v4, rezgen v4, tokenomics v4 — so reporulez is the sole outlier and an operator working a hand-written list at #257 would leave that file stale and permanently unreachable by refresh. That is the third consecutive round where the correction carried the next defect, which is what settled the shape: the file no longer states the inventory at all, it emits it.

**Alternatives considered:** Write the corrected three-file list — rejected; a hand-kept list of a thing the forge owns has now been wrong twice in two rounds. Add the homebrew-tap bypass fact I measured — rejected on the reviewer's argument and the file's own rule: the checklist already carries it command-shaped, and writing the answer would be a fourth recurrence of the defect the header warns about.

**Implications:**
- The emitted list prints all three states rather than filtering, so the marked rows are the command's own control: a repository that comes back entirely marked proves the probe recognises a marker, which makes a NEEDS HAND-REPLACEMENT row a real one. Verified by running the block verbatim out of the file — rc=0, and it returns reporulez's three plus logmind's three dogfood copies, which the paragraph above already accounts for. The #288 section also lost nine lines of archaeology about its own superseded revisions; the command transcript and the labelled probe table both stay, because the transcript is what the file's rule demands and the table is the only place the control row is named.

---

## 2026-08-16 07:46 - roadmap: make the emitted inventory carry its own control, distinguish absent from refused, and name the unblocking action

**Reasoning:** Round 11 replaced a hand-written inventory with a command, and the command inherited three of the defects the file exists to prevent. It printed only refusals while the prose claimed the marked rows were its control — a control the reader had to write. Its empty case could not tell a missing file from a refused lookup, which is the exact defect this file calls out for the block above it. And its four workflow names were hand-written against ListWorkflowTemplates, which reads the embed directory precisely because adding a template is meant to be purely additive, so a fifth would have dropped out silently.

**Alternatives considered:** Fix only the two the reviewer called blocking — rejected; the other two are this file's own stated defect appearing in its own commands, and shipping that is worse than shipping a wrong number. Keep the two-file version table alongside the four-column inventory — rejected; that is two owners for one fact, so the loop is deleted and the table now says outright that it tracks two of four templates and points at the inventory for scoping #257.

**Implications:**
- The command now derives its names from internal/templates/github/*.yml.template and prints all four states: 80 rows, 26 marked, 6 needing hands, 48 absent, 0 lookup failures. The 26 marked rows are the control the prose promised, and they are now actually emitted. The lane control-tested its own zero on the LOOKUP FAILED arm by querying ?ref=nosuchref, proving the branch is live rather than dead — that zero is real. Also corrected: 'wherever it applies, regen-on-main fails with GH013' was false, because homebrew-tap has the same ruleset applied WITH the steward on its bypass list; applying is not refusing. And 'Blocked on a person' now names the move — add App 3951953 to that ruleset's bypass list, per repository, because there is no org-wide switch, which is why one grant on logmind cleared nothing else.

---

## 2026-08-16 07:59 - roadmap: defer the dogfood split to the note that owns it instead of asserting it wrongly

**Reasoning:** The sentence said logmind's own rows appear as NEEDS HAND-REPLACEMENT. Three of its four do; logmind-self-update.yml comes back marked v10. The round-13 lens judged no reader was misled, because the dogfood note four paragraphs up already names that exception, and still returned MERGE. I fixed it anyway: 'no statement in this file is false' is the bar this PR has spent thirteen rounds meeting, and the sentence describing logmind's own repository is a poor place to make the first exception.

**Alternatives considered:** Restate the count as three-of-four — rejected; a hand-written number here rots exactly like every other one this file has shed, and there are now two places it would have to stay true. Leave it on the lens's judgement that nobody is misled — rejected on the bar above.

**Implications:**
- The sentence now defers to the note rather than repeating it, so the two say it once between them and the claim stays true unless the note itself changes. Verified file by file against the command's real output rather than by reading: the three the note calls markerless each return NEEDS HAND-REPLACEMENT, the one it calls the exception returns marked v10, four of four. Render unchanged at 4 tables / 24 rows / 2 blockquotes.

---

