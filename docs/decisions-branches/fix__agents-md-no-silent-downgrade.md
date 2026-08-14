← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-14-stop-guessing-at-an-agents-md-marker-this-binary-has-never-s -->
- **2026-08-14** — Stop guessing at an AGENTS.md marker this binary has never seen
<!-- logmind-entry-end -->

## 2026-08-14 17:27 - Stop guessing at an AGENTS.md marker this binary has never seen

**Reasoning:** matchingTemplate recognised an installed AGENTS.md block by hardcoded strings.Contains against every marker generation it knew, so a block carrying a NEWER marker returned empty and EnsureAgentsMD fell through to its own default — silently replacing the repository's block with the slim template. Reproduced: a planted v10-pointer block came back v9-pointer while the command printed 'Refreshed AGENTS.md logmind block to current template'. Same failure shape as #286 but a different root cause: #286 had a version compare running the wrong way, this had no version compare at all, only membership in a hardcoded set. It fires precisely during a staggered rollout, which is the condition #257 creates by construction.

**Alternatives considered:** Extend the enum to include v10, v11 and so on. Rejected as the bug rather than the fix: it fails again at the next generation, and the whole point is behaving correctly against a marker written by a future binary. A test at bundled+1 proves it — an enum extended by one still swallows that case.

**Implications:**
- The enum is gone. The block's id is read with a regex and parsed into a generation plus a flavour suffix. Flavour comes from the -pointer suffix, so §1.1's full-versus-slim guard survives without any list and a never-before-seen generation lands in the right flavour instead of a default; an unknown suffix refuses. Order compares against the marker read out of our own bundled template, so nothing enumerates generations and the next bump cannot stale it. An unreadable id refuses with a DIFFERENT message naming both bundled markers, because ordering is not a fact there. Signature changes force every caller to handle the refusal — the compiler rejects a swallow — and one formatter beside the #286 one serves init, doctor --fix, self-update, agents update and agents migrate. Mutation-tested three ways with the mutations reverted and run, not reasoned about: restoring the pre-#267 line reproduces #267 verbatim across 11 tests.

---

