<!-- logmind-entry-start: 2026-05-15-install-logmind-workflows-in-own-repo-for-dogfood -->
- **2026-05-15** — Install logmind workflows in own repo for dogfood
<!-- logmind-entry-end -->

## 2026-05-15 00:27 - Install logmind workflows in own repo for dogfood

**Reasoning:** Merge of PR #9 didn't fire the aggregator on logmind itself because the workflow files weren't yet on main. Adding them as a tiny PR so the merge of THIS PR fires both workflows as their first live validation.

**Implications:**
- Aggregator will append a Merged: dogfood-workflows entry to docs/decisions.md on main
- check-doc-links will run on every future PR that touches *.md files

---
