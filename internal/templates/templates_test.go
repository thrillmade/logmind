package templates

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestTemplates_ByteIdenticalToPython is the load-bearing parity gate:
// it diffs each embedded template byte-for-byte against the Python
// source-of-truth at src/logmind/templates/<name>. Any drift between
// the two trees fails the test immediately.
//
// Skipped when the Python tree isn't reachable (e.g., running tests
// from an extracted release tarball) — in that case we degrade to the
// other tests in this file which only validate marker presence.
func TestTemplates_ByteIdenticalToPython(t *testing.T) {
	pyDir := pythonTemplatesDir(t)
	if pyDir == "" {
		t.Skip("Python templates dir not reachable; skipping parity diff")
	}

	cases := []struct {
		name     string
		embedded string
	}{
		{"AGENTS.md.template", AgentsTemplate()},
		{"AGENTS.md.slim.template", AgentsSlimTemplate()},
		{"agent-stub.md", Stub()},
		{"logmind-section.md", LogmindSection()},
		{"CLAUDE.md.template", FullClaudeTemplate()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			py, err := os.ReadFile(filepath.Join(pyDir, tc.name))
			if err != nil {
				t.Fatalf("read python source %s: %v", tc.name, err)
			}
			if string(py) != tc.embedded {
				t.Fatalf("%s drift between Go embed and Python source\n--- want (python) ---\n%s\n--- got (embed) ---\n%s",
					tc.name, py, tc.embedded)
			}
		})
	}
}

// TestAgentsTemplate_HasV6Marker pins the protocol-version marker. The
// `agents update --apply` workflow keys on this marker to decide
// whether an installed block is stale.
//
// v0.6.16 bumped v5→v6: heading reframed as "REQUIRED for substantive
// commits" with an explicit DO-NOT-git-commit blockquote that pairs
// with the commit-msg hook installed by `logmind init`.
func TestAgentsTemplate_HasV6Marker(t *testing.T) {
	body := AgentsTemplate()
	if !strings.Contains(body, "<!-- logmind-block-version: v6 -->") {
		t.Fatalf("full template missing v6 marker")
	}
	if !strings.Contains(body, "<!-- logmind-start -->") || !strings.Contains(body, "<!-- logmind-end -->") {
		t.Fatalf("full template missing start/end markers")
	}
	// v0.6.16: the REQUIRED framing + DO-NOT blockquote are the
	// user-visible delta over v5. Pin both so a future inadvertent
	// revert to the v5 prose trips this test.
	if !strings.Contains(body, "REQUIRED for substantive commits") {
		t.Fatalf("v6 template missing REQUIRED framing in heading")
	}
	if !strings.Contains(body, "DO NOT run `git add` / `git commit` / `git push`") {
		t.Fatalf("v6 template missing DO-NOT-git-commit blockquote")
	}
}

// TestAgentsSlimTemplate_HasV8PointerMarker pins the slim variant
// marker so the byte-identical-rewrite path can never confuse the two
// templates' marker versions.
//
// v0.6.16 bumped v7-pointer→v8-pointer: heading reframed as
// "REQUIRED for substantive commits" + DO-NOT-git-commit blockquote
// paired with the commit-msg hook.
func TestAgentsSlimTemplate_HasV8PointerMarker(t *testing.T) {
	body := AgentsSlimTemplate()
	if !strings.Contains(body, "<!-- logmind-block-version: v8-pointer -->") {
		t.Fatalf("slim template missing v8-pointer marker")
	}
	if !strings.Contains(body, "REQUIRED for substantive commits") {
		t.Fatalf("v8-pointer template missing REQUIRED framing in heading")
	}
	if !strings.Contains(body, "DO NOT run raw `git add` / `git commit` / `git push`") {
		t.Fatalf("v8-pointer template missing DO-NOT-git-commit blockquote")
	}
}

// TestStub_HasStubMarker confirms the stub identifier matches the
// LOGMIND_STUB_MARKER constant used by the inserter package.
func TestStub_HasStubMarker(t *testing.T) {
	body := Stub()
	if !strings.Contains(body, "<!-- logmind-stub:") {
		t.Fatalf("stub template missing stub marker")
	}
}

// TestDependabotTemplate_HasMarkerAndShape pins the v1.1.0 dependabot
// template content — the marker line, the `github-actions` ecosystem,
// and the thrillmade group with the `thrillmade/*` pattern. Any of
// those drifting would break the merge-detection regex in
// inserter/dependabot.go, so the test pins them together.
func TestDependabotTemplate_HasMarkerAndShape(t *testing.T) {
	body := DependabotTemplate()
	if !strings.Contains(body, "logmind-dependabot-marker: v1") {
		t.Fatalf("dependabot template missing v1 marker")
	}
	if !strings.Contains(body, `package-ecosystem: "github-actions"`) {
		t.Fatalf("dependabot template missing github-actions ecosystem")
	}
	if !strings.Contains(body, "thrillmade:") {
		t.Fatalf("dependabot template missing thrillmade group key")
	}
	if !strings.Contains(body, `- "thrillmade/*"`) {
		t.Fatalf("dependabot template missing thrillmade/* pattern")
	}
	if !strings.Contains(body, `interval: "daily"`) {
		t.Fatalf("dependabot template should default to daily — v1.1.0 design point so action pin tracks fresh logmind releases without manual ceremony")
	}
}

// TestWorkflowTemplates_UseSetupLogmindAction pins the v1.1.0
// distribution-lock change: workflow templates must use the new
// `thrillmade/setup-logmind@vX.Y.Z` action and NOT the legacy
// `pip install logmind==X.Y.Z` line. Both check-doc-links and
// regen-timeline are user-facing CI workflows under
// `# logmind-template-version:` markers; the self-update workflow
// uses the action too but is allowed to grep for the legacy pin (so
// pre-v1.1.0 installs can upgrade once). We only assert the consumer-
// CI workflows here.
func TestWorkflowTemplates_UseSetupLogmindAction(t *testing.T) {
	for _, name := range []string{
		"check-doc-links.yml.template",
		"regen-timeline.yml.template",
	} {
		body := Workflow(name)
		if !strings.Contains(body, "thrillmade/setup-logmind@v") {
			t.Errorf("%s missing setup-logmind action ref", name)
		}
		// Legacy curl-install / pip-install patterns must not appear
		// in the consumer-CI templates. They were replaced by the
		// action in v1.1.0 per the 2026-06-05 distribution lock.
		if strings.Contains(body, "pip install \"logmind==") || strings.Contains(body, "actions/setup-python") {
			t.Errorf("%s still contains the pre-v1.1.0 pip install / setup-python pattern", name)
		}
	}
}

// pythonTemplatesDir walks up from the running test file's location
// to find the repo root, then returns src/logmind/templates/. Returns
// "" when the Python tree is absent.
func pythonTemplatesDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed")
	}
	dir := filepath.Dir(thisFile)
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "src", "logmind", "templates")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
