← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-16-refreshed-the-marketing-site-for-the-v2-0-0-release-version- -->
- **2026-07-16** — Refreshed the marketing site for the v2.0.0 release: version bumps, a new commit-enforcement plus canonical spec file section, and real logmind context --stats density numbers replacing the defunct python bench
<!-- logmind-entry-end -->

## 2026-07-16 17:13 - logmind.dev v2.0.0: version bump + surface the v2 story

**Reasoning:** v2.0.0 is the headline release (main-canonical-only timeline, token-killer context/repomap surface, commit enforcement, canonical spec file) but the marketing site still read v1.0.0 everywhere and its measured section referenced a python -m bench that never shipped in the Go rewrite

**Alternatives considered:** Bump only the version strings and leave everything else as-is, Full page redesign

**Implications:**
- New enforced section documents the two-layer commit guard (commit-msg + Claude Code PreToolUse hooks) and the canonical spec file's WHY/WHAT/WHERE-TO framing
- measured section now quotes real logmind context --stats output from this repo (repomap 22.4x denser, timeline 7.2x denser) instead of the nonexistent python bench; these numbers will drift and should be refreshed at future releases
- Site now says spec 1.4.0 (matching the thrillmade/protocol SPEC.md current version) even though internal/version/version.go's SpecVersion still reads 1.0.0 today - that constant needs bumping before the v2.0.0 tag lands or the site and logmind --version will disagree
- opengraph card drops the frozen pip install line, promotes brew + curl

---

