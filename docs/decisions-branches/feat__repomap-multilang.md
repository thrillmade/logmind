← back to [docs/timeline.md](../timeline.md)

## 2026-07-04 01:31 - repomap: generalize extraction into a language-dispatch registry (R4 framework)

**Reasoning:** R4 needs multi-language support. Generalize the .go-hard-filtered walk into an extension-keyed  registry (langExtractor{extract, isTest}); Extract (renamed from ExtractGo) dispatches by extension. Go keeps the exact go/parser path (extractGoFile → extractGoSource, parse from src) — byte-identical output. Additive: a new language is a new registry entry, no other change. Regex extractors (Python/TS-JS/Rust) land on top via the workflow.

**Alternatives considered:** Keep ExtractGo Go-only + add a separate multi-lang walker (rejected — duplicates the walk/ignore logic). tree-sitter/CGo per language (rejected — breaks the single static binary).

**Implications:**
- internal/repomap: langExtractor type + extractors map (Go registered); Extract() dispatches by ext; extractGoSource parses from string. Go output byte-identical (all Go tests + CLI byte-parity green). Per-language regex extractors add extract_<lang>.go + a registry entry each.

---

