## 2026-06-02 00:09 - feat(go-b1): scaffold Go module + cobra + version + snapshot infra

**Reasoning:** Wave B1.scaffold of the Python->Go rewrite. Three independent design calls baked in here that the rest of the rewrite inherits. (1) CLI library: cobra over urfave/cli/v2 or stdlib flag. Cobra is the Go ecosystem default (kubectl, gh, hugo, helm) so future contributors recognise the patterns immediately, and its subcommand tree mirrors Python click's @main.group/@main.command shape almost 1:1 — cheaper port path. urfave/cli would also work but its DSL diverges from click further and would force agents to learn a third style. stdlib flag scales badly past 2-3 subcommands. (2) Layout: cmd/logmind/main.go + internal/{cli,version}/. cmd/ for the binary entry point per the standard Go project layout convention; internal/ rather than pkg/ because the wave 1.0 surface is the CLI binary itself — no other repo should import logmind's guts as a library yet, and internal/ enforces that at compile time. We can promote individual packages to pkg/ later if/when the protocol contract stabilises a public Go API. (3) Snapshot golden format: plain-text files under <pkg>/testdata/<name>.golden plus a shared -update flag, byte-identical exec-output comparison. No diff framework, no JSON wrapping — just raw stdout bytes. This makes the verify-parity gate against Python v0.6.14 (later wave) a one-line diff: every Python command's stdout becomes the golden for the matching Go command. The protocol-contract --version format ('logmind <ver> (spec <spec>)') is intentionally NEW shape, not byte-identical to Python's click default 'logmind, version 0.6.14' — v1.0 publishes a stable machine-parseable line that includes the spec version so downstream tooling (clud-bug, tokenomics) can detect protocol skew without parsing the binary version separately.

**Alternatives considered:** urfave/cli/v2 (more configurable, ecosystem split with cobra; rejected for ecosystem familiarity), stdlib flag only (zero deps, but scales poorly past 2-3 subcommands and no nested group support), pkg/ instead of internal/ (would publish a public Go API; premature before SPEC.md locks), Keep Python click --version line format byte-identical instead of the (spec ...) shape (rejected: v1.0 is a clean break and the spec segment is a protocol contract requirement), Snapshot framework like go-cmp/cmpopts or testify/snapshot (rejected: plain-text goldens are easier to diff, audit in PRs, and reuse for cross-language parity)

**Implications:**
- Every subsequent wave registers its subcommand in internal/cli/ and adds a testdata/<cmd>.golden — pattern is fixed by B1.
- make verify-parity is wired as a stable target but currently a placeholder; the wave that ports  is the first one that fills it in.
- go-test.yml only gates the v1-go-rewrite branch + its feature branches; existing test.yml (pytest) keeps gating main until the cutover PR.
- Go 1.22 toolchain floor — picked to match the GitHub-hosted runner default at time of writing; bump deliberately if a stdlib feature requires it.

---
## 2026-06-02 00:19 - fix(go-b1): resolve clud-bug-review threads (duplicate error output, fragile snapshot flag)

**Reasoning:** clud-bug-review on PR #116 flagged 2 legitimate Go issues. (1) main.go printed err to stderr but cobra default SilenceErrors=false ALREADY does this via PrintErrln — every error case had duplicate stderr output. Removed the duplicate fmt.Fprintln; main() now just sets exit code. (2) Makefile snapshot target ran 'go test $(PKG) -update' across ALL of ./cmd/... ./internal/... — but -update is a custom test flag only registered by snapshot tests; any future package without registration would fail with 'flag provided but not defined: -update' (Go exits 2 on unrecognised flags). Scoped snapshot target to SNAPSHOT_PKGS := ./internal/cli/...; comment explains future waves must explicitly opt in when adding new testdata/.

**Alternatives considered:** Resolve threads without fixing — would violate dogfood discipline on the very first wave of the rewrite, Set SilenceErrors: true on root + keep explicit fmt.Fprintln in main.go — less clean than letting cobra handle errors uniformly, Make snapshot use 'go test ./... -update -run ^TestSnapshot' — fragile naming convention

**Implications:**
- Wave B1 pattern: scope -update to packages with testdata. B2+B7 must update SNAPSHOT_PKGS when adding new golden files
- Reinforces dogfood mantra: clud-bug-review findings address resolved with code + tests, not thread-resolution; even on the first wave

---
