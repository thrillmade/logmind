## 2026-05-27 23:41 - B.3: logmind --quiet / LOGMIND_QUIET=1 with ok output (v0.5.1)

**Reasoning:** Symmetric to clud-bug 0.A.6. Monkey-patches click.echo at module load so all 185 call sites become quiet-aware without per-call edits. _ok() uses unpatched echo so summary always emits. click.secho(fg=red/yellow) untouched — errors + warnings still print regardless.

**Implications:**
- Group-level --quiet flag flips a module-level _QUIET; env var honored at import time
- Tests: 3 new cover --help advertises flag, --quiet show emits single ok, env var doesn't break --help

---
