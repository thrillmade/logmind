// config_blocking_test.go — SPEC §1.6's refusal: `logmind config set` must
// not weaken git.enforce_commits / review.strict_mode / review.auto_fix on
// behalf of an agent, must still write every other setting, and must let a
// person write all three.
//
// The over-blocking controls are as load-bearing as the refusals. §1.6's next
// paragraph — "Every other setting an agent MAY write through the command,
// because setup and registering a skill it just wrote are legitimate work and
// blocking them helps nobody" — is the failure mode a careless guard causes,
// so every refusal case here has a permitted-key twin.
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runConfigCmd fires `logmind config ...` with a caller-supplied stdin and
// returns (stdout+stderr, error). Unlike runCommand it tolerates a failure —
// the refusal path is the point.
func runConfigCmd(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs(args)
	var sink bytes.Buffer
	root.SetOut(&sink)
	root.SetErr(&sink)
	root.SetIn(strings.NewReader(stdin))
	err := root.Execute()
	return sink.String(), err
}

// inFakeTTYRepo runs fn in a fresh temp cwd with isTerminalFunc pinned, and
// returns the temp dir so the caller can inspect (or assert the absence of)
// .logmind/config.yml afterwards.
func inFakeTTYRepo(t *testing.T, asTTY bool, fn func()) string {
	t.Helper()
	var dir string
	withFakeTTY(t, asTTY, func() {
		dir = withTempCwd(t, func(d string) { fn() })
	})
	return dir
}

// configFileBody returns .logmind/config.yml under dir, or "" when the
// command never created it.
func configFileBody(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".logmind", "config.yml"))
	if err != nil {
		return ""
	}
	return string(data)
}

// clearAgentEnv unsets every marker for the duration of the test, so a real
// agent harness running `go test` does not turn the person-path tests red.
// t.Setenv first so the original value is restored on cleanup.
func clearAgentEnv(t *testing.T) {
	t.Helper()
	for _, marker := range agentEnvMarkers {
		t.Setenv(marker, "")
		_ = os.Unsetenv(marker)
	}
}

// TestConfigSet_AgentWeakeningRefused is the issue-#330 regression: all three
// §1.6 keys, weakened, by an agent. Each must exit non-zero, say why, name who
// may change it, and leave nothing on disk.
func TestConfigSet_AgentWeakeningRefused(t *testing.T) {
	cases := []struct {
		key   string
		value string
		// governs is the clause the refusal must carry so the reader
		// learns what they would be turning off.
		governs string
	}{
		{"git.enforce_commits", "false", "whether a substantive commit must carry a decision"},
		{"review.strict_mode", "false", "whether a critical review finding blocks the merge"},
		{"review.auto_fix", "true", "whether a reviewer may push a fix without a person"},
	}
	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			clearAgentEnv(t)
			t.Setenv("CLAUDECODE", "1")
			// A TTY and a stdin already carrying the confirmation: the
			// marker must win anyway, or the guard is a formality.
			dir := inFakeTTYRepo(t, true, func() {
				out, err := runConfigCmd(t, c.key+"\n", "config", "set", c.key, c.value)
				if err == nil {
					t.Fatalf("config set %s %s: want refusal error, got nil\n%s", c.key, c.value, out)
				}
				mustContain(t, out, "Refusing to write "+c.key)
				mustContain(t, out, c.governs)
				mustContain(t, out, "a person")
				mustContain(t, out, "CLAUDECODE")
				mustNotContain(t, out, "Set "+c.key)
			})
			if body := configFileBody(t, dir); body != "" {
				t.Errorf("refused write still created .logmind/config.yml:\n%s", body)
			}
		})
	}
}

// TestConfigSet_AgentPermittedKeysStillWritten is the over-blocking control.
// Same environment as the refusals; these must sail through.
func TestConfigSet_AgentPermittedKeysStillWritten(t *testing.T) {
	cases := []struct{ key, value, wantInFile string }{
		{"git.commit_line_threshold", "30", "commit_line_threshold: 30"},
		{"git.auto_push", "false", "auto_push: false"},
		{"agents.claude", "false", "claude: false"},
		{"context.spec_file", "docs/spec.md", "spec_file: docs/spec.md"},
		// Near misses on the protected keys — a prefix or substring match
		// would block these, and they are ordinary settings.
		{"git.enforce_commits_note", "hi", "enforce_commits_note: hi"},
		{"review.trigger", "pre-push", "trigger: pre-push"},
	}
	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			clearAgentEnv(t)
			t.Setenv("CLAUDECODE", "1")
			dir := inFakeTTYRepo(t, false, func() {
				out, err := runConfigCmd(t, "", "config", "set", c.key, c.value)
				if err != nil {
					t.Fatalf("config set %s %s: %v\n%s", c.key, c.value, err, out)
				}
				mustContain(t, out, "Set "+c.key+" = ")
			})
			mustContain(t, configFileBody(t, dir), c.wantInFile)
		})
	}
}

// TestConfigSet_AgentMayStrengthen is ruling 4: §1.6 says "asked to weaken",
// so writing the value that keeps a person in the loop is allowed. Refusing
// `enforce_commits true` would block the setup §1.6 protects.
func TestConfigSet_AgentMayStrengthen(t *testing.T) {
	cases := []struct{ key, value, wantInFile string }{
		{"git.enforce_commits", "true", "enforce_commits: true"},
		{"review.strict_mode", "true", "strict_mode: true"},
		{"review.auto_fix", "false", "auto_fix: false"},
	}
	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			clearAgentEnv(t)
			t.Setenv("CLAUDECODE", "1")
			dir := inFakeTTYRepo(t, false, func() {
				out, err := runConfigCmd(t, "", "config", "set", c.key, c.value)
				if err != nil {
					t.Fatalf("config set %s %s: %v\n%s", c.key, c.value, err, out)
				}
				mustContain(t, out, "Set "+c.key+" = ")
			})
			mustContain(t, configFileBody(t, dir), c.wantInFile)
		})
	}
}

// TestConfigSet_PersonMayWeakenAllThree — a person at a terminal who retypes
// the key gets the write. §1.6 makes these settings theirs; a guard that
// blocked them would be a different bug.
func TestConfigSet_PersonMayWeakenAllThree(t *testing.T) {
	cases := []struct{ key, value, wantInFile string }{
		{"git.enforce_commits", "false", "enforce_commits: false"},
		{"review.strict_mode", "false", "strict_mode: false"},
		{"review.auto_fix", "true", "auto_fix: true"},
	}
	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			clearAgentEnv(t)
			dir := inFakeTTYRepo(t, true, func() {
				out, err := runConfigCmd(t, c.key+"\n", "config", "set", c.key, c.value)
				if err != nil {
					t.Fatalf("config set %s %s as a person: %v\n%s", c.key, c.value, err, out)
				}
				mustContain(t, out, "Set "+c.key+" = ")
			})
			mustContain(t, configFileBody(t, dir), c.wantInFile)
		})
	}
}

// A person who does not retype the key — or types something else — is not
// confirmed, and nothing is written. "y" is on the list on purpose: it is what
// `logmind agents remove` accepts, and what an autoresponder pipes.
func TestConfigSet_PersonWhoDoesNotRetypeTheKeyIsRefused(t *testing.T) {
	for _, reply := range []string{"y", "yes", "", "git.enforce_commit", "GIT.ENFORCE_COMMITS"} {
		t.Run("reply="+reply, func(t *testing.T) {
			clearAgentEnv(t)
			dir := inFakeTTYRepo(t, true, func() {
				out, err := runConfigCmd(t, reply+"\n", "config", "set", "git.enforce_commits", "false")
				if err == nil {
					t.Fatalf("reply %q: want refusal, got nil\n%s", reply, out)
				}
				mustContain(t, out, "Not confirmed")
			})
			if body := configFileBody(t, dir); body != "" {
				t.Errorf("unconfirmed write still wrote:\n%s", body)
			}
		})
	}
}

// No agent marker, no terminal — a script, a workflow, a cron. logmind cannot
// put the question to a person, so it refuses and says that is the reason.
func TestConfigSet_NoTerminalRefusedWithItsOwnReason(t *testing.T) {
	clearAgentEnv(t)
	dir := inFakeTTYRepo(t, false, func() {
		out, err := runConfigCmd(t, "git.enforce_commits\n", "config", "set", "git.enforce_commits", "false")
		if err == nil {
			t.Fatalf("want refusal without a terminal, got nil\n%s", out)
		}
		mustContain(t, out, "Refusing to write git.enforce_commits")
		mustContain(t, out, "not a terminal")
	})
	if body := configFileBody(t, dir); body != "" {
		t.Errorf("refused write still wrote:\n%s", body)
	}
}

// A non-bool coerced into one of these keys is a weakening too — `0` is not
// `true`, and it also makes the typed load fall back to the whole default
// config (see config.BlockingSetting.Weakens).
func TestConfigSet_NonBoolIntoBlockingKeyRefused(t *testing.T) {
	for _, value := range []string{"0", "1", "no", "off"} {
		t.Run("value="+value, func(t *testing.T) {
			clearAgentEnv(t)
			t.Setenv("CLAUDECODE", "1")
			dir := inFakeTTYRepo(t, false, func() {
				out, err := runConfigCmd(t, "", "config", "set", "git.enforce_commits", value)
				if err == nil {
					t.Fatalf("config set git.enforce_commits %s: want refusal, got nil\n%s", value, out)
				}
			})
			if body := configFileBody(t, dir); body != "" {
				t.Errorf("refused write still wrote:\n%s", body)
			}
		})
	}
}

// Every marker in the list identifies an agent, and the refusal names the one
// it saw. A marker added without a row here would go untested.
func TestConfigSet_EveryAgentMarkerRefuses(t *testing.T) {
	for _, marker := range agentEnvMarkers {
		t.Run(marker, func(t *testing.T) {
			clearAgentEnv(t)
			t.Setenv(marker, "1")
			dir := inFakeTTYRepo(t, true, func() {
				out, err := runConfigCmd(t, "git.enforce_commits\n", "config", "set", "git.enforce_commits", "false")
				if err == nil {
					t.Fatalf("marker %s: want refusal, got nil\n%s", marker, out)
				}
				mustContain(t, out, marker)
			})
			if body := configFileBody(t, dir); body != "" {
				t.Errorf("marker %s: refused write still wrote:\n%s", marker, body)
			}
		})
	}
}

// An empty marker is not a marker — `CLAUDECODE=` exported by a wrapper script
// must not make a person look like an agent.
func TestConfigSet_EmptyMarkerIsNotAnAgent(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("CLAUDECODE", "")
	dir := inFakeTTYRepo(t, true, func() {
		out, err := runConfigCmd(t, "git.enforce_commits\n", "config", "set", "git.enforce_commits", "false")
		if err != nil {
			t.Fatalf("empty marker must not read as an agent: %v\n%s", err, out)
		}
	})
	mustContain(t, configFileBody(t, dir), "enforce_commits: false")
}

// A refused write must not disturb a config file that was already there.
func TestConfigSet_RefusedWriteLeavesExistingFileByteIdentical(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("CLAUDECODE", "1")
	var before, after string
	inFakeTTYRepo(t, false, func() {
		// An allowed write first, so there IS a file to preserve.
		if out, err := runConfigCmd(t, "", "config", "set", "git.commit_line_threshold", "30"); err != nil {
			t.Fatalf("seed write: %v\n%s", err, out)
		}
		data, err := os.ReadFile(filepath.Join(".logmind", "config.yml"))
		if err != nil {
			t.Fatalf("read seeded config: %v", err)
		}
		before = string(data)
		if _, err := runConfigCmd(t, "", "config", "set", "git.enforce_commits", "false"); err == nil {
			t.Fatalf("want refusal, got nil")
		}
		data, err = os.ReadFile(filepath.Join(".logmind", "config.yml"))
		if err != nil {
			t.Fatalf("read config after refusal: %v", err)
		}
		after = string(data)
	})
	if before != after {
		t.Errorf("refused write rewrote the file\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}
}
