← back to [docs/timeline.md](../timeline.md)

## 2026-06-29 20:28 - Slice 2 PR7: agent-authored branch summary — logmind headline + log -H + per-log nudge

**Reasoning:** The timeline headline becomes a one-sentence summary of the WHOLE branch, authored by the agent (the LLM with full context; logmind makes no LLM call). New 'logmind headline' command + bundled 'logmind log -H' flag set it; the per-log nudge steers toward keeping it current (interactive at a TTY; a printed advisory for a non-TTY agent, which it acts on via logmind headline). First-decision title stays the deterministic fallback. Gated to main-canonical, so the default path is byte-stable.

**Alternatives considered:** logmind shells out to an LLM to summarize each branch — rejected: breaks determinism, adds an API-key dependency, costs tokens on every regen. The agent is already the LLM with context; it authors the sentence once, logmind persists it, the dumb union copies it.

**Implications:**
- New timeline.CurrentHeadline + ReplaceFirstHeadline (keep the date-slug key stable; refine only the visible line). New internal/cli/headline.go. log.go: -H flag + marker-block refactor (headlineText) + nudgeBranchSummary (reuses the runSelfHealLayer1 TTY gate; non-TTY never blocks). Key stable across refinements (tested). PR8 wires the convention into the skill + AGENTS.md templates; PR9 specs it.

---

