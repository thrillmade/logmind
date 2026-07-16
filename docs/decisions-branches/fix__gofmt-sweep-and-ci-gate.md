← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-07-03-hygiene-gofmt-sweep-gofmt-vet-ci-gate-surface-doctor-fix-bac -->
- **2026-07-03** — hygiene: gofmt sweep + gofmt/vet CI gate + surface doctor --fix backfill write errors
<!-- logmind-entry-end -->

## 2026-07-03 12:21 - hygiene: gofmt sweep + gofmt/vet CI gate + surface doctor --fix backfill write errors

**Reasoning:** CI had no gofmt/vet gate, so 8 Go files silently accumulated formatting drift. Separately, doctor --fix backfill dropped writeAtomic errors, making a partial --fix indistinguishable from a clean no-op.

**Alternatives considered:** golangci-lint (heavier; gofmt+vet is the minimal stdlib gate), os.Stderr directly in backfillBranchSummaries (rejected: the codebase threads writers for testability)

**Implications:**
- gofmt -w'd 8 files (pure whitespace, test-neutral); added a lint job wired into the required 'test' aggregator (needs: [go, lint]) so drift can't recur; backfillBranchSummaries now takes a stderr writer and prints a note per unwritable file (threaded via cmd.ErrOrStderr).

---

