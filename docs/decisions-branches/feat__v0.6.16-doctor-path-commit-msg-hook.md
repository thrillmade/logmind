## 2026-06-02 13:38 - v0.6.16: multi-branch self-heal + PATH probe + commit-msg hook + AGENTS.md v6/v8

**Reasoning:** tokenomics agent verification report Concerns 2/3 + the bug surfaced by writing the multi-branch self-heal test: v0.6.15 default-branch skip was over-aggressive, dropping decisions on local merges. v0.6.16 tightens skip to HEAD==origin/<default> (true pull-up case) + adds doctor PATH-conflict probe to surface tokenomics-style stale-binary cases + adds commit-msg hook for dogfood enforcement + bumps AGENTS.md templates with REQUIRED framing. Every change has Go v1.0 parity carry-forward mapped to B-wave waves.

**Alternatives considered:** ship Concern 3 (doctor PATH) + commit-msg hook + AGENTS.md bump as separate v0.6.16/v0.6.17/v0.6.18 — rejected: bundling lets multi-branch test gate all four end-to-end in one PR; the dogfood loop validates the cohesion, revert v0.6.15 default-branch skip entirely and accept the unstaged-docs-on-main pain — rejected: v0.6.15 fix is real for the gh-pr-merge-squash pull-up case; need both behaviors, make merge driver smarter (read git index instead of working tree) — rejected: requires substantial logmind timeline rewrite + breaks the byte-identical CLI contract for the Go rewrite parity gate

**Implications:**
- consumer repos must re-run logmind init or logmind self-update to pick up the new commit-msg hook + refreshed post-merge hook body
- AGENTS.md template auto-migrates on logmind self-update when slot 1 between markers is unchanged (existing _replace_marker_block guard)
- Go v1.0 parity items B2/B3/B4/B6 must absorb the v0.6.16 changes; cross-binary parity test gates v1.0 release
- PATH probe firing STALE in CI/test environments needed an autouse stub in tests/test_doctor.py — real-environment behavior unaffected

---
