← back to [docs/timeline.md](../timeline.md)

## 2026-06-29 18:41 - Slice 2 PR2: timeline.canonical + file_structure.root_label config keys (typed-only)

**Reasoning:** Second of 7 PRs for the main-canonical timeline. Adds the two config gates PR3/PR4/PR5 dispatch on. Typed-only (set in DefaultConfig) — deliberately NOT in DefaultMap, so 'logmind config list' output stays byte-identical to the Python reference (the silent serialized-output change the plan forbids). Surfacing them in config-list rides the v1.0 bump.

**Alternatives considered:** Add to DefaultMap too — rejected: changes config-list bytes, breaks the documented Python<->Go config byte-parity contract

**Implications:**
- New TimelineConfig{Canonical} + FileStructureConfig.RootLabel. IsMainCanonical() is fail-safe: ONLY the exact 'main-canonical' enables it, so a typo/case-variant can't silently flip output. Absent key -> branch-divergent (DefaultConfig seed + leafwise YAML overlay). Zero behavior change — nothing reads these yet.

---

