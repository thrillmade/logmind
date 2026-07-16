<!-- logmind-entry-start: 2026-05-27-fix-v0-3-3-drop-wall-clock-timestamp-from-docs-file-structur -->
- **2026-05-27** — fix(v0.3.3): drop wall-clock timestamp from docs/file-structure.md
<!-- logmind-entry-end -->

## 2026-05-27 06:19 - fix(v0.3.3): drop wall-clock timestamp from docs/file-structure.md

**Reasoning:** Downstream: write_file_structure already early-returns False when content matches. With determinism it now triggers on unchanged trees, the file isn't rewritten, and the hook's git add is a no-op. No hook change needed

**Alternatives considered:** Option 2 — commit-time-derived timestamp — more complex, no clear consumer, Option 3 — diff-aware staging in the hook — treats symptom not cause

**Implications:**
- Test added: test_update_file_structure_is_deterministic regens twice over a fully-realized tree, asserts byte-stability
- Existing checkouts may need one final commit/pull to drop the v0.3.2-era timestamp from CI-generated copies

---
