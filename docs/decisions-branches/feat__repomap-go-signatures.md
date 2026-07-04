← back to [docs/timeline.md](../timeline.md)

## 2026-07-03 23:46 - Add repomap: deterministic Go signature skeleton (token-killer Phase 2, first slice)

**Reasoning:** The biggest structural token-saving win: file-structure.md gives an agent the name-tree (WHERE); the repomap gives the API surface it reasons over (WHAT) at a fraction of the tokens — 21.5x denser than raw source on logmind itself (550KB→25.5KB). Go extraction uses go/parser+go/printer (stdlib) for byte-accurate signatures with bodies dropped. Additive + experimental: new internal/repomap package + a standalone 'logmind repomap' command; touches NO golden-locked surface (file-structure/timeline unchanged), changes no config default.

**Alternatives considered:** tree-sitter (CGo grammars) — rejected: breaks the single-static-binary distribution + complicates goreleaser cross-compile. universal-ctags — rejected: external runtime binary. Regex-only for Go — rejected: go/parser is exact AND still zero-dep; regex is the fallback for non-Go languages in a later slice.

**Implications:**
- New internal/repomap (ExtractGo via go/parser, deterministic path-sorted output, bodies/fields collapsed) + 'logmind repomap' (quiet-aware). Later slices: per-language regex extractors, PageRank importance ranking, token-budget packing, and folding the skeleton into 'logmind context' behind a config key that flips on at v1.0.

---

