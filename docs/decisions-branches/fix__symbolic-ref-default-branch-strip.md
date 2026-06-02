## 2026-06-02 19:14 - fix: strip 'origin/' prefix from symbolic-ref --short output in post-merge hook

**Reasoning:** clud-bug review on #128 caught the pre-existing v0.6.16 bug: git symbolic-ref --short refs/remotes/origin/HEAD returns 'origin/main' (the prefix is NOT stripped by --short for remote refs), so the comparison ["$current" = "$default"] was always false in repos with a remote — the default-branch pull-up skip never fired. Then origin/$default expanded to 'origin/origin/main' which fails. Safe failure mode (always regen, no data loss) but a real UX bug carried byte-identically to Go

**Alternatives considered:** use git rev-parse --abbrev-ref origin/HEAD — rejected: returns same prefixed form, parse from git remote show origin — rejected: heavyweight, requires network access

**Implications:**
- shell parameter expansion ${default#origin/} strips the prefix cleanly without invoking another git command
- tests/test_merge_driver.py's multi-branch self-heal tests don't trigger this path (they use local-only repos with no remote, so default falls through to the 'main' fallback). Real-world repos with origin DO hit the bug
- patches both Python source (src/logmind/core/gitattributes.py) and Go port (internal/hooks/hooks.go) byte-identically

---
