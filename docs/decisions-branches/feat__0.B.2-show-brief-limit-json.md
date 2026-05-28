## 2026-05-28 00:09 - B.2: show --brief / --limit / --json (v0.5.2)

**Reasoning:** Phase B.2. Reuses iter_decisions from logmind.core.parser. Default behavior unchanged. --json bypasses quiet patch (primary output). Tests cover all three flags + combinations.

---
## 2026-05-28 00:17 - B.2 PR #70 fixes: JSON stdout cleanliness + test coverage gaps

**Reasoning:** clud-bug findings: (1) --json stdout was polluted by sync_messages + ok line — downstream parsers couldn't jq it. (2) Missing test coverage for --quiet --json + --json --all. Fixed: suppress sync chatter when as_json=True, route ok line to stderr via _ok(err=True). +3 tests.

---
