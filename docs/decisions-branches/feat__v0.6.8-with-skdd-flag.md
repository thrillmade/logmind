## 2026-06-01 13:46 - v0.6.8: logmind init --with-skdd flag (unified install, Python entry)

**Reasoning:** User directive 2026-06-01 PM: pull Stream 7 forward for ease-of-use. The 3-command install (pip install logmind → logmind init → npx clud-bug init) becomes 2 (pip install + logmind init --with-skdd). Flag name uses BUNDLE TARGET (--with-skdd) not specific tool (--with-clud-bug) so future toolchain growth doesn't change the API surface. Symmetric mirror v0.6.33 ships on clud-bug side with --with-skdd opt-in for Node-first users.

**Alternatives considered:** Use --with-clud-bug flag name (rejected: ties API to specific tool; future tools would force flag explosion), Use --with-thrillmade flag name (rejected: brand-forward but the SkDD methodology framing better describes what users get — the loop, not the brand), Make clud-bug installation the default (rejected: aggressive — would install a Node tool when user might want logmind standalone in a Python-only repo), Interactive prompt during init (rejected: breaks CI/agent usage where prompts hang; opt-in flag is the right severity)

**Implications:**
- Mirrors the existing --install-hook pattern (line 338-347 in cli.py) — proven precedent for opt-in flag triggering a secondary action after main scaffolding
- Anti-loop: --with-skdd invokes 'npx clud-bug init' NOT 'npx clud-bug init --with-skdd'. v0.6.33's mirror does same in reverse. No mutual recursion possible
- Subprocess errors warn but don't fail logmind init — clud-bug is an additive layer; logmind side has already succeeded by the time --with-skdd runs

---
## 2026-06-01 14:04 - v0.6.8 fix: selective subprocess mock in tests (Windows OSError 22)

**Reasoning:** Windows CI failed: blanket subprocess.run mock intercepted ALL subprocess calls in cli.py, breaking git-status checks etc. that need real behavior on Windows. The 6th test (OSError case) already used selective mocking; applying same pattern to the other 2 tests

**Implications:**
- Selective mock pattern: intercept only when cmd[0] == 'npx', delegate everything else to real subprocess.run. Standard test isolation that should have been the original implementation

---
