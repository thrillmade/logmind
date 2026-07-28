← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-26-implement-spec-1-3-2-decisions-rotation-and-restore-derived- -->
- **2026-07-26** — Implement SPEC 1.3.2 decisions rotation, and restore derived docs to the merge-base instead of HEAD
<!-- logmind-entry-end -->

## 2026-07-26 22:26 - Implement SPEC 1.3.2 decisions rotation, and restore derived docs to the merge-base instead of HEAD

**Reasoning:** Two SPEC MUSTs the shipping binary did not satisfy. 1.3.2 requires the oldest entry be moved verbatim to the archive at the cap and logmind never archived at all, so decisions.md grew unbounded against the SPEC, the file own header, and the README promise. Separately the derived-doc restore targeted HEAD, but the invariant is defined against the merge-base, so on an already-diverged branch the restore re-applied the divergence and the gate own repair advice silently no-opped

**Alternatives considered:** Rotate branch decision files too (rejected: SPEC 1.4 states branch files have no capacity cap, they are durable detail pages), Keep HEAD and only correct the error message (rejected: the merge-base IS the invariant definition, so restoring to it is the more correct implementation and it self-repairs)

**Implications:**
- Byte-exactness is structural — consecutive raw entry slices abut in the original file, so a contiguous subset reproduces the exact bytes and no entry is re-rendered. The build agent found unprompted that the pre-commit hook body hardcoded checkout HEAD in shell, which would have silently undone the Go-side fix in any repo with hooks installed; fixed there too. Archive template prose corrected to append-on-overflow per SPEC 1.5

---

## 2026-07-26 23:02 - Move the merge-base repair out of the commit path and into logmind warp

**Reasoning:** The merge-base needs a current origin ref, but nothing on the commit path refreshes one and logmind log is network-free by design, so on a clone that had not fetched recently the restore computed against a stale base and wrote an OLDER snapshot than the true merge-base — the branch then failed the very gate the hook had just fixed. Previously a wrong restore was a harmless no-op; that change made it actively write wrong bytes, which is strictly worse

**Alternatives considered:** Add a freshness threshold and skip the restore when the origin ref looks stale (rejected: a heuristic guarding a correctness property — tune it wrong and you either skip valid repairs or perform invalid ones), Fetch inside the hook (rejected: breaks the offline constraint and puts network latency on every commit)

**Implications:**
- Commit-path surfaces keep a clean branch clean using HEAD, which needs no ref freshness; warp already fetches so it owns the authoritative merge-base repair of an already-diverged branch. This also makes the gate own remediation advice true, since that advice is to run logmind warp. A new test deforms the origin ref and asserts the commit path is unaffected

---

## 2026-07-26 23:40 - Close the warp-to-log handoff seam: skip derived-doc paths that are already staged

**Reasoning:** The two-surface split had a seam where they met. warp repairs the working tree and index to the merge-base, then logmind log restored unconditionally to HEAD and overwrote it, so the commit captured the divergence again and the gate remediation advice silently no-opped one command later. L1 could not tell a deliberate repair from an accidental regen because it treated every working-tree change as accidental. Staging is the signal that already exists in git: unstaged means accidental so revert it, staged means intentional so leave it

**Alternatives considered:** Have L1 compare against the merge-base to tell a legitimate staged repair from an illegitimate one (rejected: needs the merge-base on the offline hot path, which is the constraint that produced this split), A sidecar marker file written by warp (rejected: a marker can go stale and needs its own lifecycle; staging already carries this meaning everywhere else in git)

**Implications:**
- This deliberately RELAXES L1 — a user who hand-stages a divergent derived doc now gets it committed, because staged cannot be told apart from staged-and-correct offline. Documented at the call site as an accepted trade with L3 as the backstop. Verified by mutation: reverting the skip makes the remediation-sequence test fail

---

## 2026-07-27 00:08 - Pin the hooks test repo initial branch so the suite does not depend on the ambient git default

**Reasoning:** Four CI jobs failed on a test that passed locally: the helper used a bare git init, so the branch name came from init.defaultBranch — main on my machine, unset on the runner — and the test then referenced main by name and got fatal invalid reference. The test was green for an environmental reason, which is indistinguishable from green for the right reason until something moves

**Alternatives considered:** Fix the single call site to discover the branch name at runtime (rejected: the next test using this helper inherits the same bug; pinning at the helper fixes the class)

**Implications:**
- Reproduced CI exactly by forcing init.defaultBranch to master, confirmed the identical error, then confirmed the fix passes and the reverted pin fails again. Matches how internal/cli helpers already pin it. Full suite re-run under the CI git config, not the local one

---

