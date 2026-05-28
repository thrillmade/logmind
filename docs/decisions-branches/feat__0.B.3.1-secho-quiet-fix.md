## 2026-05-28 00:48 - v0.5.3: patch click.secho for LOGMIND_QUIET=1 (progress vs error fg)

**Reasoning:** v0.5.1 only patched click.echo, intentionally leaving secho untouched so red/yellow errors print. Side effect: progress lines using secho(fg=cyan) for ℹ notice and secho(fg=green) for ✓ success leaked through unsuppressed. User flagged 2026-05-28 that LOGMIND_QUIET=1 logmind log still emits 3 lines (ℹ, ✓, ok) instead of 1 (just ok). Fix: monkey-patch click.secho with a wrapper that suppresses when _QUIET unless fg in {red, yellow, bright_red, bright_yellow}. +2 tests: end-to-end log emits one ok line; direct-call regression guard asserts loud-color secho still prints under quiet mode. Smoke-tested: LOGMIND_QUIET=1 logmind log now emits exactly 'ok logged: <sha> "..."' — single line.

**Implications:**
- Future secho call sites adding NEW loud-color semantics should use red/yellow; using cyan/green for warnings would silently disappear under quiet mode.

---
