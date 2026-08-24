← back to [docs/timeline.md](../timeline.md)

## 2026-08-24 13:39 - agents migrate: preserve before destroying, and hoist every refusal ahead of the first write

**Reasoning:** MigrateToAgentsMD stubbed each per-tool file inside the loop and wrote AGENTS.md afterwards, so the user's content existed only in an in-memory slice that the error return discarded. Reproduced: two files carrying user conventions, AGENTS.md a symlink so the final write refuses — exit 1, both sources overwritten with the stub, the content in neither place. The control is what makes it an ordering defect rather than a design one: where the final write succeeds the migration is correct, moving the content into AGENTS.md and stubbing the source. It is the one command whose whole job is moving a user's words, and its failure mode destroyed exactly those words.

**Alternatives considered:** Stage every write and commit atomically — rejected; N renames is still N renames, so it moves the window rather than closing it, and leaves temp litter on failure. Pre-flight RefuseSymlink on AGENTS.md — written, then deleted: it duplicated the check inside atomicio.WriteFile with no observable difference, no mutant could kill it, and it read as though the invariant rested on it when it did not.

**Implications:**
- Three phases now: PLAN reads, refuses and classifies while writing nothing; PRESERVE writes AGENTS.md; STUB replaces the sources. The invariant is carried by the ordering rather than by a check, so nothing has to predict a full disk or a link planted mid-window — every PRESERVE failure happens while the sources are intact, and every STUB failure happens after AGENTS.md already holds the content. Two other late guards were hoisted: the per-source symlink refusal fired inside each file's own stub write, so a link on the fourth file was found after three had already been consolidated, and a link made ReadRedirectOwner classify some other file's bytes. An unreadable source now aborts the whole run instead of silently continuing — half a consolidation with no record of which half is worse than fix-it-and-re-run. Residual, documented rather than emergent: a STUB failure after PRESERVE leaves duplication, never loss. Verified by me: both sources sha256-identical after the refusal, and the happy path still moves content into AGENTS.md and leaves none behind.

---

