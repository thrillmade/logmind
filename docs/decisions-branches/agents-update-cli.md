## 2026-05-15 02:32 - Dogfood check-decisions workflow + fix branch-aware bug + always-run check-doc-links

**Reasoning:** v0.1.1 polish: install check-decisions.yml in logmind's own .github/workflows/ (we ship it as a template via init but never installed it for ourselves); fix the actual logmind check-decisions CLI command to accept docs/decisions-branches/<branch>.md as a documented change (the pre-commit hook + CLI command were hardcoded to docs/decisions.md, breaking branch-aware mode); drop the paths filter on check-doc-links so it always runs (interacts badly with required_status_checks rule on the reporulez ruleset)

**Alternatives considered:** Skip the dogfood install (leaves the gap; future fresh install would see new template via init but our own repo stays out of sync), Patch only check-decisions and skip the always-run check-doc-links fix (would mean using --admin for every PR that doesn't touch *.md)

**Implications:**
- logmind own repo now dogfoods all three init-installed workflows (logmind-aggregate.yml, check-doc-links.yml, check-decisions.yml)
- logmind check-decisions CLI + pre-commit hook now correctly pass on a feature branch when only docs/decisions-branches/<branch>.md was updated
- check-doc-links runs on every PR regardless of paths — ~15s cost, predictable required-check semantics

---
