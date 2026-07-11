← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-11-reconcile-logmind-log-stdout-to-spec-3-1-three-line-contract -->
- **2026-07-11** — Reconcile logmind log stdout to SPEC §3.1 three-line contract + add --no-push and a real gitcli.Push (salvaged from retired PR #154)
<!-- logmind-entry-end -->

## 2026-07-11 14:27 - Reconcile logmind log stdout to SPEC §3.1 three-line contract + add --no-push and a real gitcli.Push (salvaged from retired PR #154)

**Reasoning:** SPEC §3.1 mandates byte-exact stdout: an info stage-notice, then Logged decision with the quoted summary, then a Committed[-and-pushed] line; config.yml already documented git.auto_push default true but log.go never implemented push at all. PR #154 sketched this but used raw fmt.Fprintln instead of the v2 quiet-aware q.chat API and its line-3 wording (Committed changes (push disabled)) doesn't match the SPEC's literal Committed changes text, so it was reconciled to the SPEC verbatim rather than ported as-is.

**Alternatives considered:** Port PR #154 unchanged onto q.chat -- rejected, its line-3 suffix and no --stage-scoped notice drift from the current SPEC text, Leave push out of scope again -- rejected, config.yml already promises git.auto_push and nothing implemented it, a real gap

**Implications:**
- New gitcli.Push wraps git push, gated by shouldPush = shouldCommit && cfg.Git.AutoPush && not noPush; push failure (e.g. no upstream) is non-fatal and falls back to the Committed changes line
- Stage-notice wording for --stage scoped has no SPEC example, so it is this port's own text (Staging decision file only) -- flagged for review
- Known pre-existing gap left unfixed: in the TTY-interactive branch-file path, nudgeBranchSummary still prints between required lines 2 and 3, violating SPEC 3.1.1's line-3-must-be-third rule; fixing it would break the nudge's real-time prompt UX or decouple the edit from landing in the same commit, so it needs a separate product call
- internal/timeline/merge_driver_test.go already pre-patched auto_push: false in its test fixture anticipating this wiring; go test ./... and the integration-tagged merge-driver suite both pass unchanged

---

