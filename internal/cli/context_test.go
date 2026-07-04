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
		if !strings.Contains(s, `source="docs/file-structure.md" status="absent"`) || !strings.Contains(s, "regenerate=") {
			t.Errorf("absent file-structure not noted as a self-closing element:\n%s", s)
		}
		if strings.Contains(s, "<source>docs/file-structure.md</source>") {
			t.Errorf("an absent doc must not emit a full <document>…<source>…</document> block:\n%s", s)
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

// TestContext_Repomap_DisabledByDefault: without the context.repomap opt-in,
// the payload carries no repomap document (default stays byte-stable).
func TestContext_Repomap_DisabledByDefault(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, "docs/file-structure.md", "FS\n")
		mustWriteUnder(t, d, "docs/timeline.md", "TL\n")
		mustWriteUnder(t, d, "svc.go", "package p\nfunc Serve() error { return nil }\n")
		s := runContextCapture(t)
		if strings.Contains(s, `type="repomap"`) {
			t.Errorf("repomap doc present without opt-in (breaks byte-parity):\n%s", s)
		}
	})
}

// TestContext_Repomap_Enabled_StableFirst: context.repomap:true folds the
// signature skeleton in as a <document type="repomap"> placed stable-first
// (after file-structure, before the volatile timeline).
func TestContext_Repomap_Enabled_StableFirst(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, "docs/file-structure.md", "FS\n")
		mustWriteUnder(t, d, "docs/timeline.md", "TL\n")
		mustWriteUnder(t, d, "svc.go", "package p\nfunc Serve() error { return nil }\n")
		mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  repomap: true\n")
		s := runContextCapture(t)
		if !strings.Contains(s, `<document type="repomap">`) {
			t.Fatalf("repomap doc missing when enabled:\n%s", s)
		}
		if !strings.Contains(s, "func Serve() error") {
			t.Errorf("repomap body missing the signature:\n%s", s)
		}
		iFS := strings.Index(s, `type="file-structure"`)
		iRM := strings.Index(s, `type="repomap"`)
		iTL := strings.Index(s, `type="decision-timeline"`)
		if !(iFS < iRM && iRM < iTL) {
			t.Errorf("repomap not stable-first (fs=%d repomap=%d tl=%d):\n%s", iFS, iRM, iTL, s)
		}
	})
}

// TestContext_Repomap_EnabledNoGo_Omitted: enabled but no Go symbols → the
// repomap document is omitted cleanly (never an empty envelope).
func TestContext_Repomap_EnabledNoGo_Omitted(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, "docs/file-structure.md", "FS\n")
		mustWriteUnder(t, d, "docs/timeline.md", "TL\n")
		mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  repomap: true\n")
		s := runContextCapture(t) // no .go files
		if strings.Contains(s, `type="repomap"`) {
			t.Errorf("empty repomap should be omitted, not an empty envelope:\n%s", s)
		}
	})
}

// TestContext_Repomap_Stats: --stats breaks out the repomap term when enabled.
func TestContext_Repomap_Stats(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, "docs/file-structure.md", "FS\n")
		mustWriteUnder(t, d, "docs/timeline.md", "TL\n")
		mustWriteUnder(t, d, "svc.go", "package p\nfunc Serve() error { return nil }\n")
		mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  repomap: true\n")
		s := runContextCapture(t, "--stats")
		if !strings.Contains(s, "+ repomap ") {
			t.Errorf("--stats missing the repomap term:\n%s", s)
		}
	})
}
