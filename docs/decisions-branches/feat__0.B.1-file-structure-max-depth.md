## 2026-05-27 23:19 - B.1: file-structure.md default depth=2 (logmind v0.5.0)

**Reasoning:** Activates depth-truncation that already existed in _generate_fallback_tree. logmind/docs/file-structure.md drops 103 KB → ~10 KB on next regen; same shape across 6 other consuming repos. Compounds on every clud-bug review + agent session.

**Alternatives considered:** Keep default unbounded, opt-in via flag, Default deeper (depth=3)

**Implications:**
- tree(1) binary path uses -L max_depth+1; Python fallback uses existing _current_depth check
- logmind init + post-merge auto-regen pick up the default automatically
- --max-depth 0 on CLI requests full tree

---
## 2026-05-27 23:30 - Fix tree(1) -L off-by-one + --help docstring (clud-bug PR #68 findings)

**Reasoning:** Two legitimate critical findings: (1) -L max_depth+1 made tree(1) path display ONE LEVEL DEEPER than the Python fallback for the same max_depth; my 'root counts as 1' mental model was wrong about tree(1) -L semantics. (2) logmind tree --help claimed Default: unbounded but actual behavior writes depth=2.

**Implications:**
- Tests: added regression that asserts binary + fallback paths produce same depth shape at same max_depth value

---
