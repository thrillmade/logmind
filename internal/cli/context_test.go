package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestContext_PrintsBothDocs: `logmind context` concatenates the two
// derived docs (why + what) in order, with headings, in one read.
func TestContext_PrintsBothDocs(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, "docs/timeline.md", "TIMELINE_CONTENT_MARKER\n")
		mustWriteUnder(t, d, "docs/file-structure.md", "FILESTRUCTURE_CONTENT_MARKER\n")
		root := NewRootCmd()
		root.SetArgs([]string{"context"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("context: %v\n%s", err, out.String())
		}
		s := out.String()
		if !strings.Contains(s, "TIMELINE_CONTENT_MARKER") || !strings.Contains(s, "FILESTRUCTURE_CONTENT_MARKER") {
			t.Errorf("context missing a doc's content:\n%s", s)
		}
		// Order: the why (timeline) before the what (file-structure).
		iT := strings.Index(s, "docs/timeline.md")
		iF := strings.Index(s, "docs/file-structure.md")
		if iT < 0 || iF < 0 || iT > iF {
			t.Errorf("sections missing or out of order (timeline before file-structure):\n%s", s)
		}
	})
}

// TestContext_MissingDocNoted_NotError: a missing derived doc is noted
// with a regenerate hint, never an error (context is read-only convenience).
func TestContext_MissingDocNoted_NotError(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, "docs/timeline.md", "TL\n")
		// docs/file-structure.md intentionally absent.
		root := NewRootCmd()
		root.SetArgs([]string{"context"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("context must not error on a missing doc: %v", err)
		}
		s := out.String()
		if !strings.Contains(s, "docs/file-structure.md not found") || !strings.Contains(s, "regenerate with") {
			t.Errorf("missing doc not noted with a regen hint:\n%s", s)
		}
	})
}
