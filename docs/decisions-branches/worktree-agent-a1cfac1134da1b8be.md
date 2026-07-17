← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-17-derive-agent-skills-notify-changelog-from-git-history-not-th -->
- **2026-07-17** — Derive agent-skills notify changelog from git history, not the frozen Python doc
<!-- logmind-entry-end -->

## 2026-07-17 13:40 - Derive agent-skills notify changelog from git history, not the frozen Python doc

**Reasoning:** docs/changelog-python.md froze at the Python era (0.6.14), so notify-agent-skills.yml sliced an empty changelog for every Go-era release and the auto-PR pipeline has silently no-oped since v1.x (issue 208). Commit subjects from git log between the previous tag and the current one need no maintained Go-era CHANGELOG.md and no published GitHub Release to exist yet, and reuse history the workflow already fetches with fetch-depth 0.

**Alternatives considered:** Reintroduce a Go-era CHANGELOG.md (goreleaser-seeded) and keep pointing the extractor at a file, Extract from gh release view / gh api releases instead of git log

**Implications:**
- Empty commit ranges (re-tag, prev==tag) fall back to a GitHub Release-page pointer, never an empty file that would re-trigger the no-op
- parse_changelog.py is deleted as orphaned; the 3 workflow links now point at the Release page instead of the stale changelog blob

---

