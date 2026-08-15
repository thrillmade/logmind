// decisions_uncapped_test.go — SPEC §3.2: "Every file is append-only and
// uncapped. Nothing rotates, nothing overflows, and nothing is archived: a
// decision written is a decision kept."
//
// This file replaces decisions_rotate_test.go, which pinned the capacity
// rotation §3.2 removed. It pins the inverse, on observable output: after a
// run of `logmind log` calls that would have overflowed the old cap several
// times over, every entry is still in the branch file, in order, and no
// docs/decisions-archive.md exists.
//
// The `decisions.max_recent` key is left in the config on purpose. It no
// longer exists in the schema, and §1.6 requires an unrecognised key to be
// ignored rather than fail — so a repo that upgrades its binary before it
// tidies its config must keep working, and the key must cap nothing.
package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/decisions"
)

// TestLog_DecisionFileIsUncapped_EndToEnd logs 5 decisions into one branch
// file with `decisions.max_recent: 2` still present in .logmind/config.yml.
// Under the old rule that would have peeled 3 entries into
// docs/decisions-archive.md. All 5 must survive, in order, and no archive
// may be created.
func TestLog_DecisionFileIsUncapped_EndToEnd(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		// A stale key from before §3.2 removed the cap. Loading this config
		// must not error (§1.6: an unrecognised key MUST NOT cause a
		// failure) and must not cap anything.
		writeConfig(t, d, "decisions:\n  max_recent: 2\n  branch_aware: true\n")
		commitAll(t, d, "initial")

		const n = 5
		for i := 1; i <= n; i++ {
			var out bytes.Buffer
			withFakeTTY(t, false, func() {
				root := NewRootCmd()
				root.SetArgs([]string{"log", fmt.Sprintf("Decision %d", i),
					"-r", "why", "--no-commit", "--no-interactive"})
				root.SetOut(&out)
				root.SetErr(&out)
				if err := root.Execute(); err != nil {
					t.Fatalf("log %d: %v\n%s", i, err, out.String())
				}
			})
			mustContain(t, out.String(), fmt.Sprintf(`✓ Logged decision: "Decision %d"`, i))
		}

		// The observable result: the file `logmind log` wrote holds every
		// entry, oldest first.
		target := filepath.Join(d, "docs", "decisions-branches", "main.md")
		body, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read %s: %v", target, err)
		}
		_, entries := decisions.SplitRawBytes(string(body))
		var got []string
		for _, e := range entries {
			got = append(got, e.Title)
		}
		want := []string{
			// `logmind init` logs the repo's first decision into the same
			// file; it is the oldest entry and must survive too.
			firstDecisionTitle,
			"Decision 1", "Decision 2", "Decision 3", "Decision 4", "Decision 5",
		}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("branch file entries = %v; want %v — a decision written is a decision kept", got, want)
		}

		// Nothing is archived: the file must not even come into existence.
		if _, err := os.Stat(filepath.Join(d, "docs", "decisions-archive.md")); err == nil {
			t.Fatalf("docs/decisions-archive.md was created — nothing is archived under §3.2")
		}
	})
}

// TestLoad_UnknownMaxRecentKeyIsIgnored is the direct check of the §1.6
// lenient-read rule for the key this change removed: a config carrying
// `decisions.max_recent` loads without error, and the rest of the section
// still resolves.
func TestLoad_UnknownMaxRecentKeyIsIgnored(t *testing.T) {
	withTempCwd(t, func(d string) {
		writeConfig(t, d, "decisions:\n  max_recent: 2\n  branch_aware: false\n")
		root := NewRootCmd()
		root.SetArgs([]string{"config", "get", "decisions.branch_aware"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("config get with a stale max_recent key errored: %v\n%s", err, out.String())
		}
		if got := strings.TrimSpace(out.String()); got != "False" {
			t.Errorf("branch_aware = %q; want False — the neighbouring key must still resolve", got)
		}
	})
}
