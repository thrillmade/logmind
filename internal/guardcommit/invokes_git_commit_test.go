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
		// Leading env-var assignments must not hide the invocation (a
		// compliant agent inline-setting a var — date backdating, HUSKY=0,
		// GIT_EDITOR — would otherwise silently bypass the gate).
		`FOO=1 git commit -m x`,
		`GIT_AUTHOR_DATE=2020-01-01 git commit`,
		`HUSKY=0 git commit -m x`,
		// The `env` command as a wrapper, with and without its options.
		`env git commit`,
		`env FOO=1 git commit -m x`,
		`env -u HUSKY git commit -m x`,
		// Env assignment stacked on a process wrapper.
		`FOO=1 timeout 30 git commit -m x`,
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
		// An env-assignment prefix in front of a NON-git command must not
		// become a false positive after the assignment is stripped.
		`FOO=1 npm test`,
		`GIT_AUTHOR_DATE=x npm run build`,
		`env npm test`,
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

// TestInvokesGitCommit_WrapperUnwrap_Issue221 covers the three additional
// wrapper shapes from issue #221 that previously slipped past the Layer-1
// gate: (1) a shell `-c <cmdline>` (recurse into the nested command line),
// (2) the `command` builtin and its -p/-v/-V options, and (3) an absolute or
// relative path whose basename is `git`. Each capability is exercised alone
// and in combination, with negative controls that must NOT match.
func TestInvokesGitCommit_WrapperUnwrap_Issue221(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
	}{
		// (1) Shell `-c <cmdline>` recursion — every shell in the set.
		{`sh -c 'git commit'`, true},
		{`bash -c "git commit -m x"`, true},
		{`zsh -c 'git commit -m x'`, true},
		{`dash -c 'git commit'`, true},
		{`ksh -c 'git commit'`, true},
		// The inner command line is fully re-parsed, so &&/;/pipes and
		// leading non-git statements inside `-c` are handled.
		{`sh -c 'cd x && git commit'`, true},
		{`bash -c "npm test && git commit -m x"`, true},
		{`sh -c 'git status; git commit -m x'`, true},
		// Shell by absolute path (matched by basename).
		{`/bin/bash -c "git commit -m x"`, true},
		// Nested shells: recursion peels one `<shell> -c` per level and the
		// input strictly shrinks, so it terminates at the bare `git commit`.
		{`bash -c "sh -c 'git commit'"`, true},

		// (2) The `command` builtin, with and without its options.
		{`command git commit`, true},
		{`command -p git commit`, true},
		{`command -V git commit -m x`, true},

		// (3) Absolute / relative git path (basename == "git").
		{`/usr/bin/git commit`, true},
		{`/usr/local/bin/git commit -m x`, true},
		{`./bin/git commit`, true},
		{`/usr/bin/git -C sub commit`, true},

		// Combinations across capabilities.
		{`FOO=1 bash -c "git commit -m x"`, true},      // env-assign + shell -c
		{`FOO=1 command git commit -m x`, true},        // env-assign + command
		{`command /usr/bin/git commit`, true},          // command + abs git path
		{`timeout 30 bash -c "git commit -m x"`, true}, // wrapper + shell -c

		// Negative controls — must NOT match.
		{`git status`, false},
		{`bash -c "echo hi"`, false},        // shell -c, but no git commit inside
		{`sh -c 'npm test'`, false},         // shell -c into a non-git command
		{`bash -c "git status"`, false},     // recursion into a non-commit subcommand
		{`command npm test`, false},         // command builtin, non-git
		{`command -v git`, false},           // -v/git alone: no commit subcommand
		{`/usr/bin/git status`, false},      // abs path, but not the commit subcommand
		{`mygit commit`, false},             // basename "mygit" != "git"
		{`/opt/notgit/mygit commit`, false}, // basename still "mygit"
	}
	for _, c := range cases {
		if got := InvokesGitCommit(c.cmd); got != c.want {
			t.Errorf("InvokesGitCommit(%q) = %v; want %v", c.cmd, got, c.want)
		}
	}
}
