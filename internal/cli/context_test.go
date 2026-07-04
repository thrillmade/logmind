package cli

import (
	"bytes"
	"strings"
	"testing"
)

func runContextCapture(t *testing.T, args ...string) string {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs(append([]string{"context"}, args...))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("context %v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

// TestContext_XMLEnvelope_StableFirst: the payload wraps both docs in the
// <document><source><document_content> envelope, with the stable file-structure
// BEFORE the volatile (newest-first) timeline — the cache-prefix ordering.
func TestContext_XMLEnvelope_StableFirst(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, "docs/file-structure.md", "FSMARKER\n")
		mustWriteUnder(t, d, "docs/timeline.md", "TLMARKER\n")
		s := runContextCapture(t)
		for _, want := range []string{
			"<repo_context>", `<document type="file-structure">`,
			`<document type="decision-timeline">`, "<source>docs/file-structure.md</source>",
			"FSMARKER", "TLMARKER", "</repo_context>",
		} {
			if !strings.Contains(s, want) {
				t.Errorf("payload missing %q:\n%s", want, s)
			}
		}
		// Stable-first: file-structure precedes timeline (cache-prefix stability).
		if strings.Index(s, "FSMARKER") > strings.Index(s, "TLMARKER") {
			t.Errorf("file-structure must precede timeline (cache-stable ordering):\n%s", s)
		}
	})
}

// TestContext_Deterministic: byte-identical across runs — the property prompt
// caching depends on (a single changed byte busts the whole cache prefix).
func TestContext_Deterministic(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, "docs/file-structure.md", "tree\n")
		mustWriteUnder(t, d, "docs/timeline.md", "why\n")
		a := runContextCapture(t)
		b := runContextCapture(t)
		if a != b {
			t.Errorf("context not byte-deterministic (breaks prompt caching):\n--- a ---\n%s\n--- b ---\n%s", a, b)
		}
	})
}

// TestContext_MissingDocNoted_NotError: a missing derived doc is noted as a
// comment and omitted from the <document> set, never an error.
func TestContext_MissingDocNoted_NotError(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, "docs/timeline.md", "why\n")
		// docs/file-structure.md intentionally absent.
		s := runContextCapture(t)
		if !strings.Contains(s, "docs/file-structure.md absent") || !strings.Contains(s, "regenerate:") {
			t.Errorf("missing doc not noted as a comment:\n%s", s)
		}
		if strings.Contains(s, `<document type="file-structure">`) {
			t.Errorf("an absent doc must not emit a <document> block:\n%s", s)
		}
	})
}

// TestContext_Stats: --stats prints the token receipt, not the payload.
func TestContext_Stats(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, "docs/file-structure.md", "tree\n")
		mustWriteUnder(t, d, "docs/timeline.md", "why\n")
		mustWriteUnder(t, d, "docs/decisions.md", "## 2026-01-01 12:00 - X\n\nrationale\n")
		s := runContextCapture(t, "--stats")
		if !strings.Contains(s, "token receipt") || !strings.Contains(s, "payload total:") {
			t.Errorf("--stats missing the receipt:\n%s", s)
		}
		if strings.Contains(s, "<repo_context>") {
			t.Errorf("--stats must not print the payload:\n%s", s)
		}
	})
}
