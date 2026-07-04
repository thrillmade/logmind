← back to [docs/timeline.md](../timeline.md)

## 2026-07-04 01:31 - repomap: generalize extraction into a language-dispatch registry (R4 framework)

**Reasoning:** R4 needs multi-language support. Generalize the .go-hard-filtered walk into an extension-keyed  registry (langExtractor{extract, isTest}); Extract (renamed from ExtractGo) dispatches by extension. Go keeps the exact go/parser path (extractGoFile → extractGoSource, parse from src) — byte-identical output. Additive: a new language is a new registry entry, no other change. Regex extractors (Python/TS-JS/Rust) land on top via the workflow.

**Alternatives considered:** Keep ExtractGo Go-only + add a separate multi-lang walker (rejected — duplicates the walk/ignore logic). tree-sitter/CGo per language (rejected — breaks the single static binary).

**Implications:**
- internal/repomap: langExtractor type + extractors map (Go registered); Extract() dispatches by ext; extractGoSource parses from string. Go output byte-identical (all Go tests + CLI byte-parity green). Per-language regex extractors add extract_<lang>.go + a registry entry each.

---

## 2026-07-04 02:08 - repomap: TypeScript/JavaScript extractor (R4) via a zero-dep regex+brace scanner

**Reasoning:** R4 adds the one non-Go language our toolchain touches (clud-bug is TS) plus the biggest ecosystem. Zero-dep stdlib (regexp/strings) — no tree-sitter/CGo, preserving the single static binary. Extracts top-level function/class/interface/type/enum/function-valued-const, depth-0 only (matching the Go path collapsing composite bodies to their bare keyword). A display+mask two-rendering scan neutralizes string and comment interiors so brace-depth and keyword matching stay robust. Deterministic (source order, no maps). Dogfooded on the real clud-bug repo: 52 files, 453 symbols, accurate, zero body-leaks or truncation artifacts.

**Alternatives considered:** tree-sitter/CGo per language (rejected — breaks the single static binary and cross-compile). Python/Rust extractors (dropped this slice — the toolchain does not use them; the extension registry makes any language a one-step add later). Class-member descent (deferred — depth-0-only matches the Go collapsed-composite density).

**Implications:**
- internal/repomap/extract_tsjs.go + registry entries for .ts/.tsx/.js/.jsx (isJSTestFile skips .test./.spec.). Go path byte-identical. Bundled protocol 0.11.0 SPEC adds the multi-language note plus the deferred --map-tokens note from R3.

---

## 2026-07-04 02:29 - R4 review fixes: direct-arrow const detection + extends word-boundary + language-agnostic text

**Reasoning:** Dual review found a MEDIUM (a semicolon-less data const latched onto the next statement's arrow, emitting a garbled signature on any no-semicolon codebase) plus minors: an arrow with an object-literal return type was dropped (common React idiom); a const bound to a ternary was misparsed; a base type ending in the letters 'extends' swallowed the class body; and the CLI help/Render message still said Go-only. Adversarial confirmed NO blockers (Go byte-identical, determinism, panic-safety all verified by 7.1M-exec fuzzing).

**Alternatives considered:** Patch each symptom separately (rejected — rewriting scanConst's arrow detection to require a DIRECT arrow: params -> optional return type -> => via matchAngle/matchParen/findArrow fixes the MEDIUM plus two minors at the root). Track regex literals for exact brace depth (deferred — a documented best-effort masker limitation).

**Implications:**
- No-semicolon codebases, object-return-type arrows, and ternary/data consts are now handled correctly; generic arrows still extract; the extends type-position checks are word-boundary-aware; help/Render text is language-agnostic. 5 new regression tests; full suite green; clud-bug dogfood stable at 52 files/453 symbols.

---

