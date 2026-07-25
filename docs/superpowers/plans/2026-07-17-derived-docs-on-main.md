# Derived-Docs-on-Main / Clean Timeline — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a merge conflict on `docs/timeline.md` and `docs/file-structure.md` structurally impossible — a branch never edits them, so git's own 3-way merge is conflict-free — while keeping every agent's context fresh via `logmind warp`, a cold-start `context` refresh, and a pulse nudge.

**Architecture:** The two derived docs are *purely* generated (`timeline.md` = f(decision files); `file-structure.md` = f(repo tree)), so a branch-side edit carries no unique information and can always be discarded losslessly. We enforce one invariant — **on any non-default branch, both derived docs stay byte-identical to their merge-base-with-main version** — with three layers: **L0** git hooks regenerate on the default branch only; **L1** `logmind log` restores the two docs to their committed (HEAD) content before staging on a non-default branch; **L3** the CI `check-derived-docs` gate BLOCKS any PR that modified a derived doc, and a new main-only job regenerates + commits them post-merge. Freshness rides on top: `logmind warp` (fetch + read-only refresh from main), `logmind context` prefers the last-fetched `origin/main` copy, and a third pulse probe nudges "run `logmind warp`".

**Tech Stack:** Go 1.22, cobra CLI, `internal/gitcli` (thin `git` subprocess wrapper), GitHub Actions (`gh` CLI on the runner), the protocol SPEC (separate `thrillmade/protocol` repo).

## Global Constraints

- **Byte-parity with frozen Python v0.6.16** for `logmind log` stdout (the SPEC §3.1 three-line contract) and for derived-doc *generation* output. Do not alter `buildDecisionEntry`, `timeline.Generate`, or `tree.GenerateFileStructure` output bytes.
- **`logmind log` hot path stays network-free.** No `git fetch`, no `gh`, no HTTP anywhere reachable from `runLog` / `emitPulse`. (Network lives only in `logmind warp`, an explicit command.)
- **Hook bodies** are golden-fixture-pinned (`internal/hooks/hooks_test.go`). v2.0.0 is allowed to evolve them (the embedded `# logmind-hook-version:` line already differs from Python), but every body change MUST regenerate the golden fixture in the same task.
- **The zero-conflict invariant is rule #1.** No task may introduce a code path that commits a branch-local edit to `docs/timeline.md` or `docs/file-structure.md`.
- **Keep the CI job name `check-derived-docs`** exactly (branch-protection rulesets match on it).
- **The two CI files change in lockstep:** `.github/workflows/regen-timeline.yml` (self-repo, `make build`) and `internal/templates/github/regen-timeline.yml.template` (fleet, `setup-logmind`). Bump the template marker `# logmind-template-version: v7` → `v8`.
- **Commit via `logmind log`**, not raw git, for every substantive task. No backticks inside `logmind log` arguments.
- **The two derived-doc paths are a single source of truth:** `docs/timeline.md`, `docs/file-structure.md`. Introduced once as `derivedDocPaths` (Task 2) and reused everywhere.

---

## File Structure

| File | Responsibility | Tasks |
|---|---|---|
| `internal/gitcli/gitcli.go` | New helpers: `MergeBase`, `Fetch`, `ShowFile`, `RestorePathsToHead` | 1 |
| `internal/cli/derived.go` (new) | `derivedDocPaths` + `onNonDefaultBranch` — shared by log/warp/pulse/context | 2 |
| `internal/cli/log.go` | L1: restore derived docs to HEAD before staging on a non-default branch | 3 |
| `internal/hooks/hooks.go` (+ `hooks_test.go`) | L0: gate post-merge + post-rewrite regen to the default branch | 4 |
| `internal/cli/warp.go` (new) + `internal/cli/root.go` | `logmind warp` command + registration | 5 |
| `internal/cli/pulse.go` | 3rd probe: `mainDecisionsPulseLine` | 6 |
| `internal/cli/context.go` | Cold-start: prefer last-fetched `origin/main` copy of derived docs | 7 |
| `.github/workflows/regen-timeline.yml` + `internal/templates/github/regen-timeline.yml.template` | L3: blocking PR gate + main-only regen job | 8 |
| `internal/cli/*_test.go`, a merge-cleanliness integration test | Headline zero-conflict proof + per-layer tests | 9 |
| protocol `SPEC.md` §0.4 (cross-repo) | Sync-invariant reword | 10 |
| agent-skills logmind skill (cross-repo) | "Staying current" note | 10 |

---

## Task 1: gitcli helpers (MergeBase, Fetch, ShowFile, RestorePathsToHead)

**Files:**
- Modify: `internal/gitcli/gitcli.go` (append near `RunCaptured` at :551)
- Test: `internal/gitcli/gitcli_test.go`

**Interfaces — Produces:**
- `func MergeBase(repoRoot, ref string) (string, bool)` — `git merge-base <ref> HEAD`; `("",false)` on any error.
- `func Fetch(repoRoot, remote, ref string) error` — `git fetch <remote> <ref>` (network; explicit-command use only).
- `func ShowFile(repoRoot, ref, path string) (string, bool)` — `git show <ref>:<path>`; `("",false)` if absent.
- `func RestorePathsToHead(repoRoot string, paths ...string) error` — `git checkout HEAD -- <path>` per path, best-effort.

- [ ] **Step 1: Write the failing test** (`gitcli_test.go`) — build a temp repo with a committed `a.txt`, branch, edit `a.txt` on the branch, assert `RestorePathsToHead` reverts it and `ShowFile(main-sha, "a.txt")` returns the original bytes; assert `MergeBase(mainSha) != ""`.

```go
func TestRestorePathsToHead_RevertsBranchEdit(t *testing.T) {
	repo := initTempRepo(t) // helper: git init + initial commit of a.txt="v1"
	writeFile(t, repo, "a.txt", "v2-edited")
	if err := RestorePathsToHead(repo, "a.txt"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := readFile(t, repo, "a.txt"); got != "v1" {
		t.Fatalf("want restored v1, got %q", got)
	}
}

func TestShowFile_ReadsRefContent(t *testing.T) {
	repo := initTempRepo(t)
	got, ok := ShowFile(repo, "HEAD", "a.txt")
	if !ok || got != "v1" {
		t.Fatalf("ShowFile HEAD:a.txt = %q,%v want v1,true", got, ok)
	}
	if _, ok := ShowFile(repo, "HEAD", "missing.txt"); ok {
		t.Fatal("ShowFile of missing path should be false")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/gitcli/ -run 'RestorePathsToHead|ShowFile' -v` → FAIL (undefined).
- [ ] **Step 3: Implement** (append to `gitcli.go`):

```go
// MergeBase returns the merge-base commit SHA of ref and HEAD. Best-effort:
// ("", false) on any error (ref missing, no common ancestor, not a repo).
func MergeBase(repoRoot, ref string) (string, bool) {
	out, _, err := RunCaptured(repoRoot, "merge-base", ref, "HEAD")
	if err != nil {
		return "", false
	}
	sha := strings.TrimSpace(out)
	return sha, sha != ""
}

// Fetch runs `git fetch <remote> <ref>` (a NETWORK call). Used only by explicit
// commands (logmind warp) — never the `logmind log` hot path.
func Fetch(repoRoot, remote, ref string) error {
	_, _, err := RunCaptured(repoRoot, "fetch", remote, ref)
	return err
}

// ShowFile returns the content of path at ref (`git show <ref>:<path>`).
// ("", false) if the path does not exist at ref or on any error.
func ShowFile(repoRoot, ref, path string) (string, bool) {
	out, _, err := RunCaptured(repoRoot, "show", ref+":"+path)
	if err != nil {
		return "", false
	}
	return out, true
}

// RestorePathsToHead restores each path to its committed (HEAD) content in BOTH
// the index and the working tree (`git checkout HEAD -- <path>`), discarding any
// staged or unstaged change. Per-path and best-effort: a path untracked at HEAD
// errors for that path only and is skipped; the first error (if any) is returned
// for logging but callers generally ignore it (the derived docs are purely
// generated, so a failed restore just leaves the pre-existing state).
func RestorePathsToHead(repoRoot string, paths ...string) error {
	var firstErr error
	for _, p := range paths {
		if _, _, err := RunCaptured(repoRoot, "checkout", "HEAD", "--", p); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
```

- [ ] **Step 4: Run to verify pass** — `go test ./internal/gitcli/ -run 'RestorePathsToHead|ShowFile|MergeBase' -v` → PASS. Confirm `strings` already imported (it is).
- [ ] **Step 5: Commit** — `logmind log "Add gitcli MergeBase/Fetch/ShowFile/RestorePathsToHead helpers" -r "Foundation for the derived-docs-on-main invariant: L1 restore, warp refresh, and the pulse main-compare all need merge-base, ref-file reads, and HEAD restore" -i "warp is the only network caller of Fetch; the hot path never touches it"`

---

## Task 2: shared derived-doc constants + branch predicate

**Files:**
- Create: `internal/cli/derived.go`
- Test: `internal/cli/derived_test.go`

**Interfaces — Produces:**
- `var derivedDocPaths = []string{"docs/timeline.md", "docs/file-structure.md"}`
- `func onNonDefaultBranch(cwd string) bool`

- [ ] **Step 1: Write the failing test:**

```go
func TestOnNonDefaultBranch(t *testing.T) {
	repo := initTempRepo(t) // on default branch "main" after initial commit
	if onNonDefaultBranch(repo) {
		t.Fatal("default branch should be false")
	}
	runGit(t, repo, "checkout", "-b", "feat/x")
	if !onNonDefaultBranch(repo) {
		t.Fatal("feature branch should be true")
	}
}
```

- [ ] **Step 2: Verify fails** — `go test ./internal/cli/ -run TestOnNonDefaultBranch -v` → FAIL.
- [ ] **Step 3: Implement** (`derived.go`):

```go
package cli

import "github.com/thrillmade/logmind/internal/gitcli"

// derivedDocPaths are the two committed, purely-derived context docs governed by
// the zero-conflict invariant: on any non-default branch they MUST stay
// byte-identical to their merge-base-with-main version (the branch never edits
// them), so git's 3-way merge is conflict-free. They are regenerated only at the
// integration point (main). Repo-relative, forward-slash (git pathspec form).
var derivedDocPaths = []string{"docs/timeline.md", "docs/file-structure.md"}

// onNonDefaultBranch reports whether cwd is a git repo currently on a branch
// other than the default branch. Best-effort: false on a non-repo, detached
// HEAD, unborn branch, or unknown default — the conservative answer, since every
// caller uses `true` to ENABLE the extra invariant guard and `false` preserves
// pre-v2.0.0 behavior.
func onNonDefaultBranch(cwd string) bool {
	if !gitcli.IsRepo(cwd) {
		return false
	}
	cur := gitcli.CurrentBranch(cwd)
	if cur == "" {
		return false
	}
	def := gitcli.DefaultBranch(cwd)
	if def == "" {
		return false
	}
	return cur != def
}
```

- [ ] **Step 4: Verify pass** — `go test ./internal/cli/ -run TestOnNonDefaultBranch -v` → PASS.
- [ ] **Step 5: Commit** — `logmind log "Add shared derivedDocPaths + onNonDefaultBranch predicate" -r "Single source of truth for the two governed docs and the branch test, reused by L1 log/warp/pulse/context so the invariant boundary is defined in exactly one place"`

---

## Task 3: L1 — `logmind log` restores derived docs to HEAD on a non-default branch

**Files:**
- Modify: `internal/cli/log.go` — `commitDecision` (:877-898)
- Test: `internal/cli/log_test.go`

**Interfaces — Consumes:** `gitcli.RestorePathsToHead` (Task 1), `derivedDocPaths` + `onNonDefaultBranch` (Task 2).

- [ ] **Step 1: Write the failing test** — on a feature branch, dirty `docs/timeline.md`, run `logmind log`, assert the resulting commit does NOT include a change to `docs/timeline.md` and the working tree copy matches HEAD.

```go
func TestLog_DoesNotCommitDirtiedDerivedDocOnBranch(t *testing.T) {
	repo := initRepoWithLogmind(t)        // docs/ scaffolded, decisions.md committed, timeline.md committed
	runGit(t, repo, "checkout", "-b", "feat/y")
	writeFile(t, repo, "docs/timeline.md", "STALE BRANCH EDIT\n") // simulate a stray hook/manual regen
	if err := runLog(repo, "Decide something", &logFlags{stage: "all"}, false, strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatalf("runLog: %v", err)
	}
	// The commit must not carry the derived-doc edit.
	names, _, _ := gitcli.RunCaptured(repo, "show", "--name-only", "--format=", "HEAD")
	if strings.Contains(names, "docs/timeline.md") {
		t.Fatal("commit must NOT include docs/timeline.md on a branch (invariant)")
	}
	// And the working tree copy is back to committed content.
	head, _ := gitcli.ShowFile(repo, "HEAD", "docs/timeline.md")
	if readFile(t, repo, "docs/timeline.md") != head {
		t.Fatal("working-tree derived doc must be restored to HEAD")
	}
}
```

- [ ] **Step 2: Verify fails** — `go test ./internal/cli/ -run TestLog_DoesNotCommitDirtiedDerivedDocOnBranch -v` → FAIL (the edit is currently swept by `git add -A`).
- [ ] **Step 3: Implement** — at the TOP of `commitDecision`, before the stage switch (log.go:878):

```go
	// Zero-conflict invariant (v2.0.0): docs/timeline.md + docs/file-structure.md
	// are purely-derived, main-only artifacts. On a non-default branch, restore
	// them to their committed (HEAD) content BEFORE staging so neither a stray
	// hook regen nor `git add -A` can sweep a branch-local edit into this commit —
	// which would diverge the branch from main and cause a future merge conflict.
	// Lossless: they regenerate deterministically from the decision files (which
	// ARE committed, per-branch, and never conflict). On the default branch this
	// is intentionally skipped — main is where the derived docs SHOULD be current.
	if onNonDefaultBranch(cwd) {
		_ = gitcli.RestorePathsToHead(cwd, derivedDocPaths...)
	}
```

- [ ] **Step 4: Verify pass** — `go test ./internal/cli/ -run TestLog -v` → PASS (new test green, existing log tests unaffected — default-branch tests skip the restore). Then `go test ./internal/cli/` full-package green.
- [ ] **Step 5: Confirm hot path unchanged** — grep `runLog`/`commitDecision` for any `Fetch`/network; there is none. `git checkout HEAD -- <path>` is local. Run `go test ./internal/cli/ -run Pulse -race` to confirm no race regressions.
- [ ] **Step 6: Commit** — `logmind log "L1: logmind log restores derived docs to HEAD before staging on a non-default branch" -r "The primary defense of the zero-conflict invariant on the commit path: even if a hook or manual regen dirties timeline.md/file-structure.md, the branch commit never carries the divergent copy; auto-heal is lossless because the docs are pure functions of the committed decision files" -a "Exclude the two paths from git add via pathspec (rejected: leaves a dirty working tree a later raw git add could catch)" -i "On the default branch this is a no-op so main stays current; only branches are constrained"`

---

## Task 4: L0 — gate post-merge + post-rewrite hook regen to the default branch

**Files:**
- Modify: `internal/hooks/hooks.go` — `BuildPostMergeBody` (:102-178), `BuildPostRewriteBody` (:183-213)
- Modify: `internal/hooks/hooks_test.go` — regenerate golden fixtures

**Context:** post-merge already skips a fast-forward pull-up on the default branch but REGENERATES on feature branches; post-rewrite regenerates AND `git add`s unconditionally (hooks.go:210). Both leave (or stage) a branch-local derived-doc edit → invariant violation. Gate both to `current == default`.

- [ ] **Step 1: Write the failing test** — assert the post-merge body contains an early `exit 0` when `current != default`, and the post-rewrite body regenerates only under a default-branch guard. (Behavioral fixture: install the hooks into a temp repo on a feature branch, run them, assert `docs/timeline.md` is NOT modified/staged.)

```go
func TestPostRewriteHook_NoRegenOnFeatureBranch(t *testing.T) {
	repo := initRepoWithLogmind(t)
	installHooks(t, repo) // writes .git/hooks/post-rewrite from BuildPostRewriteBody
	runGit(t, repo, "checkout", "-b", "feat/z")
	// Add a decision so a regen WOULD change timeline.md, then amend to fire post-rewrite.
	appendFile(t, repo, "docs/decisions-branches/feat__z.md", "## 2026-07-17 10:00 - x\n\n---\n\n")
	runGit(t, repo, "add", "-A"); runGit(t, repo, "commit", "-m", "wip")
	runGit(t, repo, "commit", "--amend", "--no-edit") // fires post-rewrite
	if isStagedOrDirty(t, repo, "docs/timeline.md") {
		t.Fatal("post-rewrite must NOT regen/stage derived docs on a feature branch")
	}
}
```

- [ ] **Step 2: Verify fails** — `go test ./internal/hooks/ -run PostRewriteHook_NoRegen -v` → FAIL.
- [ ] **Step 3: Implement** — in `BuildPostMergeBody`, after the `current`/`default` computation (hooks.go:160-165), replace the "skip only on fast-forward" block so a **non-default branch exits 0 outright**:

```sh
  if [ -n "$current" ] && [ "$current" != "$default" ]; then
    # v2.0.0 derived-docs-on-main: never regenerate on a non-default branch —
    # the branch must keep the derived docs byte-identical to its main
    # merge-base (the zero-conflict invariant). main regenerates post-merge.
    exit 0
  fi
  if [ -n "$current" ] && [ "$current" = "$default" ]; then
    head_sha=$(git rev-parse HEAD 2>/dev/null || true)
    origin_sha=$(git rev-parse "origin/$default" 2>/dev/null || true)
    if [ -n "$origin_sha" ] && [ "$head_sha" = "$origin_sha" ]; then
      exit 0
    fi
  fi
```

In `BuildPostRewriteBody`, wrap the regen+add in the same default-branch guard (add `current`/`default` detection mirroring post-merge, then only regen when `current = default`):

```sh
if command -v logmind >/dev/null 2>&1 && [ -f .logmind/config.yml ]; then
  current=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)
  default=$(git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null || true)
  default=${default#origin/}
  [ -z "$default" ] && default=main
  # v2.0.0: regenerate only on the default branch (invariant: branches never
  # edit the derived docs). A rebase/amend on a feature branch leaves them as-is.
  if [ -n "$current" ] && [ "$current" = "$default" ] && [ -d docs ]; then
    logmind timeline --write docs/timeline.md >/dev/null 2>&1 || true
    logmind file-structure --write docs/file-structure.md >/dev/null 2>&1 || true
    git add docs/timeline.md docs/file-structure.md 2>/dev/null || true
  fi
fi
```

- [ ] **Step 4: Regenerate golden fixtures** — update the golden strings/files in `hooks_test.go` (the fixture that pins the exact body). Run `go test ./internal/hooks/ -run Body -v`; if it diffs, update the golden to the new body (this is the sanctioned v2.0.0 body change — verify the diff is ONLY the new guard lines, nothing else).
- [ ] **Step 5: Verify pass** — `go test ./internal/hooks/ -v` → PASS (behavioral + golden).
- [ ] **Step 6: Commit** — `logmind log "L0: post-merge + post-rewrite hooks regenerate derived docs on the default branch only" -r "A rebase/amend/merge on a feature branch previously regenerated (post-rewrite even git-added) the derived docs, diverging the branch from main; gating regen to the default branch keeps branches clean so merges never conflict" -i "Golden hook-body fixtures updated — the sanctioned v2.0.0 body evolution; the embedded hook-version line already differs from Python v0.6.16"`

---

## Task 5: `logmind warp` command

**Files:**
- Create: `internal/cli/warp.go`
- Modify: `internal/cli/root.go` (register near :83, after `newRebaseCmd`)
- Test: `internal/cli/warp_test.go`

**Interfaces — Consumes:** `gitcli.Fetch`, `gitcli.ShowFile` (Task 1), `derivedDocPaths` (Task 2), `writeAtomic` (existing). **Produces:** `newWarpCmd`, `runWarp`.

**Behavior:** fetch `origin/<default>`, then overwrite the working-tree derived docs from `origin/<default>` for READING — **never staged, never committed** (committing main's newer blobs would set branch-blob ≠ merge-base-blob and trip the Task-8 gate). Report how many decision-touching commits main is ahead.

- [ ] **Step 1: Write the failing test** — with `origin/<default>` ahead by a decision, `runWarp` overwrites `docs/timeline.md` with origin's content and leaves it UNstaged.

```go
func TestWarp_RefreshesFromOriginUncommitted(t *testing.T) {
	origin, repo := initClonePair(t)                 // repo tracks origin/main
	commitOn(t, origin, "docs/timeline.md", "MAIN-FRESH\n")
	runGit(t, repo, "fetch", "origin", "main")       // pre-fetch so ShowFile sees it
	runGit(t, repo, "checkout", "-b", "feat/w")
	if err := runWarp(repo, io.Discard, io.Discard); err != nil {
		t.Fatalf("warp: %v", err)
	}
	if readFile(t, repo, "docs/timeline.md") != "MAIN-FRESH\n" {
		t.Fatal("warp must refresh the working copy from origin/main")
	}
	if isStaged(t, repo, "docs/timeline.md") {
		t.Fatal("warp must NOT stage the refreshed docs")
	}
}
```

- [ ] **Step 2: Verify fails** — `go test ./internal/cli/ -run TestWarp -v` → FAIL.
- [ ] **Step 3: Implement** (`warp.go`):

```go
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/gitcli"
)

func newWarpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "warp",
		Short: "Refresh docs/timeline.md + docs/file-structure.md from main (read-only catch-up)",
		Long: `Fetch the latest default branch and refresh your working copy of the two
DERIVED docs (docs/timeline.md, docs/file-structure.md) so your context reflects
main's current decisions.

Read-only: warp NEVER stages or commits these files. They are regenerated on
main only; a branch must keep them byte-identical to its merge-base with main so
that merges never conflict. Your branch's own decisions live in
docs/decisions-branches/<branch>.md (committed, per-branch, conflict-free).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runWarp(cwd, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

func runWarp(cwd string, stdout, stderr io.Writer) error {
	if !gitcli.IsRepo(cwd) {
		fmt.Fprintln(stdout, "Not a git repo — nothing to warp.")
		return nil
	}
	def := gitcli.DefaultBranch(cwd)
	if def == "" {
		def = "main"
	}
	if err := gitcli.Fetch(cwd, "origin", def); err != nil {
		fmt.Fprintf(stderr, "warn: git fetch origin %s failed: %v (refreshing from last-known main)\n", def, err)
	}
	ref := "origin/" + def
	refreshed := 0
	for _, rel := range derivedDocPaths {
		content, ok := gitcli.ShowFile(cwd, ref, rel)
		if !ok {
			continue
		}
		if err := writeAtomic(filepath.Join(cwd, rel), content); err != nil {
			fmt.Fprintf(stderr, "warn: could not write %s: %v\n", rel, err)
			continue
		}
		refreshed++
	}
	ahead := ""
	if out, _, err := gitcli.RunCaptured(cwd, "rev-list", "--count", "HEAD.."+ref, "--",
		"docs/decisions.md", "docs/decisions-branches", "docs/decisions-archive.md"); err == nil {
		if n, e := strconv.Atoi(strings.TrimSpace(out)); e == nil && n > 0 {
			ahead = fmt.Sprintf(" · main is +%d decision commit(s) ahead", n)
		}
	}
	fmt.Fprintf(stdout, "ok warp: refreshed %d derived doc(s) from %s (read-only — not committed)%s\n", refreshed, ref, ahead)
	return nil
}
```

Register in `root.go` after line 83: `root.AddCommand(newWarpCmd())`.

- [ ] **Step 4: Verify pass** — `go test ./internal/cli/ -run TestWarp -v` → PASS. `go build ./cmd/logmind && ./bin/logmind warp --help` shows the command.
- [ ] **Step 5: Commit** — `logmind log "Add logmind warp: read-only refresh of derived docs from main" -r "The explicit catch-up verb — fetch origin/main and refresh the working copy of the two derived docs so an agent mid-branch sees main's current timeline, without committing them (committing would break the merge-base invariant and trip the CI gate)" -a "warp merges origin/main into the branch (rejected: creates merge commits / changes branch history for a pure context refresh)" -a "name it sync/refresh/rebase (rejected: sync and rebase are taken; warp is timeline-themed and carries no git-commit baggage)"`

---

## Task 6: 3rd pulse probe — `mainDecisionsPulseLine`

**Files:**
- Modify: `internal/cli/pulse.go` — add function + a third block in `emitPulse` (:88-93); add `strconv` import
- Test: `internal/cli/pulse_test.go`, `internal/cli/pulse_hotpath_test.go`

**Interfaces — Consumes:** `onNonDefaultBranch` (Task 2). **NETWORK-FREE** — reads the existing `origin/<default>` remote-tracking ref only (no fetch); the count reflects the last `logmind warp` / `git fetch`.

- [ ] **Step 1: Write the failing test** — on a feature branch with `origin/main` ahead by a decision commit, `emitPulse` stderr contains `main has 1 new decision commit — run 'logmind warp'`; on the default branch it does NOT.

- [ ] **Step 2: Verify fails** — `go test ./internal/cli/ -run Pulse -v` → FAIL.
- [ ] **Step 3: Implement:**

```go
// mainDecisionsPulseLine reports the "main has advanced" freshness advisory:
// on a NON-default branch, when the last-fetched origin/<default> carries
// decision-touching commits the branch does not, nudge a catch-up. NETWORK-FREE:
// reads the existing remote-tracking ref (no fetch), so the count is as of the
// last warp/fetch. Best-effort: ("", false) on any error or missing origin ref —
// this runs on every `logmind log`, so it must never fail or slow the hot path.
func mainDecisionsPulseLine(cwd string) (string, bool) {
	if !onNonDefaultBranch(cwd) {
		return "", false
	}
	def := gitcli.DefaultBranch(cwd)
	if def == "" {
		return "", false
	}
	out, _, err := gitcli.RunCaptured(cwd, "rev-list", "--count", "HEAD..origin/"+def, "--",
		"docs/decisions.md", "docs/decisions-branches", "docs/decisions-archive.md")
	if err != nil {
		return "", false
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil || n <= 0 {
		return "", false
	}
	plural := "s"
	if n == 1 {
		plural = ""
	}
	return fmt.Sprintf("logmind: main has %d new decision commit%s — run 'logmind warp' to catch up", n, plural), true
}
```

Add to `emitPulse` after the spec block (pulse.go:92):

```go
	if line, ok := mainDecisionsPulseLine(cwd); ok {
		fmt.Fprintln(stderr, line)
	}
```

Add `"strconv"` and confirm `"strings"` imports in pulse.go.

- [ ] **Step 4: Verify pass** — `go test ./internal/cli/ -run Pulse -v` → PASS. Update the pulse hot-path test to allow the third probe. Confirm no network: the probe uses `rev-list` against the LOCAL `origin/<default>` ref only.
- [ ] **Step 5: Verify hot-path budget** — `go test ./internal/cli/ -run Pulse -race`; confirm the new probe is guarded by `onNonDefaultBranch` (default-branch logs skip it entirely).
- [ ] **Step 6: Commit** — `logmind log "Third pulse probe: nudge logmind warp when main has advanced" -r "Closes the freshness loop — after a log on a stale branch, stderr surfaces how far main has moved and points at logmind warp; network-free (reads the last-fetched origin ref) to preserve the hot-path budget" -i "The count is as of the last fetch/warp; warp itself does the network fetch"`

---

## Task 7: `logmind context` prefers the last-fetched main copy of derived docs

**Files:**
- Modify: `internal/cli/context.go` — the derived-doc read path in `contextPayload` (:164-194 / the `contextDocs` loop reading via `os.ReadFile` at :181)
- Test: `internal/cli/context_test.go`

**Interfaces — Consumes:** `gitcli.ShowFile` (Task 1), `onNonDefaultBranch` (Task 2), `derivedDocPaths` (Task 2). Non-network.

**Behavior:** when on a non-default branch, render `docs/timeline.md` and `docs/file-structure.md` from `origin/<default>` (last-fetched, local — fresher than the branch's stale merge-base snapshot) if available, else the local file. The spec/repomap docs are unchanged. Never fetches (that is `warp`'s job).

- [ ] **Step 1: Write the failing test** — on a feature branch with `origin/main`'s `timeline.md` = "MAIN", the branch's committed `timeline.md` = "BRANCH-STALE", assert `contextPayload` embeds "MAIN".
- [ ] **Step 2: Verify fails** — `go test ./internal/cli/ -run Context -v` → FAIL.
- [ ] **Step 3: Implement** — in the file-backed read for a doc whose `rel` is in `derivedDocPaths`, when `onNonDefaultBranch(cwd)`, try `gitcli.ShowFile(cwd, "origin/"+gitcli.DefaultBranch(cwd), rel)` first; fall back to the existing `os.ReadFile`. Keep the "absent" element behavior when neither source exists. Add a helper `readDerivedForContext(cwd, rel string) (string, bool)` to keep `contextPayload` readable.
- [ ] **Step 4: Verify pass** — `go test ./internal/cli/ -run Context -v` → PASS; confirm the default-branch path still reads the local file (unchanged) and no network is invoked.
- [ ] **Step 5: Commit** — `logmind log "logmind context renders derived docs from the last-fetched main on a branch" -r "Cold-start context reflects main's current decisions instead of the branch's stale merge-base snapshot, without a network call — origin/<default> is read locally; warp does the fetch" -i "The branch's committed derived docs stay at the merge-base (invariant); only the context RENDERING prefers the fresher local main copy"`

---

## Task 8: L3 — blocking PR gate + main-only regen (CI, lockstep)

**Files:**
- Rewrite: `.github/workflows/regen-timeline.yml`
- Rewrite: `internal/templates/github/regen-timeline.yml.template` (bump `v7` → `v8`)

**Contract:** keep job name `check-derived-docs`. The PR gate BLOCKS (exit 1) when the PR modified either derived doc — GitHub's 3-dot PR diff IS the branch-vs-merge-base delta, correct for forks, no checkout needed. A separate `push: [main]` job regenerates + commits the docs (the only writer to a tracked branch); no-op when current (no push loop).

- [ ] **Step 1: Write the new `regen-timeline.yml`** (self-repo variant):

```yaml
name: logmind / check-derived-docs

# v2.0.0 derived-docs-on-main. The two derived docs are regenerated ONLY here on
# main. On a PR this gate is NON-mutating and BLOCKING: it fails if the branch
# edited either derived doc (which would cause cross-PR merge conflicts). A
# branch's real decisions live in docs/decisions-branches/<branch>.md; timeline
# and file-structure regenerate on main after merge.

on:
  pull_request:
    types: [opened, synchronize, reopened]
  push:
    branches: [main]

permissions:
  contents: write   # PR gate uses read (gh pr diff); the main job commits regen

jobs:
  check-derived-docs:
    name: check-derived-docs
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    steps:
      - name: Assert the branch did not edit the derived docs
        env:
          GH_TOKEN: ${{ github.token }}
          PR: ${{ github.event.pull_request.number }}
        run: |
          set -euo pipefail
          changed=$(gh pr diff "$PR" --name-only --repo "$GITHUB_REPOSITORY")
          if printf '%s\n' "$changed" | grep -qxE 'docs/(timeline|file-structure)\.md'; then
            echo "::error title=Derived docs were edited on this branch::This PR modifies docs/timeline.md and/or docs/file-structure.md. These are DERIVED, main-only artifacts; editing them on a branch causes the cross-PR merge conflicts v2.0.0 eliminates. Fix: revert them to main's version — run 'logmind warp', or 'git checkout origin/main -- docs/timeline.md docs/file-structure.md' — then commit. Their real content regenerates on main after merge."
            exit 1
          fi
          echo "Branch did not edit the derived docs — the merge cannot conflict on them."

  regen-on-main:
    name: regen-on-main
    if: github.event_name == 'push'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
        with:
          fetch-depth: 0
          persist-credentials: false
      - uses: actions/setup-go@v6
        with:
          go-version: "1.22"
          cache: true
      - name: Build logmind from source
        run: make build
      - name: Regenerate derived docs
        run: |
          set -euo pipefail
          ./bin/logmind timeline --write docs/timeline.md
          ./bin/logmind file-structure --write docs/file-structure.md
      - name: Commit + push if changed
        env:
          PAT: ${{ secrets.LOGMIND_AUTO_REGEN_PAT }}
        run: |
          set -euo pipefail
          if [ -z "$(git status --porcelain -- docs/timeline.md docs/file-structure.md)" ]; then
            echo "Derived docs already current on main."
            exit 0
          fi
          if [ -z "${PAT:-}" ]; then
            echo "::warning title=Derived docs stale on main::No LOGMIND_AUTO_REGEN_PAT configured; cannot push the regen. main is momentarily stale until the next push or a manual regen. (No conflict risk — this is a freshness-only gap.)"
            exit 0
          fi
          git config user.name "github-actions[bot]"
          git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
          git add docs/timeline.md docs/file-structure.md
          git commit -m "[skip-logmind] chore: regen derived docs"
          git push "https://x-access-token:${PAT}@github.com/${GITHUB_REPOSITORY}.git" "HEAD:main"
```

- [ ] **Step 2: Write the lockstep template** `internal/templates/github/regen-timeline.yml.template` — identical jobs, with `# logmind-template-version: v8` as line 1, and the regen job using `thrillmade/setup-logmind@v1.0.0` (with `token: ${{ github.token }}`) + `logmind timeline --write ...` instead of `make build` + `./bin/logmind`.
- [ ] **Step 3: Lint both** — `yamllint`/`actionlint` if available; otherwise `python3 -c "import yaml,sys; yaml.safe_load(open(sys.argv[1]))" <file>` for both. Confirm the two files differ ONLY in the build/setup mechanism and the template-version line (diff them side by side).
- [ ] **Step 4: Update the template-version test** — if a test pins the installed template's version marker, bump its expectation to `v8`. `go test ./internal/... -run Template -v`.
- [ ] **Step 5: Commit** — `logmind log "L3: check-derived-docs becomes a blocking PR gate; derived docs regenerate on main only" -r "The structural wall: a PR that edits a derived doc cannot merge (gh pr diff is the branch-vs-merge-base delta, fork-correct), so even an unforeseen write path can never reintroduce a conflict; regen moves to a main-only job that no-ops when current (no push loop)" -a "merge-base diff in shell (rejected: base.sha may be absent in a fork checkout; gh pr diff is simpler and fork-correct)" -i "Existing PRs that modified derived docs (e.g. site #200) now block until reverted — the intended one-time migration; keeps job name check-derived-docs for ruleset matching; template bumped v7->v8"`

---

## Task 9: Headline zero-conflict integration test + layer regression tests

**Files:**
- Create: `internal/cli/derived_integration_test.go`

- [ ] **Step 1: Two-concurrent-branches merge-clean test** — build a temp repo, scaffold logmind; on `main` commit timeline.md. Branch `A` and `B` from main; on each, `logmind log` a decision (writing only `docs/decisions-branches/<branch>.md`, derived docs restored by L1). Merge A into main, then B into main; assert BOTH merges succeed with **no conflict** on `docs/timeline.md`/`docs/file-structure.md` (check `git merge` exit 0 and no conflict markers).

```go
func TestConcurrentBranches_MergeWithoutDerivedDocConflict(t *testing.T) {
	repo := initRepoWithLogmind(t)
	for _, br := range []string{"feat/a", "feat/b"} {
		runGit(t, repo, "checkout", "main")
		runGit(t, repo, "checkout", "-b", br)
		mustRunLog(t, repo, "decision on "+br)
	}
	runGit(t, repo, "checkout", "main")
	for _, br := range []string{"feat/a", "feat/b"} {
		out, _, err := gitcli.RunCaptured(repo, "merge", "--no-edit", br)
		if err != nil || strings.Contains(out, "CONFLICT") {
			t.Fatalf("merge %s conflicted: %v\n%s", br, err, out)
		}
	}
}
```

- [ ] **Step 2: Verify** — `go test ./internal/cli/ -run TestConcurrentBranches -v` → PASS.
- [ ] **Step 3: Full suite** — `go build ./... && go test ./... && go vet ./... && gofmt -l .` all clean.
- [ ] **Step 4: Commit** — `logmind log "Integration test: concurrent branches merge with zero derived-doc conflict" -r "The headline proof of the feature — two branches each logging a decision merge into main cleanly because L1 keeps their derived docs at the merge-base; this is the regression guard for the whole invariant"`

---

## Task 10: SPEC §0.4 reword + agent-skill note (cross-repo, coordinate)

**Files (separate repos — do NOT edit from this repo's worktree):**
- `thrillmade/protocol` `SPEC.md` §0.4 — sync-invariant reword.
- agent-skills logmind skill — "staying current" note.

- [ ] **Step 1: Draft the §0.4 reword** as an additive `<!-- v1.6.0 addition (DRAFT ...) -->` comment (matching the house draft-then-tag flow; marker stays 1.5.1 until the CTO tags spec-v1.6.0): "Derived docs (`docs/timeline.md`, `docs/file-structure.md`) are regenerated at the integration point (the default branch); a non-default branch carries a byte-identical merge-base snapshot and never regenerates them, so concurrent branches never conflict on derived docs. Freshness is served by `logmind warp` / a cold-start `logmind context` refresh, both read-only." Additive per §8.3; co-tags with the 1.6.0 bump.
- [ ] **Step 2: Draft the skill "staying current" note** — one paragraph: "Your branch's `docs/timeline.md` is a snapshot of main from when you branched. Run `logmind warp` (or re-run `logmind context`) to see decisions merged to main since. Never edit the derived docs — they regenerate on main; the CI gate blocks a PR that touches them."
- [ ] **Step 3: Deliver both as orchestrator hand-off prompts** to the protocol co-author and the agent-skills owner (these land in their repos, co-tagging with spec-v1.6.0 / the skill wave). Do NOT commit cross-repo from here.

---

## Self-Review

**Spec coverage:** L0 (Task 4) · L1 (Task 3) · L3 blocking gate + main regen (Task 8) · `warp` (Task 5) · 3rd pulse probe (Task 6) · context refresh (Task 7) · gitcli foundation (Task 1) · shared constants (Task 2) · headline conflict-free proof (Task 9) · SPEC §0.4 + skill (Task 10). L2 (dedicated pre-commit strip) is intentionally deferred — redundant with L3 (the wall) and L1 (the commit path); the only uncovered path is a raw `git commit --no-verify` of a hand-edited derived doc, which L3 blocks at PR time. Recorded here so the omission is explicit.

**Type consistency:** `derivedDocPaths` / `onNonDefaultBranch` (Task 2) are the single definitions consumed by Tasks 3/5/6/7. `gitcli.{MergeBase,Fetch,ShowFile,RestorePathsToHead}` (Task 1) signatures match every call site. `RunCaptured(repoRoot, args...) (stdout, stderr string, err error)` used consistently.

**Ordering / dependencies:** 1 → 2 → {3, 5, 6, 7} (all consume 1+2, file-isolated from each other) ; 4 (hooks, isolated) ; 8 (CI, isolated) ; 9 (integration, needs 3+4) ; 10 (cross-repo, last). Tasks 3/5/6/7 and 4 and 8 can run in parallel once 1+2 land.

**Risk surface:** Task 8 (CI semantics — a wrong `if:`/permission strands a required check) and Task 4 (golden-fixture body change) are the highest-risk; both get opus-tier build + full dual review. Task 3 is load-bearing but small.
