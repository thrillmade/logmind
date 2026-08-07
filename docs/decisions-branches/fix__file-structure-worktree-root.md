← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-07-derive-the-file-structure-tree-root-from-the-repository-not- -->
- **2026-08-07** — Derive the file-structure tree root from the repository, not the checkout directory
<!-- logmind-entry-end -->

## 2026-08-07 17:38 - Derive the file-structure tree root from the repository, not the checkout directory

**Reasoning:** docs/file-structure.md rendered its tree root as the checkout directory's basename. In a normal clone that basename happens to equal the repo name, so the bug was invisible; in a git worktree — which is how parallel agents work — it is agent-<id>, and every regeneration rewrote the root line to garbage. Measured inside a real worktree: git rev-parse --show-toplevel returns the WORKTREE path, so it is the wrong signal. git rev-parse --git-common-dir returns the one shared .git every worktree points at, and basename(dirname(...)) of that gives the real repository name in every case tested — relative from a top-level clone, ../../.git from a subdirectory, absolute from a worktree. A fix was promised in #167 ('the default flip rides v1.0') and never landed.

**Alternatives considered:** Add a config key for the root label. Rejected: one already exists — root_label, honoured for both a literal string and 'auto' — so a new key would be a second way to say the same thing. Config still wins; only the unconfigured default changed.

**Implications:**
- The normal-clone rendering is byte-identical, which matters because docs/file-structure.md is a derived doc under a byte-comparison CI gate — any change there would churn every repo in the fleet. Verified: internal/tree/testdata/generate-file-structure.golden untouched, and rendering this repo from its normal checkout still emits 'logmind'. A non-git directory keeps the basename fallback and exits 0 rather than erroring. The regression test was checked against the old behaviour by reverting tree.go and watching it fail with resolveRootLabel(main checkout, "") = ""; want "myrepo". The issue's own reproduction now gives myrepo/myrepo where it previously gave myrepo/agent-deadbeef123.

---

