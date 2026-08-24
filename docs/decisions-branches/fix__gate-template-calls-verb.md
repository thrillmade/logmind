← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-07-make-the-ci-gate-call-the-verb-it-claimed-to-mirror-and-chec -->
- **2026-08-07** — Make the CI gate call the verb it claimed to mirror, and check out the base ref
<!-- logmind-entry-end -->

## 2026-08-07 17:39 - Make the CI gate call the verb it claimed to mirror, and check out the base ref

**Reasoning:** The template's own header said it 'mirrors the local logmind check-decisions pre-commit hook'. It did not mirror it — it reimplemented it in bash, and the copy had drifted five ways: *.md wholesale in the exclusion case (#260), a live-PR-title [skip-logmind] read (#278), a path-match decision test instead of §3.1 shape (#278), a hardcoded THRESHOLD 20 that never read commit_line_threshold (#284), and a second exclusion list that §3.4 says will inevitably disagree with the first. The gate now invokes 'logmind check-decisions --base <base> --head <head>' and the bash decides nothing, so all five die at once rather than needing five patches and inviting a sixth drift.

**Alternatives considered:** Patch the five defects in the bash and keep the reimplementation. Rejected: that leaves two encodings of one rule, which is the defect §3.4 names outright — 'Two lists that mean the same thing are two lists that will disagree.'

**Implications:**
- The workspace is the BASE ref, never the merge ref, and that is load-bearing rather than incidental: the verb reads git.commit_line_threshold from the checkout, so a merge-ref workspace would let a pull request raise its own threshold in the diff under judgement. Proven live in a scratch repo — a PR setting commit_line_threshold 10000 PASSES from a merge-ref workspace and FAILS from a base-ref one. That is SPEC §6.3, a gate is never satisfiable by the change it judges, and without the base-ref checkout the config threshold added in #287 would itself have been a new self-service escape. permissions drops to contents:read alone, since deleting the PR-title read removed the only consumer of pull-requests:read. logmind's own installed workflow is deliberately NOT regenerated here: setup-logmind installs the latest RELEASE, and --base/--head exists only on the unmerged parent, so regenerating now would red logmind's own gate on every PR including this one. The same ordering binds consumers — the fleet cannot take this template until a release carries the verb.

---

