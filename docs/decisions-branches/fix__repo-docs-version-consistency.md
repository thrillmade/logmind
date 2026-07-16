← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-03-docs-repo-version-consistency-specversion-0-1-1-0-8-0-readme -->
- **2026-07-03** — docs: repo version consistency — SpecVersion 0.1.1→0.8.0, README badges/version, AGENTS.md Python-era dedup
<!-- logmind-entry-end -->

## 2026-07-03 12:27 - docs: repo version consistency — SpecVersion 0.1.1→0.8.0, README badges/version, AGENTS.md Python-era dedup

**Reasoning:** Version-consistency audit: version.go SpecVersion lagged at 0.1.1 while protocol is now 0.8.0 (merged in #26) and logmind implements the surface; README headlined PyPI/pyversions badges + a stale '1.1.0 (spec 0.1.1)' --version example; the repo's own AGENTS.md carried a Python-era concat tail (empty stubs + an embedded CLAUDE.md with pip/pytest/python -m build commands + a 'Phase 2 Complete' roadmap + an empty Cursor Rules section).

**Alternatives considered:** Keep SpecVersion 0.1.1 until every pre-existing SPEC-vs-code divergence is fixed (rejected: SpecVersion declares the TARGETED spec version; 0.1.1 is absurdly stale; the §3.1/isatty divergences are separate bugs)

**Implications:**
- SpecVersion → 0.8.0 (regenerated version.golden + updated the docstring); README drops the PyPI badges + shows '1.2.0 (spec 0.8.0)'; AGENTS.md gets a clean Go Project Overview + Development Commands. Left untouched: the managed clud-bug block, and the version-pin examples (floor/mechanism references, judgment call).

---

