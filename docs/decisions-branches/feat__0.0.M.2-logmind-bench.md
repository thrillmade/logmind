## 2026-05-28 16:41 - 0.0.M.2: logmind-bench internal Q7 enforcement (4-angle net-saver measurement)

**Reasoning:** First Phase 0.5 measurement infrastructure for logmind. Internal tool — NOT released to PyPI. Lives in bench/ + .github/workflows/bench.yml nightly. Four angles: (1) per-call — tempdir harness compares logmind <cmd> output bytes vs git equivalent; benchmarks 'log' (replaces add+commit) + 'show --brief' (replaces git log --pretty=format with date+title+sha). (2) worst-case — fresh tempdir, single logmind log, never reads back; the hardest guarantee. (3) per-session amortization — STUB (returns null net_pct); real impl needs session-log sampler from ~/.claude/projects. (4) org cumulative — STUB; depends on per-session first. Honest framing: only commands agents actually invoke per session belong in per-call; artifact-regen commands (timeline, tree, file-structure) belong to amortization. SMOKE-TESTED on this repo: per-call -18%, worst-case -58%. Q7-logmind PROVEN net saver on the foundational surfaces. CI gate: python -m bench exits non-zero on any non-stub angle being a spender; nightly workflow + PR comment summary with marker-based dedup. +9 tests assert the net-saver invariant + entry-point behavior. 600 pass.

**Implications:**
- Stub angles ship now so the 4-angle frame is in production. Real per-session + org-cumulative implementations land in follow-up PRs when session-log path is designed. Bench CI workflow uses paths-filter to only run on src/logmind/cli.py, core/**, bench/** changes — keeps PR CI fast.

---
