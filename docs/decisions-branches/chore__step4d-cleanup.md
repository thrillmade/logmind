← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-26-step-4d-cleanup-remove-five-flags-that-silently-did-nothing- -->
- **2026-07-26** — STEP 4d cleanup: remove five flags that silently did nothing, correct false docs, delete dead code and config
<!-- logmind-entry-end -->

## 2026-07-26 21:51 - STEP 4d cleanup: remove five flags that silently did nothing, correct false docs, delete dead code and config

**Reasoning:** Three exhaustive sweeps found user-facing flags that lied (agents update --commit never committed; four init flags printed a deferred note and no-opped), comments that described the permanent opposite of the shipped behavior, ~1200 lines of code nothing referenced, and config keys shipped into every scaffolded repo that nothing reads. A missing flag errors loudly; a silent no-op gets trusted by a pipeline

**Alternatives considered:** Keep the flags and document them as no-ops (rejected: help text keeps lying and --configure-github still silently no-ops for CI that believes it applied branch protection)

**Implications:**
- Removing CLI flags is legal in a major bump. Dead config keys are still PARSED for silent back-compat, only no longer emitted. An adversarial panel caught that the app-id to client-id rename would have broken the release pipeline at tag time — reverted with a comment; and that deleting a Python-parity test left the fresh-file gitattributes format pinned by nothing — restored as a pure golden test

---

