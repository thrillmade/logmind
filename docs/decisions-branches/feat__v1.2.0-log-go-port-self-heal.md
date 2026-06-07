← back to [docs/timeline.md](../timeline.md)

## 2026-06-07 15:07 - v1.2.0: Port logmind log to Go and add 3-layer markdown self-healing

**Reasoning:** Closes Phase B3 catch-up + plan §8.7. The Python shim has carried the log surface through v1.0 and v1.1 — this PR finally lands the native Go port AND folds in Layer 1 of self-heal (linkcheck-driven interactive retry loop) so the heuristic is built in from day one rather than bolted on later. Layer 3 (CI workflow template v4→v5) lands in the same PR for the same reason — dual-mode (Anthropic auto-fix when ANTHROPIC_API_KEY is set; deterministic PR comment otherwise) means EVERY logmind user gets actionable feedback on link issues, with or without API tokens. Bidirectional linking (branch decision files now carry a back-link header to docs/timeline.md) closes the round-trip the timeline already had one direction of.

**Alternatives considered:** Defer port to v1.3 — rejected: every release that ships the Python shim drift-tests against Go binaries that don't have parity yet, Skip Layer 3 dual-mode and just ship the mode-A auto-fix — rejected: orgs without ANTHROPIC_API_KEY are the common case; zero-config is the design point, Layer 3 in a separate follow-up — rejected: shipping Layer 1 alone leaves the CI gate red on missed self-heals; better to ship the safety net in the same wave

**Implications:**
- internal/cli/log.go: new file. cobra command for logmind log; resolves target file via branch_aware (default branch → docs/decisions.md; feature branch → docs/decisions-branches/<sanitized>.md); writes backlink header on first creation; auto-commits (stage=all default per memory note) via gitcli; runs Layer 1 self-heal.
- internal/linkcheck/linkcheck.go: extended with CheckReport struct + CheckWithReport function. Each Finding carries Path/Reason/SuggestedFix; heuristic suggestion mapping covers timeline-regen / file-structure-regen / generic / well-known orphans / nearest-README orphans. JSON-tagged for the v5 mode-B comment shape.
- internal/cli/check_links.go: --json flag emits CheckReport as indented JSON. Used by v5 workflow template's mode-B step.
- internal/templates/github/check-doc-links.yml.template: v4→v5. Dual-mode self-heal job triggers on pull_request when check-links fails; mode A invokes Claude to propose link-fix diff + commits via GITHUB_TOKEN; mode B posts deterministic PR comment via gh pr comment.
- internal/templates/decisions-branch-header.md.template: NEW. Single-line ← back to [docs/timeline.md](../timeline.md) + blank line. POSIX line endings.
- internal/gitcli/gitcli.go: added AddAll() + Commit() helpers (mirror Python git_handler.git_add_all + git_commit).
- internal/version/version.go: 1.1.0-dev → 1.2.0-dev. Golden files bumped (testdata/version.golden, hooks/post-{merge,rewrite}.golden).
- Tests: 21 new (13 logmind log specs covering routing/backlink/retry-loop/TTY/auto-commit; 6 CheckReport specs covering suggestion heuristics; 2 template specs covering v5 dual-mode shape + backlink header bytes). Full suite: 398 PASS + 5 SKIP (was 377 + 5 on main). go test ./... -race -count=1 green.
- TTY detection uses pure stdlib (os.Stdin.Stat() + ModeCharDevice check); avoids golang.org/x/term dependency that would bump the go-version requirement. Indirected via isTerminalFunc test seam so each test pins the interactive/non-interactive path explicitly.
- Out of scope for v1.2.0 (carried by the Python shim until follow-up): decisions-archive rotation when count > max_recent; --template flag for pre-filled entries; --no-push (v1.2.0 commits but does not push). All three noted in log.go file-level docstring.

---

## 2026-06-07 15:08 - Regen derived docs after v1.2.0 decision log

**Reasoning:** After the v1.2.0 decision was logged via the new Go binary, docs/timeline.md and docs/file-structure.md need refreshing so they reflect the new branch decision file. logmind log doesn't yet auto-regen on the Go path (deferred to a follow-up — Python carries this in the v0.6.16 shim's auto_update_file_structure + write_timeline calls); do it explicitly here.

**Alternatives considered:** Wire auto-regen into the Go log command in this PR — rejected: out-of-scope, would balloon the diff. Tracked as a follow-up.

**Implications:**
- Two additional commits land on the branch (the regen) — the timeline test won't blink because the merge driver auto-resolves these on main.

---

