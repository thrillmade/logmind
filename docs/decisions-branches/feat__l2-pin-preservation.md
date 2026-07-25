← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-25-make-the-derived-docs-invariant-opt-in-via-derived-docs-mode -->
- **2026-07-25** — Make the derived-docs invariant opt-in via derived_docs.mode plus a min_binary floor, and add the L2a/L2b pin-preservation layers
<!-- logmind-entry-end -->

## 2026-07-25 16:18 - Make the derived-docs invariant opt-in via derived_docs.mode plus a min_binary floor, and add the L2a/L2b pin-preservation layers

**Reasoning:** The protocol owner reviewed the SPEC amendment and returned HOLD on two blocking findings: the compatibility argument rested on file_structure.auto_update which nothing reads (reproduced against released v1.2.0 — an old binary regenerates on a branch and the new blocking gate then fails a required check), and the SPEC called the behavior opt-in while L0 and L1 applied it unconditionally. Defaulting the new mode to driver makes a v2 binary in an unadopted repo behave exactly like v1, which closes both that break and its inverse

**Alternatives considered:** Gate only on an adoption signal (rejected: an older tool cannot read the signal, so the old-tool break survives), Declare only a version floor (rejected: says who may adopt but not what a v2 binary does in a repo that never adopted, leaving the inverse break), Weaken the CI gate to advisory (rejected: trades away the structural guarantee to paper over an error in my own compatibility argument)

**Implications:**
- L0 reads the adoption signal from inside the shell hook body so one canonical body per hook keeps doctor byte-comparison working; the CI gate now passes with an explanation instead of blocking when a repo has not adopted; the binary lands before the SPEC reshape so the spec describes something that exists

---

