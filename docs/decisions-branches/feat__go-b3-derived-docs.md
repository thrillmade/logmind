## 2026-06-02 01:35 - feat(go-b3): port timeline, file-structure, tree, rebase + internal config/decisions/timeline/tree packages with byte-identical Python v0.6.14 output

**Reasoning:** B3 ships the foundation B4-B6 will read. Brief-mode month grouping is the highest-complexity bit: assemble []string mirror of Python's lines + strings.Join('\\n') instead of trying to simulate join semantics at the byte level. tree walker is pure Go (no subprocess to system tree(1)) so consuming repos get identical output across OSes. config loader uses gopkg.in/yaml.v3's typed-struct merge so user keys overlay defaults leaf-by-leaf. --check-without-write exits 1 in Go vs Python's exit 2 — known divergence, stdout byte-identical, cmd/logmind/main.go would need an ErrSilentExit2 sentinel to bridge (deferred to coordinated change).

**Alternatives considered:** Custom python-style join helper (rejected — Go's strings.Join is sufficient and clearer), Embed Python's tree.py output verbatim via go-embed (rejected — would defeat the purpose of pure-Go binary), Skip the rebase port for B3 (rejected — three-step wrapper is small + tightly coupled to gitcli I had to extend anyway)

**Implications:**
- B4 agents/skill, B5 doctor/init, B6 self-update will all read from internal/timeline + internal/tree + internal/decisions; their stdout byte-identity gate inherits B3's parity guarantee.
- Once cmd/logmind/main.go gains an exit-code sentinel (coordinated with whoever lands B4 first), the --check-without-write divergence closes.
- Makefile SNAPSHOT_PKGS now includes internal/timeline + internal/tree; make snapshot regenerates 7 golden files.

---
## 2026-06-02 01:47 - fix(go-b3): rebase against B4 + follow symlinks in sort (matches Python pathlib.is_dir) — clud-bug PR #119

**Reasoning:** Two fixes for PR #119: (1) Rebase against v1-go-rewrite after B4 merged — conflicts in internal/cli/root.go (two waves adding subcommand registrations) + docs/timeline.md + docs/file-structure.md (derived; took B4's version then regenerated). (2) Tree sort divergence flagged by clud-bug-review: Python's pathlib.Path.is_dir() follows symlinks by default; Go's os.DirEntry.IsDir() uses lstat semantics and returns false for symlinks-to-directories. Result: any unignored symlink-to-dir sorted with files in Go but with directories in Python → non-byte-identical output. Added isDirFollow helper that follows symlinks (Stat instead of lstat) and uses it in the sort comparator. Walk-time loop guard (don't recurse into symlinks) still applies — matches Python's 'if item.is_dir() and not item.is_symlink()'.

**Alternatives considered:** Skip symlinks entirely in the walk — would diverge from Python (Python lists them, just doesn't recurse), Document the divergence — fails the byte-identical contract, Use Type() bit-mask instead of Stat — Type() reflects ENTRY type (always symlink), not target type

**Implications:**
- Future repos with symlinks-to-dirs (rare but possible for monorepo-style code organization) get correct output
- Performance: one extra Stat() per symlink encountered during sort; negligible because symlinks are rare and the sort runs once per directory

---
