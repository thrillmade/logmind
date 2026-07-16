<!-- logmind-entry-start: 2026-05-15-refresh-repo-s-own-workflows-to-v0-1-2-fix-windows-utf-8-enc -->
- **2026-05-15** — refresh repo's own workflows to v0.1.2 + fix Windows utf-8 encoding bug
<!-- logmind-entry-end -->

## 2026-05-15 10:47 - refresh repo's own workflows to v0.1.2 + fix Windows utf-8 encoding bug

**Reasoning:** The aggregate run on the v0.1.2 merge commit failed with GH013 because logmind's OWN .github/workflows/logmind-aggregate.yml was frozen at the pre-v0.1.2 format (the bug the v0.1.2 template fix addresses). Applying the same PR-fallback fix to the live workflow. Separately: Windows pytest matrix failed with UnicodeEncodeError in _install_github_action_templates — write_text was missing encoding='utf-8' on the target file. Templates use unicode (→, em-dash) which Windows cp1252 default can't encode.

**Alternatives considered:** Run logmind init against the logmind repo itself to refresh workflows in one shot — but in-place edits keep the dogfood-specific intro comments and 'pip install -e .' line intact, Skip the Windows encoding fix and let it ride — but it's a 1-line fix that prevents future Windows-matrix failures on every init

**Implications:**
- Next merge to logmind/main won't fail the aggregator step
- Windows pytest matrix will go green again on the next CI run

---
