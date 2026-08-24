// json_agent_body_test.go — covers the JSON body written to
// .sourcegraph/cody.json and .zed/settings.json.
//
// This surface had no test at all. internal/agents/agents_test.go pins the
// registry (the filename each agent gets, and that both are IsJSON), but
// nothing looked at what is actually inside the file — so the body could,
// and did, keep pointing two AI tools at docs/decisions.md as the place to
// find "recent decisions" long after SPEC §3.2 made that the one file the
// branch-aware write path never writes.
package inserter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/agents"
)

// TestCreateAgentFile_JSONAgentsPointAtTheBranchAwareLayout drives the real
// CreateAgentFile write path for both JSON agents and asserts on the file
// left on disk — not on jsonAgentBody's return value, which a caller could
// stop using without this test noticing.
//
// It checks three things the previous body got wrong:
//
//  1. The file parses as JSON. It is a config two editors load; a body that
//     is merely a plausible-looking string is worthless to them.
//  2. context_files names the branch-aware layout — the timeline (both
//     halves) and docs/decisions-branches/ — so an agent reading this
//     config is pointed at where decisions actually live.
//  3. docs/decisions.md is still listed, and listed LAST. It is
//     read-where-it-exists legacy: dropping it would lose a pre-§3.2 repo's
//     history from these tools, and promoting it is the bug this fixes.
func TestCreateAgentFile_JSONAgentsPointAtTheBranchAwareLayout(t *testing.T) {
	// The two JSON agents are looked up from the registry rather than
	// hardcoded, so an agent added as IsJSON later is covered here the day
	// it is added instead of silently shipping an untested body.
	var jsonAgents []string
	for _, name := range agents.Names() {
		if agents.IsJSON(name) {
			jsonAgents = append(jsonAgents, name)
		}
	}
	if len(jsonAgents) == 0 {
		t.Fatal("no IsJSON agents in the registry — this test would pass vacuously; the registry or agents.IsJSON changed shape")
	}

	for _, name := range jsonAgents {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			written, _, err := CreateAgentFile(name, root)
			if err != nil {
				t.Fatalf("CreateAgentFile(%s): %v", name, err)
			}
			path := written.Path
			if path == "" {
				t.Fatalf("CreateAgentFile(%s) returned no path for a registered agent", name)
			}
			// The registry owns the filename; assert we wrote where it says.
			a, ok := agents.Lookup(name)
			if !ok {
				t.Fatalf("agents.Lookup(%s) failed for a name that came from the registry", name)
			}
			if want := filepath.Join(root, filepath.FromSlash(a.FilePattern)); path != want {
				t.Fatalf("wrote %s; registry says %s", path, want)
			}

			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}

			var parsed struct {
				Logmind struct {
					Enabled      bool     `json:"enabled"`
					Description  string   `json:"description"`
					ContextFiles []string `json:"context_files"`
				} `json:"logmind"`
			}
			if err := json.Unmarshal(raw, &parsed); err != nil {
				t.Fatalf("%s is not valid JSON (%v); body:\n%s", a.FilePattern, err, raw)
			}
			if !parsed.Logmind.Enabled {
				t.Errorf("logmind.enabled is false in %s", a.FilePattern)
			}

			got := parsed.Logmind.ContextFiles
			index := func(want string) int {
				for i, v := range got {
					if v == want {
						return i
					}
				}
				return -1
			}

			// The branch-aware layout must be present.
			for _, want := range []string{
				"docs/timeline.md",
				"docs/timeline-archive.md",
				"docs/decisions-branches/",
				"docs/file-structure.md",
			} {
				if index(want) < 0 {
					t.Errorf("context_files in %s omits %q — an agent loading this config never sees it; got %v",
						a.FilePattern, want, got)
				}
			}

			// Legacy stays, but last: read-where-it-exists, never the
			// headline. Before §3.2 it was first and docs/decisions-branches/
			// was absent entirely.
			legacy := index("docs/decisions.md")
			if legacy < 0 {
				t.Errorf("context_files in %s dropped docs/decisions.md — a repo that predates §3.2 keeps its history there and it must stay readable; got %v",
					a.FilePattern, got)
			} else if legacy != len(got)-1 {
				t.Errorf("docs/decisions.md is at position %d of %d in %s; it must come LAST — it is legacy, not the place decisions are written; got %v",
					legacy, len(got), a.FilePattern, got)
			}

			// The prose an agent reads must not send it to the legacy file
			// for "recent decisions" — that was the shipped bug.
			desc := parsed.Logmind.Description
			if desc == "" {
				t.Fatalf("logmind.description is empty in %s", a.FilePattern)
			}
			for _, want := range []string{"docs/decisions-branches/", "docs/timeline.md"} {
				if !strings.Contains(desc, want) {
					t.Errorf("description in %s never mentions %q; it reads: %q", a.FilePattern, want, desc)
				}
			}
			if strings.Contains(desc, "See docs/decisions.md for recent decisions") {
				t.Errorf("description in %s still points at the legacy file for recent decisions — §3.2's branch-aware path never writes it: %q",
					a.FilePattern, desc)
			}
		})
	}
}
