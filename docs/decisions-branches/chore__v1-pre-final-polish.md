## 2026-06-02 22:54 - Lock SpecVersion to 0.1.0 (drop -draft suffix)

**Reasoning:** thrillmade/protocol SPEC.md tagged v0.1.0 FINAL (commit 86c2212). Binary's --version was still advertising spec 0.1.0-draft, which would mislead downstream tools and any binary the v1.0.0 final tag ships.

**Alternatives considered:** Keep -draft until the next spec drift, Cut SpecVersion to a const

**Implications:**
- Golden file (internal/cli/testdata/version.golden) must match: bumped from 'logmind 1.0.0-dev (spec 0.1.0-draft)' to 'logmind 1.0.0-dev (spec 0.1.0)'. No other consumers of SpecVersion in the tree.

---
