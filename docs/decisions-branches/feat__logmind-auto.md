← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-14-add-logmind-auto-set-the-repository-up-then-hand-over-to-a-h -->
- **2026-08-14** — Add logmind auto — set the repository up, then hand over to a human
<!-- logmind-entry-end -->

## 2026-08-14 21:55 - Add logmind auto — set the repository up, then hand over to a human

**Reasoning:** #241 is pre-tag by Ruling 12 and was blocked on two skills that did not exist. agent-skills#207 merged them as afe8461, so the setup surface can now be built. An operator otherwise hand-assembles the whole rig in-session — pause threshold, checkpoint convention, hard stops, digest expectations — and none of it survives the session, which is what happened on the tokenomics overnight run.

**Alternatives considered:** Ship the profiles the issue named — `logmind auto
night` and `logmind auto skdd`. Rejected on the merits, and reported for
override. `night` is refused because the skill was deliberately renamed AWAY from
the clock during review: #174's own rule is that the hour never starts the mode,
so a verb called `auto night` would teach the exact vocabulary its policy
rejects. `skdd` is refused because nothing defines what it would write. Only
`unattended` ships, and an unknown profile refuses by name rather than falling
back — which is #267 and #286's rule, now applied a third time.

**Implications:**
- No new config key. The directive is self-describing — its own marker and profile line — so a copy in config.yml would be exactly the hand-kept duplicate this repository spent the week removing. It carries durable policy only: a test asserts it contains no live-state keys, and another FORBIDS a percentage threshold, because the issue sketched a flat 90% and session-heartbeat explicitly refutes it — the skill wins over the issue that requested it. auto never overwrites an existing directive; a human authored that policy. doctor reports drift as an ADVISORY, not a probe row, because a probe row would flip Overall to DRIFT with no --fix path and make residualProbes' existing note a lie. Eight mutations, all compiled and all died. Two gaps reported not invented: skill install is a report plus the npx command, since the Go binary has no skills.sh integration and SPEC §5.2's subscription model is Planned at skdd#6; and the loop invocation is not printed because the wake mechanism is what the human names at handover.

---

