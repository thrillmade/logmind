← back to [docs/timeline.md](../timeline.md)

## 2026-06-29 19:34 - Slice 2 PR5: deterministic file-structure root label (RenderWithLabel + file_structure.root_label)

**Reasoning:** Fifth of 7 PRs. tree.Render hardcoded the root line to the checkout-dir basename, so the same repo in two different checkout paths produced a different first line and a perpetually-stale file-structure.md. RenderWithLabel takes an explicit label; an empty label reproduces today's basename behavior byte-for-byte (the default), a set label is checkout-independent.

**Alternatives considered:** Auto-derive from the git remote always — rejected: the remote name is itself non-deterministic (fork vs upstream, missing in shallow CI), so only an explicit config label is fully stable. root_label auto is an opt-in convenience that degrades to the basename.

**Implications:**
- tree.RenderWithLabel (Render delegates with an empty label for byte-parity); GenerateFileStructure threads cfg.FileStructure.RootLabel via resolveRootLabel (auto resolves through new gitcli.RemoteRepoName, else verbatim). Default empty root_label preserves all bytes; the default flip rides v1.0.

---

