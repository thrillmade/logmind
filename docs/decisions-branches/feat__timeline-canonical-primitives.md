← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-06-29-slice-2-pr1-timeline-entry-block-primitives-slugify-extract- -->
- **2026-06-29** — Slice 2 PR1: timeline entry-block primitives (Slugify, extract, detect) — zero wiring
<!-- logmind-entry-end -->

## 2026-06-29 18:27 - Slice 2 PR1: timeline entry-block primitives (Slugify, extract, detect) — zero wiring

**Reasoning:** First of 7 additive PRs for the main-canonical timeline. These implement SPEC §1.6.3 (slug derivation §1.6.3.1, entry-block stack scan §1.6.3.3, format detection) that PR3 (union generation) and PR4 (logmind log marker writing) build on. Shipped alone so the byte-critical slug algorithm is reviewed in isolation against the spec.

**Alternatives considered:** Inline the primitives into PR3 — split out so a wrong slug (= wrong union keys downstream) gets focused review

**Implications:**
- New internal/timeline/canonical.go: Slugify (§1.6.3.1 4-step), HasEntryBlocks (§1.6.3.3 detection), extractEntryBlocks (stack scan; tolerate-and-warn on unclosed/nested). NOTHING wired — default branch-divergent timeline (Generate/Render) untouched, byte-parity preserved. Slug truncation follows spec order (trim then truncate, so a truncated slug MAY end in '-').

---

