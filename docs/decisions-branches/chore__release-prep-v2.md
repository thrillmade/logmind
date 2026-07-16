← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-16-release-prep-specversion-1-4-0-goreleaser-check-filter-moved -->
- **2026-07-16** — Release prep: SpecVersion 1.4.0, goreleaser-check filter moved to main, install pins and stale narrative updated to v2.0.0
<!-- logmind-entry-end -->

## 2026-07-16 17:20 - Release prep for v2.0.0: SpecVersion 1.4.0, goreleaser-check on main, v2 install pins

**Reasoning:** The binary still declared SpecVersion 1.0.0 while the protocol SPEC advanced to 1.4.0 with sections 15 and 16; the goreleaser smoke-test was branch-filtered to the retired v1-go-rewrite branch so the release pipeline had no CI proof; install docs pinned the never-tagged v1.0.0

**Alternatives considered:** Bump SpecVersion at tag time via ldflags, which would leave the committed constant and version golden stale

**Implications:**
- logmind --version now reports spec 1.4.0; the goreleaser config check runs on main PRs; setup-logmind action pins stay at that repos own v1 tags

---

