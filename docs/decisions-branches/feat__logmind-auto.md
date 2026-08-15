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

## 2026-08-15 13:59 - auto: refuse to write through a symlink, and pin the percentage guard on the class rather than three literals

**Reasoning:** The panel returned merge with two low findings, and low described the blast radius rather than the standard of the fix. A dangling symlink planted at the directive path made os.WriteFile follow it and create a file outside the logmind directory, which hands anything that can drop a symlink into a repository an arbitrary write out of a tool people run in repositories they did not write. Separately, the guard meant to keep a context percentage out of the directive compared against three literal strings, so a mutation adding pause_at 85 percent shipped green; reinstating the old test against that same mutation confirmed it stayed green while the new one goes red.

**Alternatives considered:** Guard the one call site the finding named. Rejected: a patch at each site plus a promise to remember is how the next writer reintroduces it. The refusal lives inside atomicio.WriteFile itself, so every existing caller inherits it without being touched, and the two lock file paths that cannot use a whole-content replace call the same exported check directly.

**Implications:**
- A repo-wide audit found no other write targeting the logmind directory: os.Create has zero hits and os.OpenFile only the unix lock, with the Windows path using syscall.CreateFile and checked by reading it. The grep was controlled first against the line the finding named. The percentage guard now matches any digit-led percent or percent-word notation under any key plus bare fractions between zero and one; the fraction case is deliberately kept because one of the three original literals was already a decimal, and dropping it would have narrowed the guarantee while appearing to widen it.

---

