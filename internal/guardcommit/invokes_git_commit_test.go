package guardcommit

import "testing"

// TestInvokesGitCommit covers every MUST/MUST-NOT case from the PR brief.
//
// One deliberate deviation from the brief's literal bucketing: the brief
// lists `git commit -m "has && inside quotes"` under "MUST NOT match", with
// the annotation "(don't mis-split on quoted &&)". Taken literally that
// would mean InvokesGitCommit should return FALSE for a string that begins
// with `git commit -m ...` — but that string unambiguously runs `git
// commit` (the "&&" is just data inside a properly double-quoted -m
// argument; a real shell executes it as one `git commit` invocation, full
// stop). Classifying it as "does not invoke git commit" would be a genuine
// enforcement bypass: any commit whose message happens to contain "&&"
// would silently skip the gate. The annotation's actual intent — "don't
// mis-split the string into two statements at the quoted &&" — is a
// description of how to correctly recognize this as ONE unsplit `git
// commit` statement, which is exactly what treating it as a MUST-match
// case verifies. See TestInvokesGitCommit_QuotedAmpersandsDoNotBypassMatch
// below; flagged here for PR review in case the brief intended the
// opposite and this needs to be revisited.
func TestInvokesGitCommit(t *testing.T) {
	mustMatch := []string{
		`git commit`,
		`git commit -am "x"`,
		`git -C sub commit`,
		`npm test && git commit -m x`,
		`echo $(git commit -m x)`,
		`timeout 30 git commit -m x`,
	}
	for _, cmd := range mustMatch {
		if !InvokesGitCommit(cmd) {
			t.Errorf("InvokesGitCommit(%q) = false; want true", cmd)
		}
	}

	mustNotMatch := []string{
		`git commit-tree -m x`,
		`gh pr merge`,
		`git rebase --continue`,
		`git merge --no-edit`,
	}
	for _, cmd := range mustNotMatch {
		if InvokesGitCommit(cmd) {
			t.Errorf("InvokesGitCommit(%q) = true; want false", cmd)
		}
	}
}

// TestInvokesGitCommit_QuotedAmpersandsDoNotBypassMatch is the flagged
// case from the doc comment above: a quoted "&&" inside a commit message
// must not cause a mis-split that hides the git-commit invocation. See
// TestInvokesGitCommit's doc comment for why this is asserted as a MATCH
// rather than a non-match.
func TestInvokesGitCommit_QuotedAmpersandsDoNotBypassMatch(t *testing.T) {
	cmd := `git commit -m "has && inside quotes"`
	if !InvokesGitCommit(cmd) {
		t.Errorf("InvokesGitCommit(%q) = false; want true (quoted && must not hide the invocation)", cmd)
	}
}

// TestInvokesGitCommit_AdditionalCases covers a few more shapes not in the
// original MUST/MUST-NOT list but worth pinning down given the wrapper /
// global-flag / substitution handling this function implements.
func TestInvokesGitCommit_AdditionalCases(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
	}{
		// Chained wrappers.
		{`nohup timeout 30 git commit -m x`, true},
		// Backtick substitution (the other command-substitution form).
		{"echo `git commit -m x`", true},
		// Nested substitution.
		{`echo $(echo $(git commit -m x))`, true},
		// Separator variety: pipe, semicolon, background, pipe-or.
		{`git status | cat && git commit -m x`, true},
		{`git status; git commit -m x`, true},
		{`git status & git commit -m x`, true},
		{`false || git commit -m x`, true},
		// git global flags before the subcommand.
		{`git --git-dir=/tmp/x.git commit -m x`, true},
		{`git --git-dir /tmp/x.git commit -m x`, true},
		{`git -c user.name=bot commit -m x`, true},
		{`git --work-tree=/tmp/wt commit -m x`, true},
		// Not git at all.
		{`echo hello`, false},
		{`npm run build && npm test`, false},
		// git, but not the commit subcommand.
		{`git status`, false},
		{`git log --oneline`, false},
		// A subject that just mentions "commit" as data, not as the git
		// subcommand.
		{`echo "please commit this"`, false},
		// Empty / whitespace-only input.
		{``, false},
		{`   `, false},
	}
	for _, c := range cases {
		if got := InvokesGitCommit(c.cmd); got != c.want {
			t.Errorf("InvokesGitCommit(%q) = %v; want %v", c.cmd, got, c.want)
		}
	}
}
