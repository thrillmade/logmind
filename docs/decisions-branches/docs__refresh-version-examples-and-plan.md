← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-11-refresh-stale-version-output-examples-and-rewrite-docs-plan- -->
- **2026-07-11** — Refresh stale version-output examples and rewrite docs/plan.md for v2.0
<!-- logmind-entry-end -->

## 2026-07-11 15:09 - Refresh stale version-output examples and rewrite docs/plan.md for v2.0

**Reasoning:** README.md and docs/install.md still showed the example --version output as logmind 1.2.0 (spec 0.8.0), which no longer matches the shipped v2.0.0-dev / spec 1.0.0 values in internal/version/version.go; docs/plan.md still described long-shipped legacy Python Phase 5-10 IN PROGRESS work instead of the current Go v2.0 architecture and roadmap

**Alternatives considered:** Leave the stale examples and plan.md as-is since they are only illustrative text, not executable code, Regenerate plan.md purely mechanically from git log without editorial rewrite

**Implications:**
- Version-output examples in README.md and docs/install.md now read logmind 2.0.0 (spec 1.0.0); version-pin install examples (v1.0.0 curl/setup-logmind/go install) were left untouched since those are release-coupled
- docs/plan.md now documents the Go package layout, the main-canonical-only timeline, the token-killer surface (context/repomap/LOGMIND_QUIET/protocol section 14), and the two features still in progress before the v2.0.0 tag
- docs/custom-integrations.md needed a link restored (moved into the Python-era paragraph) to avoid a new orphan flagged by logmind check-links after the old plan.md link was removed

---

