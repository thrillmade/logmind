## 2026-06-02 00:41 - feat(go-b2): port install-hook, check-decisions, check-links + git/hook helpers

**Reasoning:** B2 is the foundation layer for B3 (derived doc generators) — they call into hook bodies + .gitattributes block management. Splitting it across four narrow packages (gitcli, hooks, gitattr, linkcheck) keeps the dependency graph DAG-shaped: cli depends on the leaf packages, leaf packages depend on each other only via gitcli. This shape mirrors the Python module split (core/git_handler.py, core/gitattributes.py, actions/link_check.py) and makes byte-identical-vs-Python parity tests focused: each Go package has a parity gate against its Python counterpart, independent of the others.

**Alternatives considered:** Single internal/gitops/ package with all helpers — rejected. Mixing hook builders (pure functions, no I/O) with git subprocess wrappers (heavy on os/exec) made the test surface noisy; the parity tests for hook bodies don't need git on PATH, so they should NOT skip when git is missing., Embedding hook bodies via text/template — rejected. Python uses raw string literals; any template layer introduces whitespace risk and breaks the bytes-equal-to-Python contract. The Go side concatenates string literals directly so a diff against python3 -c 'print(_build_post_merge_hook_body(), end="")' is exact modulo the version-marker line., Port the check-links config-file override now — deferred to B3. The .logmind/config.yml loader hasn't landed in Go yet (it's part of B3's init/log work); without it, the Go check-links uses default roots + default allowlist. Most projects don't override, so parity holds for the dominant case.

**Implications:**
- Hook-body parity is enforced by TWO complementary tests: (1) golden file in internal/hooks/testdata captures the Go shape and trips on whitespace drift; (2) TestPostMergeBody_ByteIdenticalToPython shells to Python, extracts the body, normalises the version-marker line (Go=1.0.0-dev vs Python=0.6.14), and asserts byte-equality. CI without Python skips (2) gracefully; (1) keeps the contract honest.
- errSilentExit1 sentinel pattern preserves Python's sys.exit(1)-after-print byte layout. Subcommand RunE writes the user-facing message via fmt.Fprintln(stdout) then returns errSilentExit1; cobra (with SilenceErrors:true on the root) doesn't re-print, and cmd/logmind/main.go calls os.Exit(1) on any non-nil Execute() return. Result: stdout matches Python byte-for-byte, stderr stays empty on these paths.
- internal/gitcli/ wraps EVERY git invocation the Go binary makes. Future waves (B3+) MUST add new git operations as named functions there rather than calling exec.Command("git", ...) inline. This keeps subprocess error handling consistent (GitError wrapper, ErrGitNotFound sentinel, ConfigGet's (string, bool) tri-state).

---
## 2026-06-02 00:54 - fix(go-b2): main.go prints real errors; rename errSilent → ErrSilent exported (clud-bug PR #117 thread)

**Reasoning:** clud-bug-review found a silent-error bug: B1's main.go relied on cobra's default error printing, but B2 set SilenceErrors: true (to avoid duplicate stdout/stderr output for already-printed user errors). Net effect: every non-Silent error (real OS failures like mkdir denied, chmod failed) went silent — user got no diagnostic, just exit 1. Fix: main.go checks errors.Is(err, cli.ErrSilent); silent errors exit quietly, anything else prints 'Error: <msg>' to stderr. Renamed the existing unexported errSilentExit1 sentinel to exported cli.ErrSilent so main can reference it via the package boundary. Tests + build green.

**Alternatives considered:** Restore SilenceErrors: false (the B1 design) — would duplicate user-facing messages on stderr for expected failure paths, Define a separate SilentError type wrapping the cause — richer but introduces two sentinels (B2's errSilentExit1 + new type) that mean the same thing, Have every RunE explicitly print errors itself — works but pushes the concern into every command handler

**Implications:**
- Future waves: real OS errors should be returned raw (not wrapped in ErrSilent); already-printed control-flow errors return ErrSilent. main + cobra do the rest
- Establishes the package boundary: cli.ErrSilent is now part of the cli package's public surface (within internal/)

---
