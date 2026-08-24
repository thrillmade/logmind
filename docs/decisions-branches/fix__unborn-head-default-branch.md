← back to [docs/timeline.md](../timeline.md)

## 2026-08-24 13:58 - gitcli: resolve the default branch from an unborn HEAD, so a non-main repo scaffolds its own branch

**Reasoning:** DefaultBranch ran a five-step search that never read HEAD, so 'git init -b trunk' followed by 'logmind init' baked branches: [main] into both workflow triggers — byte-identical to a repo actually on main, at exit 0, with doctor reporting the repo healthy. The workflows then never fire. That is the README's own Quick Start on any repo whose default is not main or master, and logmind is meant to work standalone. The function's own doc comment claimed only a repo offering no evidence at all lands on main; an unborn repo created with -b trunk does offer evidence, and logmind reads HEAD correctly elsewhere in the same run.

**Alternatives considered:** Special-case the unborn repo at each scaffolding call site — rejected; DefaultBranch is the one place that answers this question and every caller should get the right answer from it. Read HEAD unconditionally rather than only when unborn — rejected, and this is the subtle one: it would make every feature branch its own default and collapse onNonDefaultBranch entirely.

**Implications:**
- The new rung sits fourth, below origin/HEAD, conventional names and single-branch, and above init.defaultBranch. Below the first three so no answer they already give can change — a remote's declared default outranks a local ref that was never pushed. Above init.defaultBranch because that key is what git init consulted to write HEAD in the first place, and -b overrules it. It reads the FULL symbolic-ref rather than --short, because --short shortens refs/custom/x to a branch-looking custom/x, and it tests unborn-ness as 'the ref HEAD names does not exist' rather than 'symbolic-ref failed' — the latter is a trap this repo already fell into once, producing a false unborn-HEAD claim in seven places. Verified by me on three cases: trunk scaffolds trunk, unborn main still gets main, and a clone sitting on sidebranch with origin/HEAD=main still gets main. Three false doc claims in the same comment block corrected. One behaviour change surfaced a pre-existing inconsistency in onNonDefaultBranch rather than a regression: the lane proved the retargeted tests pass both with the fix and with it removed, and that inverting the nudge gate still kills all three.

---

