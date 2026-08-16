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

