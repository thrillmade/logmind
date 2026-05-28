## 2026-05-27 23:19 - B.1: file-structure.md default depth=2 (logmind v0.5.0)

**Reasoning:** Activates depth-truncation that already existed in _generate_fallback_tree. logmind/docs/file-structure.md drops 103 KB → ~10 KB on next regen; same shape across 6 other consuming repos. Compounds on every clud-bug review + agent session.

**Alternatives considered:** Keep default unbounded, opt-in via flag, Default deeper (depth=3)

**Implications:**
- tree(1) binary path uses -L max_depth+1; Python fallback uses existing _current_depth check
- logmind init + post-merge auto-regen pick up the default automatically
- --max-depth 0 on CLI requests full tree

---
