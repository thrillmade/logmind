← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-03-feat-context-cache-optimal-cold-start-payload-stats-token-re -->
- **2026-07-03** — feat(context): cache-optimal cold-start payload + --stats token receipt (token-killer Phase 1a/1c)
<!-- logmind-entry-end -->

## 2026-07-03 22:37 - feat(context): cache-optimal cold-start payload + --stats token receipt (token-killer Phase 1a/1c)

**Reasoning:** Research: the ~10x token lever is prompt caching, and logmind's byte-stable re-read context is the ideal cache target. Re-architected 'logmind context': cut the human-facing boilerplate to one machine preface; wrapped the two docs in Anthropic's <document><source><document_content> envelope; reordered file-structure-first (stable) / timeline-last (volatile) so the cache prefix stays byte-identical longest; guaranteed byte-determinism (the property caching depends on). Added --stats: a deterministic ceil(len/4) token receipt (payload size + how much denser the timeline is than the raw decision logs — 115.8x on this repo).

**Alternatives considered:** Keep the prose format (rejected: boilerplate + volatile-first ordering waste tokens and bust the cache), An ML/LLMLingua compressor (rejected: lossy + requires shipping a model)

**Implications:**
- context.go is non-byte-gated (the free surface); new internal/tokens estimator = the toolchain's shared ceil/4 per the coming protocol §Token-efficiency contract; the Long help documents the caching placement (system-prompt, breakpoint, 1h TTL, pre-warm, above-the-task). Byte-stability now has a determinism test. The derived docs' OWN headers stay (byte-gated → 1d/Phase 2 / v1.0 flip).

---

## 2026-07-03 22:43 - context: make the missing-doc note strict-XML-safe (self-closing element, review fix)

**Reasoning:** The clud-bug review noted the missing-doc XML comment held a '--' double-hyphen (from '--write'), which strict XML forbids inside <!-- -->. Since the PR's pitch is unambiguous parsing, replaced the comment with a self-closing <document ... status="absent" regenerate="..."/> element — well-formed (the command lives in an attribute value) and still carries what's absent + how to restore it.

**Implications:**
- Only the degraded doc-absent path changes; the present-doc envelope is unchanged; test updated to assert the self-closing form.

---

