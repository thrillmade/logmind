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

