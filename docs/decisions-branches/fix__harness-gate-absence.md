← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-15-claudehook-report-engine-skew-which-is-the-harness-gate-s-ge -->
- **2026-08-15** — claudehook: report engine skew, which is the harness gate's genuinely silent failure
<!-- logmind-entry-end -->

## 2026-08-15 14:00 - claudehook: report engine skew, which is the harness gate's genuinely silent failure

**Reasoning:** The issue's premise turned out to be partly wrong, and measuring it changed the fix. Driving real headless Claude Code sessions with live PreToolUse hooks and reading the resulting transcripts showed that a missing binary is already loud: the shell's command-not-found exits nonzero and non-two, which the harness records as a non-blocking error carrying both the stderr and the full command string. Adding a command -v guard would make it worse, because to stay fail-open the guard must exit zero, and exit zero with stderr is the one combination that reaches nobody. The verifiably silent case is engine skew, where a logmind on PATH answers guard-commit but is not the binary that installed the entry: it exits zero and says nothing.

**Alternatives considered:** Prefix the command with a binary existence check as the issue proposed. Rejected on the measurement above: it addresses the case that already reports itself and silences it. Skew is worse on this layer than on the git layer because the settings file is cloned with the repository while git hooks are not, so a fresh clone can carry an entry no local binary matches.

**Implications:**
- The canonical command string is unchanged; only its comment, which previously argued there was no behavioural gain, now records what was measured. Skew is detected from the marker read inside the binary rather than through the shell, so there is no portability branch and no unknown-flag breakage, and it routes through the git layer's existing notice primitive rather than growing a second copy. The notice goes to stderr and, on the allow path, as a systemMessage the harness displays. Known blind spots: an entry living in user-level settings, and an unrelated binary named logmind. The old test asserting the guard's absence is replaced by one that runs the command under an empty PATH and pins loud, fail-open, and named; a mutation that adds the guard and updates the expected string passes the shape test and is caught only by the behavioural one.

---

