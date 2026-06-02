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

// TestAgentsTemplate_HasV5Marker pins the protocol-version marker. The
// `agents update --apply` workflow keys on this marker to decide
// whether an installed block is stale.
func TestAgentsTemplate_HasV5Marker(t *testing.T) {
	body := AgentsTemplate()
	if !strings.Contains(body, "<!-- logmind-block-version: v5 -->") {
		t.Fatalf("full template missing v5 marker")
	}
	if !strings.Contains(body, "<!-- logmind-start -->") || !strings.Contains(body, "<!-- logmind-end -->") {
		t.Fatalf("full template missing start/end markers")
	}
}

// TestAgentsSlimTemplate_HasV7PointerMarker pins the slim variant
// marker so the byte-identical-rewrite path can never confuse the two
// templates' marker versions.
func TestAgentsSlimTemplate_HasV7PointerMarker(t *testing.T) {
	body := AgentsSlimTemplate()
	if !strings.Contains(body, "<!-- logmind-block-version: v7-pointer -->") {
		t.Fatalf("slim template missing v7-pointer marker")
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
