// search_no_archive_branch_test.go — regressions for PR #301 round-4 panel
// findings: --no-archive deciding what to drop by Label alone, ignoring
// IsBranch.
//
// decisions.NonBranchSources() stamps Label: "archive" on the legacy
// docs/decisions-archive.md. decisions.BranchLabelFromFilename mirrors a
// branch's sanitized filename straight back to its name, so a repository
// with a branch literally named "archive" gets a Source with the SAME
// Label ("archive") but IsBranch: true, from
// docs/decisions-branches/archive.md. searchSources used to test Label
// alone, so --no-archive dropped that branch's decisions file along with
// the real archive, and the archive= receipt then reported a scan that
// didn't happen (or claimed a branch-only scan was "the archive").
//
// `show` and `repomap` guard the same Label collision with `!s.IsBranch`
// already, elsewhere in this PR — see show.go:166 and rank.go:95 — which is
// what makes the one-line search.go gap a divergence rather than a design
// question.
//
// Both tests below are pinned on the RENDERED OUTPUT of the real `logmind
// search --quiet` command against a real repo, never on searchSources'
// internal archiveScanned return value: the harm is in what the command
// PRINTS to a caller deciding whether to trust the scan.
package cli

import (
	"path/filepath"
	"testing"
)

// TestSearch_NoArchive_BranchNamedArchiveIsNotExcluded is the BLOCK
// regression: --no-archive must exclude ONLY the non-branch legacy archive
// file, never a branch whose name happens to be "archive". The receipt must
// also tell the truth in this exact shape of repo.
func TestSearch_NoArchive_BranchNamedArchiveIsNotExcluded(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)

		// A REAL legacy archive file, so --no-archive has something genuine
		// to exclude — the control that proves the flag still works at all.
		mustWrite(t, filepath.Join(d, "docs", "decisions-archive.md"),
			"# Decision Archive\n\n## 2025-01-01 09:00 - Old decision mentioning legacy-archive-term\n\n**Reasoning:** archived-rationale\n\n---\n")

		// Check out a branch literally named "archive" and log a decision
		// there. sanitizeBranchName leaves it as "archive" (no "/" to
		// escape), so the file is docs/decisions-branches/archive.md with
		// Label "archive", IsBranch true — colliding with the legacy
		// archive's Label but not its identity.
		checkoutBranch(t, d, "archive")
		withFakeTTY(t, false, func() { logOnce(t, "Adopt branch-archive-decision for rollout") })
		branchFile := filepath.Join(d, "docs", "decisions-branches", "archive.md")
		if !pathExists(branchFile) {
			t.Fatalf("fixture precondition: %s was not written by logmind log", branchFile)
		}

		// THE REGRESSION: the "archive" branch's own decision must still be
		// found with --no-archive.
		body := runSearchCmd(t, "branch-archive-decision", "--no-archive")
		mustContain(t, body, "docs/decisions-branches/archive.md")
		mustNotContain(t, body, "No matches found")

		// CONTROL, same invocation shape: the legacy archive's term is
		// genuinely excluded by the same flag. If this ever finds a hit the
		// fixture proves nothing about the branch case above.
		control := runSearchCmd(t, "legacy-archive-term", "--no-archive")
		mustContain(t, control, "No matches found")

		// The receipt must be truthful for this exact repo shape: the real
		// (non-branch) archive was excluded, so archive=false — even though
		// a source LABELED "archive" (the branch) was scanned.
		receipt := runSearchCmd(t, "branch-archive-decision", "--no-archive", "--quiet")
		mustContain(t, receipt, "archive=false")
	})
}

// TestSearch_QuietReceipt_ArchivePinnedToActualScan is the HIGH regression:
// the --quiet receipt's archive= field must track whether
// docs/decisions-archive.md was ACTUALLY scanned in THIS run, not a
// hardcoded value. Each case is chosen so a mutation forcing archive=true
// (or archive=false) unconditionally fails at least one of them — the
// mutation the panel reported surviving the whole suite.
func TestSearch_QuietReceipt_ArchivePinnedToActualScan(t *testing.T) {
	cases := []struct {
		name        string
		writeLegacy bool
		writeBranch bool // a branch literally named "archive"
		noArchive   bool
		wantArchive string
	}{
		{
			name:        "no legacy archive on disk, default: archive=false",
			writeLegacy: false,
			wantArchive: "archive=false",
		},
		{
			name:        "no legacy archive on disk, --no-archive: archive=false",
			writeLegacy: false,
			noArchive:   true,
			wantArchive: "archive=false",
		},
		{
			name:        "legacy archive present, default: archive=true",
			writeLegacy: true,
			wantArchive: "archive=true",
		},
		{
			name:        "legacy archive present, --no-archive: archive=false",
			writeLegacy: true,
			noArchive:   true,
			wantArchive: "archive=false",
		},
		{
			name:        "branch named archive, no legacy file, --no-archive: archive=false",
			writeBranch: true,
			noArchive:   true,
			wantArchive: "archive=false",
		},
		{
			name:        "branch named archive PLUS legacy file, --no-archive: archive=false",
			writeLegacy: true,
			writeBranch: true,
			noArchive:   true,
			wantArchive: "archive=false",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withTempCwd(t, func(d string) {
				initLogTestGitRepo(t, d)
				scaffoldDocs(t)

				if tc.writeLegacy {
					mustWrite(t, filepath.Join(d, "docs", "decisions-archive.md"),
						"# Decision Archive\n\n## 2025-01-01 09:00 - Old decision\n\n**Reasoning:** why\n\n---\n")
				}
				if tc.writeBranch {
					checkoutBranch(t, d, "archive")
					withFakeTTY(t, false, func() { logOnce(t, "Adopt something on the archive branch") })
				}

				args := []string{"--quiet"}
				if tc.noArchive {
					args = append(args, "--no-archive")
				}
				receipt := runSearchCmd(t, "something", args...)
				mustContain(t, receipt, tc.wantArchive)
			})
		})
	}
}
