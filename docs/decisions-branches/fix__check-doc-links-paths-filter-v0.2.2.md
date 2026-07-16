<!-- logmind-entry-start: 2026-05-18-v0-2-2-fix-paths-filter-bug-in-check-doc-links-yml-template -->
- **2026-05-18** — v0.2.2: fix paths-filter bug in check-doc-links.yml.template
<!-- logmind-entry-end -->

## 2026-05-18 15:08 - v0.2.2: fix paths-filter bug in check-doc-links.yml.template

**Reasoning:** The shipped check-doc-links.yml.template had  filters on pull_request + push triggers. When a PR doesn't touch markdown, GitHub Actions skips the workflow — no status report. But if check-links is in required_status_checks (as reporulez's clud-bug-logmind variant ships it), GitHub treats the missing report as 'expected but never reported' and blocks merge forever. Bit clud-bug PR #52 directly (template-marker PR had no markdown changes; sat blocked until a CHANGELOG entry was added as a fake-trigger). The fix has been in logmind's OWN dogfood workflow for months but never backported to the shipped template. Bumping template marker to v2 so v0.2.1's idempotent refresh auto-rewrites downstream installs.

**Alternatives considered:** Document the bug in a release note and tell users to add CHANGELOG-style fake-trigger markdown changes — terrible UX, Add a fallback null-check step that always reports success — defeats the purpose of the check, Leave paths filter but add docs telling users to NOT make check-links required — fights against reporulez's clud-bug-logmind variant which ships check-links as required

**Implications:**
- Workflow now runs unconditionally on every PR + main push. ~15s end-to-end, predictable gate behavior
- Template marker bumped v1→v2; v0.2.1+ idempotent refresh auto-rewrites the workflow on next logmind init in downstream repos
- clud-bug + reporulez + any repo using the canonical clud-bug-logmind ruleset no longer needs paths:-triggered fake-changes to unblock merges

---
