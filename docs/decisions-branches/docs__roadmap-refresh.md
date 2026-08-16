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

