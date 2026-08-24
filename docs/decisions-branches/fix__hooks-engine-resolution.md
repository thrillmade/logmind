← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-14-make-the-commit-gate-report-its-own-absence -->
- **2026-08-14** — Make the commit gate report its own absence
<!-- logmind-entry-end -->

## 2026-08-14 17:28 - Make the commit gate report its own absence

**Reasoning:** The git hooks located their engine by bare name, so the hook bound to whatever PATH answered rather than the binary that installed it. Combined with §3.4's mandatory fail-open, an unrelated or older install disabled the gate silently. Reproduced against the REAL released v1.2.0 built from its tag, which genuinely lacks the verb: 'Error: unknown command guard-commit', then '[master a389240] 40 substantive lines, no decision logged', exit 0. Control with the current binary on PATH: blocked, exit 1. §3.4 names the gap exactly — 'Failing open MUST NOT be silent. A hook that cannot run the engine it was installed for MUST say so on stderr, naming what it looked for and what it found' — and 'a gate that cannot report its own absence will be trusted long after it stopped working.'

**Alternatives considered:** §3.4 offers pinning the engine path resolved at install. Rejected because ruling 4 forbids it: probeHook byte-compares the installed file against BuildCommitMsgBody() and commit-msg.golden pins the same bytes, so the body must be a pure function of version.Version. A pinned path makes it a function of the installing machine's filesystem — every correct hook on a machine whose logmind lives elsewhere would read as content drift, and no golden could be checked in at all. It also breaks on reinstall to a new prefix and needs a PATH fallback, reintroducing the bare-name binding.

**Implications:**
- The handshake is an environment variable, LOGMIND_HOOK_VERSION, not a flag. An engine that does not know the variable ignores it; an unknown FLAG errors out, which would convert 'the gate ran against an older engine' into 'the gate did not run' — in exactly the mid-migration fleet §3.4 cares about. The engine complains on MAJOR skew only, because a per-patch notice fires on every commit everywhere and gets ignored. What this remedy misses that pinning would catch: an unrelated logmind that exits 0 on guard-commit. post-merge and post-rewrite are left alone deliberately — they are regenerators, not gates, quiet by design for contributors without logmind, and carry the Python byte-identical contract. Ten mutations run and all killed, including notices-to-stdout, fail-closed-on-missing-engine, and skew-notice-blocks.

---

