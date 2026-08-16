// gitattr_symlink_test.go — regression for a sibling-PR panel finding
// against internal/gitattr: `logmind init` and `logmind doctor --fix` wrote
// OUTSIDE the repository through a dangling symlink planted at
// `.gitattributes`, while STILL printing a success line ("✓ Added logmind
// block to .gitattributes") and a truthful-looking receipt
// (`gitattributes=written`). The mechanism was the one #310 closes
// tree-wide: os.WriteFile follows a symlink at its destination via
// open(2)'s O_CREATE, so a caller that reads a write as "it didn't error,
// so it must have landed at the path I asked for" is wrong whenever that
// path is a symlink — dangling or not.
//
// internal/gitattr/gitattr.go's three os.WriteFile call sites now route
// through atomicio.WriteFile, which refuses a symlinked destination up
// front (RefuseSymlink) instead of following it.
//
// Both tests below run the REAL command path — `logmind init` and `logmind
// doctor --fix` — and assert on what the two things a user actually
// observes say: the rendered output, and the filesystem outside the repo.
// A test against gitattr.EnsureBlock's return value alone would miss the
// reported symptom entirely, because the symptom IS what the CLI layer
// prints on top of that return value.
package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// plantDanglingGitattributesSymlink puts a symlink at <cwd>/.gitattributes
// pointing OUTSIDE cwd, at a target that does not exist yet — the exact
// shape of the reported escape (mirrors
// atomicio_test.go's TestWriteFile_RefusesDanglingSymlinkDestination).
// Returns the (nonexistent) outside target path.
func plantDanglingGitattributesSymlink(t *testing.T, cwd string) string {
	t.Helper()
	outside := filepath.Join(cwd, "..", "escaped-gitattributes")
	link := filepath.Join(cwd, ".gitattributes")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	return outside
}

// assertSymlinkUntouchedAndOutsideClean is the shared filesystem half of
// both regressions: the outside-the-repo target was never created, and the
// planted symlink itself is left exactly as found (not replaced by a
// regular file — a rename onto an existing symlink would detach it, which
// RefuseSymlink deliberately avoids by refusing before ever getting there).
func assertSymlinkUntouchedAndOutsideClean(t *testing.T, cwd, outside string) {
	t.Helper()
	if _, err := os.Lstat(outside); err == nil {
		t.Errorf("wrote outside the repo at %s by following the dangling .gitattributes symlink", outside)
	}
	link := filepath.Join(cwd, ".gitattributes")
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat %s: %v", link, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf(".gitattributes symlink was replaced with a regular file; want it left exactly as planted")
	}
}

// TestInit_DanglingGitattributesSymlink_RefusedNotFollowed is the `logmind
// init` half: a fresh (never-initialized) repo whose .gitattributes is
// ALREADY a dangling symlink when init runs — e.g. a hostile or malformed
// starting tree. Before the fix, EnsureBlock's os.WriteFile silently
// followed the link, wrote outside the repo, and returned (true, nil), so
// init printed the success line. init.go's error handling
// (fmt.Fprintln(...,"Warning: .gitattributes update failed:", err) on a
// non-nil error) is untouched by this fix — it was already correct; it just
// never had a real error to report until EnsureBlock could produce one.
func TestInit_DanglingGitattributesSymlink_RefusedNotFollowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unprivileged symlink creation is unreliable on Windows CI runners")
	}
	withTempCwd(t, func(d string) {
		gitInitCwd(t)
		outside := plantDanglingGitattributesSymlink(t, d)

		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-git"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			// init treats a .gitattributes failure as a warn-and-continue,
			// not fatal — a non-nil error here would mean some OTHER step
			// failed, which is itself worth surfacing.
			t.Fatalf("init: %v\n%s", err, out.String())
		}
		body := out.String()

		// THE FALSE SUCCESS LINE must be gone.
		if contains(body, "✓ Added logmind block to .gitattributes") {
			t.Errorf("init printed the .gitattributes success line despite writing through a dangling symlink; output:\n%s", body)
		}
		// A clear failure must take its place, naming the actual problem.
		mustContain(t, body, "Warning: .gitattributes update failed")
		mustContain(t, body, "symlink")

		assertSymlinkUntouchedAndOutsideClean(t, d, outside)
	})
}

// TestDoctorFix_DanglingGitattributesSymlink_FailsLoudNotFalseSuccess is the
// `logmind doctor --fix` half — the more severe of the two, because
// applyRefresh treats a .gitattributes write failure as the FIRST hard
// error and doctor --fix propagates it as a genuine command failure
// (ErrSilent), never reaching formatDoctorFixOK. Before the fix, the write
// "succeeded" (having landed outside the repo), so --fix reported
// `gitattributes=written` — a truthful-LOOKING receipt for a write that
// never touched the file it claimed to.
func TestDoctorFix_DanglingGitattributesSymlink_FailsLoudNotFalseSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unprivileged symlink creation is unreliable on Windows CI runners")
	}
	withTempCwd(t, func(d string) {
		gitInitCwd(t)
		outside := plantDanglingGitattributesSymlink(t, d)

		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--fix", "--offline"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		err := root.Execute()

		if err == nil {
			t.Fatalf("expected doctor --fix to fail loudly on a dangling .gitattributes symlink; stdout=%s", out.String())
		}
		if !errors.Is(err, ErrSilent) {
			t.Errorf("err = %v; want ErrSilent (the hard-write-error path)", err)
		}
		// THE FALSE RECEIPT must never be printed: no ok doctor-fix line at
		// all reaches stdout on a hard write error (see
		// TestDoctorFix_HardWriteErrorExitsNonZero for the sibling case).
		if contains(out.String(), "gitattributes=written") {
			t.Errorf("doctor --fix reported gitattributes=written for a write that landed outside the repo; stdout:\n%s", out.String())
		}
		if contains(out.String(), "ok doctor-fix") {
			t.Errorf("doctor --fix printed its success receipt despite a hard write error; stdout:\n%s", out.String())
		}
		mustContain(t, errOut.String(), "doctor --fix")
		mustContain(t, errOut.String(), "symlink")

		assertSymlinkUntouchedAndOutsideClean(t, d, outside)
	})
}
