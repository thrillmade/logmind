// timeline_archive_driver_test.go — the two BLOCK 2 regressions.
//
//  1. docs/timeline-archive.md was added to derivedDocPaths, the pre-commit
//     restore, warp and the CI gate — and to NO merge driver. Two branches that
//     each regenerate the pair therefore handed the user `UU` and conflict
//     markers inside a file whose own header says "do not edit by hand".
//  2. `logmind timeline --write <anywhere>` ALSO rewrote the repo's tracked
//     docs/timeline-archive.md, because the archive's path was pinned to
//     <cwd>/docs instead of following --write. It fired on every merge-driver
//     run, which invokes `--write <git scratch file>`.
//
// Everything here is asserted on real artifacts in real git repositories —
// the `.gitattributes` a real `logmind init` writes, the bytes on disk after a
// real `git merge`, and the rendered output of the real command.
//
// The merge-driver coverage lives in this NEW file rather than
// internal/gitattr/gitattr_test.go, which another lane owns.
package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/gitattr"
)

// seedMarkedBranchFile writes a docs/decisions-branches/<stem>.md holding n
// §1.6.3 entry-block markers — the shape `logmind log` writes — so each
// decision becomes its own timeline row. A MARKERLESS file collapses to one
// row, which is far too few to push anything past the §3.3 cut.
func seedMarkedBranchFile(t *testing.T, repo, stem string, n int, year int, prefix string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("← back to [docs/timeline.md](../timeline.md)\n\n")
	for i := 1; i <= n; i++ {
		date := fmtDate(year, (i-1)%12+1, (i-1)%28+1)
		title := prefix + " decision " + itoa(i)
		b.WriteString("<!-- logmind-entry-start: " + date + "-" + prefix + "-decision-" + itoa(i) + " -->\n")
		b.WriteString("- **" + date + "** — " + title + "\n")
		b.WriteString("<!-- logmind-entry-end -->\n\n")
		b.WriteString("## " + date + " 09:00 - " + title + "\n\n**Reasoning:** because\n\n---\n\n")
	}
	mustMkdir(t, filepath.Join(repo, "docs", "decisions-branches"))
	mustWrite(t, filepath.Join(repo, "docs", "decisions-branches", stem+".md"), b.String())
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func fmtDate(y, m, d int) string {
	pad := func(n int) string {
		s := itoa(n)
		if len(s) < 2 {
			return "0" + s
		}
		return s
	}
	return itoa(y) + "-" + pad(m) + "-" + pad(d)
}

// TestTimeline_WriteElsewhere_LeavesTheRepoArchiveAlone is regression (2), in
// both of the directions it was wrong in.
//
// The archive's path was first PINNED to <cwd>/docs, so `--write /elsewhere.md`
// rewrote the repo's tracked docs/timeline-archive.md on the way past. It was
// then made to FOLLOW --write, which moved the same unrequested write to
// whatever directory --write named — the worktree root, on every merge-driver
// run. Neither is a path the caller named, so the probe asserts both: the
// repo's archive is untouched AND no sibling appears beside the target.
//
// The probe is deliberately loud: docs/timeline-archive.md is filled with
// content a render could never produce, so if the command touches it at all
// the sentinel is gone.
func TestTimeline_WriteElsewhere_LeavesTheRepoArchiveAlone(t *testing.T) {
	withTempCwd(t, func(d string) {
		scaffoldDocs(t)
		seedMarkedBranchFile(t, d, "base", 60, 2020, "base")

		repoArchive := filepath.Join(d, "docs", "timeline-archive.md")
		const sentinel = "DELIBERATELY STALE — nothing may rewrite this\n"
		mustWrite(t, repoArchive, sentinel)

		elsewhere := t.TempDir()
		target := filepath.Join(elsewhere, "elsewhere.md")

		root := NewRootCmd()
		root.SetArgs([]string{"timeline", "--write", target})
		var out strings.Builder
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("timeline --write %s: %v\n%s", target, err, out.String())
		}

		got, err := os.ReadFile(repoArchive)
		if err != nil {
			t.Fatalf("read %s: %v", repoArchive, err)
		}
		if string(got) != sentinel {
			t.Errorf("--write %s rewrote the repo's tracked docs/timeline-archive.md.\ncommand output:\n%s\nfile now:\n%s",
				target, out.String(), truncateForTest(string(got)))
		}
		if strings.Contains(out.String(), repoArchive) {
			t.Errorf("--write %s reported writing %s:\n%s", target, repoArchive, out.String())
		}

		// And nothing landed beside the target either: --write wrote the one
		// file it was given.
		entries, err := os.ReadDir(elsewhere)
		if err != nil {
			t.Fatalf("readdir %s: %v", elsewhere, err)
		}
		if len(entries) != 1 || entries[0].Name() != "elsewhere.md" {
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Errorf("--write %s wrote more than the file it was given: %v\noutput:\n%s", target, names, out.String())
		}
	})
}

func truncateForTest(s string) string {
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}

// TestGitattributes_RegistersAMergeDriverForEveryDerivedDoc is regression (1)
// at the level that makes it a class rather than a case: every purely-derived
// doc governed by the zero-conflict invariant must have a merge driver, and
// every driver named in .gitattributes must actually be defined in git config.
//
// Asserted against the .gitattributes and .git/config a REAL `logmind init`
// leaves behind in a REAL git repo — the artifacts git itself reads.
func TestGitattributes_RegistersAMergeDriverForEveryDerivedDoc(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	withTempCwd(t, func(d string) {
		gitIn(t, d, "init", "-q", "-b", "main", ".")
		gitIn(t, d, "config", "user.email", "t@example.com")
		gitIn(t, d, "config", "user.name", "t")

		root := NewRootCmd()
		root.SetArgs([]string{"init"})
		var out strings.Builder
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("logmind init: %v\n%s", err, out.String())
		}

		data, err := os.ReadFile(filepath.Join(d, ".gitattributes"))
		if err != nil {
			t.Fatalf("read .gitattributes: %v", err)
		}
		attrs := string(data)

		// Every derived doc is registered...
		driverFor := map[string]string{}
		for _, line := range strings.Split(attrs, "\n") {
			f := strings.Fields(line)
			if len(f) == 2 && strings.HasPrefix(f[1], "merge=") {
				driverFor[f[0]] = strings.TrimPrefix(f[1], "merge=")
			}
		}
		for _, p := range derivedDocPaths {
			if driverFor[p] == "" {
				t.Errorf("derived doc %s has no merge driver in .gitattributes — a parallel merge hands the user conflict markers in a file they are told never to edit by hand:\n%s", p, attrs)
			}
		}

		// ...and every driver it names is actually DEFINED in this clone's
		// git config. git refuses to run a driver it has no definition for,
		// so a registration without a definition is a silent no-op.
		for path, driver := range driverFor {
			key := "merge." + driver + ".driver"
			val := gitOut(d, "config", "--local", "--get", key)
			if val == "" {
				t.Errorf(".gitattributes routes %s through driver %q, but %s is not set in git config", path, driver, key)
			}
		}

		// The archive's driver must render the ARCHIVE half, not the recent
		// one: git hands a driver the scratch file for ONE conflicted path.
		archiveDriver := driverFor["docs/timeline-archive.md"]
		if archiveDriver != "" {
			cmdline := gitOut(d, "config", "--local", "--get", "merge."+archiveDriver+".driver")
			if !strings.Contains(cmdline, "--half archive") {
				t.Errorf("docs/timeline-archive.md's driver is %q, which does not select the archive half; it would write the RECENT timeline into the archive file", cmdline)
			}
		}
	})
}

// TestGitattr_DefaultLinesCoverEveryDerivedDoc pins the same coupling at the
// source-of-truth level, so the two lists cannot drift between releases even
// where no init has run.
func TestGitattr_DefaultLinesCoverEveryDerivedDoc(t *testing.T) {
	registered := map[string]bool{}
	for _, line := range gitattr.DefaultLines {
		if f := strings.Fields(line); len(f) > 0 {
			registered[f[0]] = true
		}
	}
	for _, p := range derivedDocPaths {
		if !registered[p] {
			t.Errorf("derivedDocPaths has %s but gitattr.DefaultLines does not register a merge driver for it", p)
		}
	}
}

// TestGitattributes_UpgradesAnExistingBlockInPlace: a repo initialised by an
// older binary has a logmind block WITHOUT the newly-shipped registration.
// EnsureBlock must add it — otherwise every existing repo keeps handing its
// users conflict markers forever and nothing in the system ever says so.
// A user's own edit inside the block survives untouched.
func TestGitattributes_UpgradesAnExistingBlockInPlace(t *testing.T) {
	withTempCwd(t, func(d string) {
		path := filepath.Join(d, ".gitattributes")
		// The block exactly as a pre-archive binary wrote it, plus a line the
		// user added themselves.
		mustWrite(t, path, gitattr.BlockStart+"\n"+
			"docs/timeline.md          merge=logmind-timeline\n"+
			"docs/file-structure.md    merge=logmind-file-structure\n"+
			"*.bin                     binary\n"+
			gitattr.BlockEnd+"\n")

		changed, err := gitattr.EnsureBlock(path)
		if err != nil {
			t.Fatalf("EnsureBlock: %v", err)
		}
		if !changed {
			t.Error("EnsureBlock reported no change on a block missing a shipped registration")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		body := string(data)
		mustContain(t, body, "docs/timeline-archive.md")
		mustContain(t, body, "*.bin                     binary")
		// Exactly one block, and the addition landed inside it.
		if n := strings.Count(body, gitattr.BlockStart); n != 1 {
			t.Errorf("want exactly 1 logmind block, got %d:\n%s", n, body)
		}
		archiveIdx := strings.Index(body, "docs/timeline-archive.md")
		endIdx := strings.Index(body, gitattr.BlockEnd)
		if archiveIdx < 0 || archiveIdx > endIdx {
			t.Errorf("the new registration landed outside the block:\n%s", body)
		}

		// Idempotent: a second call changes nothing.
		changed2, err := gitattr.EnsureBlock(path)
		if err != nil {
			t.Fatalf("EnsureBlock (2nd): %v", err)
		}
		if changed2 {
			t.Error("EnsureBlock is not idempotent — it reported a change on an already-complete block")
		}
	})
}

// TestTimeline_Half_WritesExactlyOneFile pins the primitive BOTH merge drivers
// depend on: PATH is the ONLY file touched — no sibling, ever, with or without
// --half. git hands a driver a scratch file at the worktree root, so a sibling
// write drops a stray timeline-archive.md there on every merge.
//
// The empty case is the load-bearing one. `merge.logmind-timeline.driver` is
// FROZEN at `logmind timeline --write %A` — an older binary on PATH executes
// it — so the no-flag invocation is exactly what runs on a real timeline
// merge, and it is the one that used to leave the stray.
func TestTimeline_Half_WritesExactlyOneFile(t *testing.T) {
	for _, half := range []string{"", "recent", "archive"} {
		name := half
		if name == "" {
			name = "default(no --half, the frozen driver's invocation)"
		}
		t.Run(name, func(t *testing.T) {
			withTempCwd(t, func(d string) {
				scaffoldDocs(t)
				seedMarkedBranchFile(t, d, "base", 60, 2020, "base")

				scratch := t.TempDir()
				target := filepath.Join(scratch, ".merge_file_XXXX")

				args := []string{"timeline", "--write", target}
				if half != "" {
					args = append(args, "--half", half)
				}
				root := NewRootCmd()
				root.SetArgs(args)
				var out strings.Builder
				root.SetOut(&out)
				root.SetErr(&out)
				if err := root.Execute(); err != nil {
					t.Fatalf("timeline %v: %v\n%s", args, err, out.String())
				}

				entries, err := os.ReadDir(scratch)
				if err != nil {
					t.Fatalf("readdir: %v", err)
				}
				if len(entries) != 1 || entries[0].Name() != ".merge_file_XXXX" {
					var names []string
					for _, e := range entries {
						names = append(names, e.Name())
					}
					t.Errorf("timeline %v wrote more than the file it was given: %v", args, names)
				}

				body := mustReadString(t, target)
				// The two halves are distinguishable by their own headers.
				wantHeader := "# Decision Timeline\n"
				if half == "archive" {
					wantHeader = "# Decision Timeline — Archive\n"
				}
				if !strings.HasPrefix(body, wantHeader) {
					t.Errorf("--half %s rendered the wrong half; want prefix %q, got:\n%s", half, wantHeader, truncateForTest(body))
				}
			})
		})
	}
}

func mustReadString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// TestMerge_DivergedTimelineArchive_ResolvesWithoutConflictMarkers is the
// user-visible symptom, end to end: two branches whose docs/timeline-archive.md
// legitimately diverge, merged by REAL git with the drivers exactly as
// `logmind init` installs them.
//
// Before the fix this left `UU docs/timeline-archive.md` and conflict markers
// inside a file whose own header says "do not edit by hand".
//
// The fixture removes the pre-commit hook, whose job is the separate
// zero-conflict invariant (it restores derived docs to HEAD on a non-default
// branch). Leaving it in would mean neither side ever committed a divergent
// archive and the merge under test would never happen.
func TestMerge_DivergedTimelineArchive_ResolvesWithoutConflictMarkers(t *testing.T) {
	r := newDriverMergeRepo(t)
	repo := r.dir

	// Enough rows that the §3.3 cut is real (RecentLimit is 50).
	seedMarkedBranchFile(t, repo, "base", 60, 2020, "base")
	regenPair(t, r.run)
	r.commitAll("seed")
	if n := strings.Count(mustReadString(t, filepath.Join(repo, "docs", "timeline-archive.md")), "logmind-entry-start"); n == 0 {
		t.Fatalf("fixture precondition: the archive is empty, so it cannot diverge")
	}

	// Both sides add decisions OLDER than the cut, so each side's ARCHIVE
	// changes and the two disagree.
	gitIn(t, repo, "checkout", "-q", "-b", "left")
	seedMarkedBranchFile(t, repo, "left", 30, 2010, "left")
	regenPair(t, r.run)
	r.commitAll("left")

	gitIn(t, repo, "checkout", "-q", "main")
	gitIn(t, repo, "checkout", "-q", "-b", "right")
	seedMarkedBranchFile(t, repo, "right", 30, 2011, "right")
	regenPair(t, r.run)
	r.commitAll("right")

	// Fixture precondition: the archive really does diverge between the two.
	diff := exec.Command("git", "diff", "--quiet", "left", "right", "--", "docs/timeline-archive.md")
	diff.Dir = repo
	if err := diff.Run(); err == nil {
		t.Fatal("fixture precondition: docs/timeline-archive.md does not diverge between left and right")
	}

	gitIn(t, repo, "checkout", "-q", "left")
	mergeOut, mergeErr := r.merge("right")

	status := gitOut(repo, "status", "--porcelain")
	archive := mustReadString(t, filepath.Join(repo, "docs", "timeline-archive.md"))
	for _, marker := range []string{"<<<<<<<", ">>>>>>>"} {
		if strings.Contains(archive, marker) {
			t.Errorf("docs/timeline-archive.md carries a %q conflict marker after the merge — it has no working merge driver.\nmerge output:\n%s\nstatus:\n%s", marker, mergeOut, status)
		}
	}
	if strings.Contains(status, "UU") {
		t.Errorf("the merge left an unresolved path:\n%s\nmerge output:\n%s", status, mergeOut)
	}
	if mergeErr != nil {
		t.Errorf("git merge failed: %v\n%s", mergeErr, mergeOut)
	}
	// And no stray sibling next to git's scratch file at the worktree root.
	if pathExists(filepath.Join(repo, "timeline-archive.md")) {
		t.Errorf("the merge driver dropped a stray timeline-archive.md at the worktree root:\n%s", mergeOut)
	}
}

// TestMerge_RecentTimelineConflict_LeavesNoStrayAndClobbersNothing is BLOCK-1
// measured where it bit: a merge in which docs/timeline.md ITSELF conflicts,
// so git runs `merge.logmind-timeline` — the driver whose command string is
// frozen at `logmind timeline --write %A` and therefore carries no --half.
//
// %A is a scratch file git creates at the WORKTREE ROOT. While `--write` also
// wrote an inferred sibling, that run left `timeline-archive.md` beside it:
//
//   - untracked, `git check-ignore` exit 1, doctor and check-links both
//     silent — and the next `logmind log` commits it, because --stage all is
//     the default. From there it propagates to every clone.
//   - and where a file of that name was already TRACKED at the root, the
//     merge printed "✓ Regenerated timeline-archive.md" over the user's
//     content. Silent data loss.
//
// Both subtests run the same real merge; they differ only in whether a
// root-level timeline-archive.md exists to be clobbered.
func TestMerge_RecentTimelineConflict_LeavesNoStrayAndClobbersNothing(t *testing.T) {
	const userFile = "timeline-archive.md"
	const sentinel = "# my own notes\n\nNothing logmind writes may replace this.\n"

	for _, tc := range []struct {
		name        string
		trackAtRoot bool
	}{
		{name: "no_root_file_no_stray_appears", trackAtRoot: false},
		{name: "tracked_root_file_is_not_clobbered", trackAtRoot: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newDriverMergeRepo(t)
			repo := r.dir

			// A base whose 50 most recent rows are shared by both sides.
			seedMarkedBranchFile(t, repo, "base", 60, 2020, "base")
			if tc.trackAtRoot {
				mustWrite(t, filepath.Join(repo, userFile), sentinel)
			}
			regenPair(t, r.run)
			r.commitAll("seed")

			// Both sides add rows NEWER than the base, so each side's RECENT
			// half changes and the two disagree — which is what makes git run
			// merge.logmind-timeline on docs/timeline.md.
			gitIn(t, repo, "checkout", "-q", "-b", "left")
			seedMarkedBranchFile(t, repo, "left", 30, 2030, "left")
			regenPair(t, r.run)
			r.commitAll("left")

			gitIn(t, repo, "checkout", "-q", "main")
			gitIn(t, repo, "checkout", "-q", "-b", "right")
			seedMarkedBranchFile(t, repo, "right", 30, 2031, "right")
			regenPair(t, r.run)
			r.commitAll("right")

			// Fixture precondition: without a RECENT-half divergence the
			// frozen driver never runs and this test proves nothing.
			diff := exec.Command("git", "diff", "--quiet", "left", "right", "--", "docs/timeline.md")
			diff.Dir = repo
			if err := diff.Run(); err == nil {
				t.Fatal("fixture precondition: docs/timeline.md does not diverge between left and right, so merge.logmind-timeline never fires")
			}

			gitIn(t, repo, "checkout", "-q", "left")
			mergeOut, mergeErr := r.merge("right")
			if mergeErr != nil {
				t.Errorf("git merge failed: %v\n%s", mergeErr, mergeOut)
			}

			status := gitOut(repo, "status", "--porcelain")
			rootPath := filepath.Join(repo, userFile)

			if tc.trackAtRoot {
				got := mustReadString(t, rootPath)
				if got != sentinel {
					t.Errorf("the merge replaced the tracked %s at the worktree root — silent data loss.\nwant:\n%s\ngot:\n%s\nmerge output:\n%s",
						userFile, sentinel, truncateForTest(got), mergeOut)
				}
				if strings.Contains(status, userFile) {
					t.Errorf("the merge left %s modified at the worktree root:\n%s\nmerge output:\n%s", userFile, status, mergeOut)
				}
			} else if pathExists(rootPath) {
				t.Errorf("the merge driver dropped a stray %s at the worktree root — untracked, invisible to doctor, and committed by the next `logmind log` (--stage all is the default).\nstatus:\n%s\nmerge output:\n%s",
					userFile, status, mergeOut)
			}

			// Nothing else got left behind either: an untracked file after a
			// merge is a write to a path no caller named, whatever its name.
			for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
				if strings.HasPrefix(line, "??") {
					t.Errorf("the merge left an untracked file behind: %q\nfull status:\n%s\nmerge output:\n%s", line, status, mergeOut)
				}
			}
		})
	}
}

// regenPair regenerates both halves of the §3.3 split the way every real
// caller does — each file named explicitly, because `--write` writes the one
// file it is given.
func regenPair(t *testing.T, run func(args ...string) string) {
	t.Helper()
	run("timeline", "--write", "docs/timeline.md")
	run("timeline", "--write", "docs/timeline-archive.md", "--half", "archive")
}

// driverMergeRepo is a real repository with a real `logmind init` in it, plus
// the handles a merge-driver test needs to drive it.
type driverMergeRepo struct {
	dir string
	// run invokes the built logmind binary in dir.
	run func(args ...string) string
	// commitAll stages everything and commits, bypassing the commit guard.
	commitAll func(msg string)
	// merge runs a real `git merge` with the built binary on PATH, so GIT
	// itself can invoke the drivers. Returns the combined output and error.
	merge func(branch string) (string, error)
}

// newDriverMergeRepo builds the real binary, runs a real `logmind init` in a
// fresh repo, and returns it wired for merge-driver tests.
//
// The pre-commit hook is removed: its job is the separate zero-conflict
// invariant (restore derived docs to HEAD on a non-default branch). Leaving it
// in would mean neither side ever committed a divergent derived doc and the
// merge under test would never happen.
func newDriverMergeRepo(t *testing.T) *driverMergeRepo {
	t.Helper()
	if testing.Short() {
		t.Skip("builds a binary and runs real git merges")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	binPath := buildGuardCommitBinary(t, goBin)

	repo := t.TempDir()
	gitIn(t, repo, "init", "-q", "-b", "main", ".")
	gitIn(t, repo, "config", "user.email", "t@example.com")
	gitIn(t, repo, "config", "user.name", "t")

	// The drivers must be findable when GIT invokes them, not only when this
	// test does: git runs a merge driver through `sh -c` with the environment
	// it was handed.
	withBin := func() []string {
		return append(os.Environ(),
			"PATH="+filepath.Dir(binPath)+string(os.PathListSeparator)+os.Getenv("PATH"),
			"LOGMIND_ALLOW_GIT_COMMIT=1")
	}

	r := &driverMergeRepo{dir: repo}
	r.run = func(args ...string) string {
		t.Helper()
		cmd := exec.Command(binPath, args...)
		cmd.Dir = repo
		cmd.Env = withBin()
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("logmind %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	r.commitAll = func(msg string) {
		t.Helper()
		gitIn(t, repo, "add", "-A")
		cmd := exec.Command("git", "commit", "-qm", msg)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "LOGMIND_ALLOW_GIT_COMMIT=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit %q: %v\n%s", msg, err, out)
		}
	}
	r.merge = func(branch string) (string, error) {
		t.Helper()
		cmd := exec.Command("git", "merge", "--no-edit", branch)
		cmd.Dir = repo
		cmd.Env = withBin()
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	r.run("init")
	_ = os.Remove(filepath.Join(repo, ".git", "hooks", "pre-commit"))
	return r
}

// TestTimeline_BadHalf_IsRejected: an unknown --half value must fail loudly
// rather than silently falling through to a different half, which would write
// the wrong rendering into the file the caller named.
func TestTimeline_BadHalf_IsRejected(t *testing.T) {
	withTempCwd(t, func(d string) {
		scaffoldDocs(t)
		root := NewRootCmd()
		root.SetArgs([]string{"timeline", "--write", filepath.Join(d, "docs", "timeline.md"), "--half", "everything"})
		var out strings.Builder
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err == nil {
			t.Fatalf("--half everything was accepted:\n%s", out.String())
		}
		mustContain(t, out.String(), "--half must be")
	})
}
