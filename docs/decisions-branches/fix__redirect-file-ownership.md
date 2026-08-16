← back to [docs/timeline.md](../timeline.md)

## 2026-08-16 10:41 - redirect files: splice logmind's own entry and copy every other byte through, per SPEC:1101 and protocol#77

**Reasoning:** logmind init replaced the SPEC §1.2 redirect files wholesale. Measured: one init run destroyed a hand-written CLAUDE.md while AGENTS.md's user prose survived in the same run — so preservation logic existed and this path simply did not use it. Silent, exit 0. The protocol owner then ruled that the governing sentence is SPEC:1101's FIRST — an installer MUST merge rather than replace, writing only the entry it owns — which applies whether or not logmind's marker is present. A file carrying our marker is not a file we own end to end. And the severity was worse than filed: line 1 of CLAUDE.md in agent-skills, reporulez and logmind itself is @AGENTS.md, Claude Code's import directive, so a whole-file replace does not cost a paragraph, it costs every rule in AGENTS.md being loaded at all.

**Alternatives considered:** Refuse only unmarked and foreign-marked files, and keep replacing our own — rejected on the ruling; that leaves the bug in the one state my first brief told the lane to leave alone. Anchor detection to line 1 — rejected; four of five CLAUDE.md files in this org carry @AGENTS.md on line 1, so line-1-only refuses all of them forever.

**Implications:**
- One classifier, planRedirectWrite, shaped after planBlockRefresh (#267) and reusing the existing MarkerOwnership vocabulary rather than a fourth copy — MarkerForeign is the only new value. CreateAgentFile now owns the read as well as the write, so writing without checking is unrepresentable; the signature change forced all three call sites. Refresh is a splice: everything outside the logmind entry's span is copied through, so a foreign marker and a bare @import are handled by one rule — outside our span — not two. Verified against the REAL files: agent-skills' and protocol's CLAUDE.md are sha256-identical after init, and the control on the pre-fix binary changes both and drops @AGENTS.md from 1 to 0. Full suite run by me, not taken on report: 23 packages ok, exit 0.

---

