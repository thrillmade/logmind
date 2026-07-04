← back to [docs/timeline.md](../timeline.md)

## 2026-07-03 23:46 - Add repomap: deterministic Go signature skeleton (token-killer Phase 2, first slice)

**Reasoning:** The biggest structural token-saving win: file-structure.md gives an agent the name-tree (WHERE); the repomap gives the API surface it reasons over (WHAT) at a fraction of the tokens — 21.5x denser than raw source on logmind itself (550KB→25.5KB). Go extraction uses go/parser+go/printer (stdlib) for byte-accurate signatures with bodies dropped. Additive + experimental: new internal/repomap package + a standalone 'logmind repomap' command; touches NO golden-locked surface (file-structure/timeline unchanged), changes no config default.

**Alternatives considered:** tree-sitter (CGo grammars) — rejected: breaks the single-static-binary distribution + complicates goreleaser cross-compile. universal-ctags — rejected: external runtime binary. Regex-only for Go — rejected: go/parser is exact AND still zero-dep; regex is the fallback for non-Go languages in a later slice.

**Implications:**
- New internal/repomap (ExtractGo via go/parser, deterministic path-sorted output, bodies/fields collapsed) + 'logmind repomap' (quiet-aware). Later slices: per-language regex extractors, PageRank importance ranking, token-budget packing, and folding the skeleton into 'logmind context' behind a config key that flips on at v1.0.

---

## 2026-07-03 23:51 - repomap: preserve generic type params + make composite type rendering layout-independent

**Reasoning:** Self-review of the first commit found two correctness bugs: (1) typeSignature hand-built the string from ts.Name, DROPPING generic type params (type List[T any] struct rendered as 'type List struct'); (2) preserving the original brace token positions made an emptied composite render 'struct { }' for multi-line source but 'struct{}' for single-line — source-layout-dependent output that breaks the caching-determinism invariant.

**Alternatives considered:** Leave generics unhandled (logmind itself uses none) — rejected: the repomap must be correct for consumer repos too. Post-process the string — rejected in favor of printing the TypeSpec node (printer renders type params correctly) with a zero-position emptied body, then trimming the empty-brace suffix to the dense 'type Name struct' form.

**Implications:**
- Composite types now render as the dense keyword form WITH type params (type Stack[T any] struct), identically regardless of source brace layout. Added generics + layout-independence test coverage.

---

## 2026-07-04 00:07 - repomap review fixes: valid inline-composite rendering + walk robustness

**Reasoning:** Dual review (adversarial + clud-bug-lens) of #183: adversarial found a BLOCKER — strings.Fields flattening dropped the semicolon inline anonymous struct/interface fields need when put on one line (func WithAnon(x struct { A int B string }) is invalid Go), and made multi-line-formatted func signatures render with ugly, source-layout-dependent ', )' artifacts. clud-bug found a Medium — an unreadable directory aborted the whole walk, violating the 'never a gate' contract and diverging from internal/tree.

**Alternatives considered:** Tokenize + ASI-reconstruct (heavy); or accept invalid output (rejected). Chosen: print with a FRESH empty FileSet so the printer emits canonical single-line layout, then an ASI-aware join turns the only surviving newlines (inline struct/interface fields) into ';' — valid, canonical, layout-independent. Robustness: mirror tree's os.IsPermission/os.IsNotExist swallow; also skip testdata/ dirs and symlinked .go (both minor review notes).

**Implications:**
- All emitted func signatures are now valid Go (gofmt-verified); output is source-layout-independent (stronger caching guarantee); unreadable dirs, testdata/, and symlinks handled. Known limitations (const/var, constraint type-sets, build-tags, non-Go) documented for later slices.

---

