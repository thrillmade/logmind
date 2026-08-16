← back to [docs/timeline.md](../timeline.md)

## 2026-08-16 11:03 - docs: cite protocol#77's ruling rather than the issue thread

**Reasoning:** The protocol owner ruled on #77 and asked for exactly this follow-through: logmind implemented the behaviour on the strength of a comment left on our own issue rather than a ruling there, so the citation pointed at a three-comment thread rather than at the operative text. The ruling names why that matters — a correct rule whose emphasis reads backwards to every implementer is in practice an unwritten one — which makes the difference between citing a discussion and citing a ruling load-bearing rather than cosmetic.

**Alternatives considered:** Leave it pointing at the issue — rejected; the thread now has three comments and a reader landing on the first would get the framing the ruling explicitly overturns. Restate the ruling in the doc — rejected; one owner per fact, and the doc already states the rule correctly in its own words.

**Implications:**
- The doc's prose and its five-state table already matched the ruling before it landed, which is the useful signal: the implementation and the contract were derived from the same reading. One row of that table — a foreign-marked file is left alone and reported — is true of logmind init and false of logmind agents add, which is filed as #338 and awaits a ruling on whether an explicit user request outranks another tool's ownership. Deliberately not pre-empted here.

---

