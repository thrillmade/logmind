← back to [docs/timeline.md](../timeline.md)

## 2026-08-24 12:26 - config: refuse an agent's attempt to weaken a blocking setting, and make config get scriptable

**Reasoning:** SPEC §1.6 says review.strict_mode, git.enforce_commits and review.auto_fix MUST NOT be written by an agent, and a tool asked to weaken one MUST refuse and say why. Measured on dev: all three accepted the write under CLAUDECODE=1, exit 0, persisted — with a control showing a permitted key also succeeded, so config set worked and the refusal was simply absent. That is the enforcement story's own off-switch, reachable through the documented interface with no bug required. Second defect in the same surface: config get printed Python-capitalized False while the file held false, so a workflow comparing against 'false' took the wrong branch — against §1.6's requirement that the non-interactive form be scriptable.

**Alternatives considered:** Refuse every write to those keys — rejected; §1.6's very next paragraph protects an agent's right to write everything else, and setup plus skill registration are legitimate work. Only weakening is refused, so an agent turning enforcement ON still succeeds. Detect a hand-edit and refuse to honour it — rejected as a refusal, taken as an advisory: doctor now reports a weakened blocking setting without flipping Overall, because a person is entitled to have turned one off and doctor --fix writing it back would be logmind setting a blocking value on its own account.

**Implications:**
- The guard runs before the load and the write, so a refusal leaves an existing file byte-identical and creates none where there was none. Two owners, one fact each: internal/config/blocking.go owns which keys and which direction, internal/cli/config_blocking.go owns who is asking, and both config set and doctor read the first so they cannot disagree about what is protected. The bypass surface is stated plainly in the code and the README rather than implied away: any agent with a shell defeats it by unsetting a marker and answering the prompt, or by editing the file. It is a speed bump and a signal, not a boundary. Verified by me rather than taken on report: all three refused and persisted nothing, a permitted key still written, strengthening still allowed, and config get now emits lowercase true.

---

