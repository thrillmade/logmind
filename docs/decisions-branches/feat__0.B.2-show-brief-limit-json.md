## 2026-05-28 00:09 - B.2: show --brief / --limit / --json (v0.5.2)

**Reasoning:** Phase B.2. Reuses iter_decisions from logmind.core.parser. Default behavior unchanged. --json bypasses quiet patch (primary output). Tests cover all three flags + combinations.

---
## 2026-05-28 00:17 - B.2 PR #70 fixes: JSON stdout cleanliness + test coverage gaps

**Reasoning:** clud-bug findings: (1) --json stdout was polluted by sync_messages + ok line — downstream parsers couldn't jq it. (2) Missing test coverage for --quiet --json + --json --all. Fixed: suppress sync chatter when as_json=True, route ok line to stderr via _ok(err=True). +3 tests.

---
## 2026-05-28 00:30 - PR #70 fixes: limit-only output bug + py3.8 Click compat + JSON-wins-over-brief test

**Reasoning:** clud-bug findings + py3.8 CI failures: (1) 🔴 show --limit N alone produced no output — parsed-view path only handled as_json/brief, control-flow hole. Fixed: brief-style rendering for limit-only (we only have date+title from parser, brief is the natural answer). (2) py3.8 CI failed 3 JSON tests with JSONDecodeError — Click 8.1.x defaults mix_stderr=True so the _ok(err=True) line landed in result.stdout. Added _separate_streams_runner() helper with try/TypeError fallback. (3) Added test_show_json_wins_over_brief for the help-text precedence contract. (4) Added test_show_limit_only_emits_decisions as regression.

---
