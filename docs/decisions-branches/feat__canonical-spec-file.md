← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-16-add-canonical-spec-file-support-context-spec-file-fold-in-lo -->
- **2026-07-16** — Add canonical spec file support: context.spec_file fold-in, logmind init --spec scaffolding, and doctor advisories
<!-- logmind-entry-end -->

## 2026-07-16 16:32 - Add canonical spec file support: context.spec_file fold-in, logmind init --spec scaffolding, and doctor advisories

**Reasoning:** Agents need a hand-authored, forward-looking spec doc alongside the derived file-structure (what) and timeline (why) docs. Folding it into the context payload as the first, most-stable document maximizes prompt-cache prefix stability while keeping the existing payload byte-identical when spec_file is unset.

**Alternatives considered:** Allow an absolute spec_file path -- rejected: repo-relative keeps the config portable across checkouts and worktrees, and closes an out-of-root read vector, Let doctor --fix auto-create a missing spec file -- rejected: there is no honest mechanical content to author automatically, so --fix stays read-only for spec content and logmind init --spec is the explicit opt-in, Surface context.spec_file in config list defaults (DefaultMap) -- rejected: mirrors the existing context.repomap precedent of staying out of DefaultMap to preserve config list byte-parity

**Implications:**
- context.spec_file defaults to unset, so byte-parity with the pre-feature payload is proven by golden-file tests captured from the pre-feature binary
- logmind init --spec scaffolds docs/spec.md and sets context.spec_file only when both are absent/unset respectively, so repeated runs and refresh mode are both idempotent
- doctor surfaces four advisory conditions (configured-but-missing, configured-but-empty, unsafe path, and an unset-but-file-exists nudge) but never flips Overall to DRIFT, and --fix deliberately never creates or edits spec content

---

