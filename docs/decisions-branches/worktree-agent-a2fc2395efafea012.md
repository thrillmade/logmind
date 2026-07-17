← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-17-fix-repomap-skill-template-literal-interpolation-scan-stable -->
- **2026-07-17** — fix(repomap,skill): template-literal interpolation scan + stable suggest sort + assert scanner Kind (#219 #223 #225)
<!-- logmind-entry-end -->

## 2026-07-17 13:55 - fix(repomap,skill): template-literal interpolation scan + stable suggest sort + assert scanner Kind (#219 #223 #225)

**Reasoning:** Batch D pre-tag cleanup of three confirmed adversarial-panel issues. #219: maskTSJS treated backtick as a flat single-char delimiter with no interpolation awareness, so a backtick nested inside a dollar-brace interpolation closed the template literal early and masked all subsequent real source, dropping every later declaration from the repomap. Rewrote the backtick branch as an explicit state machine: dollar-brace enters interpolation code mode at brace depth 1, and nested strings, comments, and template literals recurse through the same masking helpers so only a backtick reached in string-body mode closes the outer literal. #223: SuggestFromDecisions ranked map-derived candidates with an unstable sort and a lowercase-only tiebreak, so ties could reorder across runs and drop a different candidate at the top cutoff. #225: the credential-scanner baseline table declared an expected-kind field but discarded it, so a miscategorisation would pass.

**Alternatives considered:** #219: keep the flat single-delimiter scan and accept precision loss (rejected: silently drops real declarations after the literal)., #219: track interpolation braces structurally instead of neutralising them (rejected: masking the whole literal keeps repomap output byte-identical for all non-pathological cases)., #223: unstable sort.Slice with only a token tiebreak (rejected: SliceStable plus a case-sensitive final key is fully deterministic).

**Implications:**
- Repomap now retains declarations after a nested-backtick template literal; no golden changed because no fixture contained the pattern and the full suite stayed byte-clean.
- skill suggest ranking is deterministic regardless of Go map iteration order.
- Credential-scanner baseline now fails on a miscategorisation regression instead of passing vacuously.

---

