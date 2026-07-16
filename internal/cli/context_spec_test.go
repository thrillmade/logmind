package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// specFixture seeds the minimal docs/ pair every context test in this file
// needs. Deterministic content so the golden captures stay stable.
func specFixture(t *testing.T, d string) {
	t.Helper()
	mustWriteUnder(t, d, "docs/file-structure.md", "FSTREE\n")
	mustWriteUnder(t, d, "docs/timeline.md", "TLBODY\n")
}

// checkGoldenIn is checkGolden's shape but resolves testdata/ under an
// explicit baseDir rather than the process cwd. Needed here because
// withTempCwd chdirs into a t.TempDir() for the life of the whole test (the
// restore is a t.Cleanup, which fires after the test body returns) — a
// plain checkGolden call made after withTempCwd returns would still resolve
// "testdata/..." against the temp fixture dir, not the package's real
// testdata/.
func checkGoldenIn(t *testing.T, baseDir, name, got string) {
	t.Helper()
	path := filepath.Join(baseDir, "testdata", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %s", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `make snapshot` to create it)", path, err)
	}
	if string(want) != got {
		t.Fatalf("drift vs %s\n--- want ---\n%s\n--- got ---\n%s", path, string(want), got)
	}
}

// TestContext_SpecUnset_ByteParityToMain is the load-bearing parity gate for
// H1.5: with context.spec_file unset (the default — no .logmind/config.yml
// at all here), `logmind context` must be BYTE-IDENTICAL to the payload
// main produced before the canonical-spec-file feature existed. The golden
// fixture was captured by running this exact test with `-update` against
// the pre-feature code (see the PR description) — a live drift here means
// the fold-in broke the default (disabled) path, not just a stale golden.
func TestContext_SpecUnset_ByteParityToMain(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	var got string
	withTempCwd(t, func(d string) {
		specFixture(t, d)
		got = runContextCapture(t)
	})
	checkGoldenIn(t, origWD, "context_spec_unset.golden", got)
}

// TestContextStats_SpecUnset_ByteParityToMain is the --stats counterpart —
// same parity guarantee for the token receipt.
func TestContextStats_SpecUnset_ByteParityToMain(t *testing.T) {
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	var got string
	withTempCwd(t, func(d string) {
		specFixture(t, d)
		got = runContextCapture(t, "--stats")
	})
	checkGoldenIn(t, origWD, "context_stats_spec_unset.golden", got)
}

// TestContext_Spec_EnabledPresent_FirstPosition: configured + present +
// non-whitespace → the spec doc is emitted FIRST — before file-structure,
// repomap, and the timeline (H1.3's "most stable doc first" placement).
func TestContext_Spec_EnabledPresent_FirstPosition(t *testing.T) {
	withTempCwd(t, func(d string) {
		specFixture(t, d)
		mustWriteUnder(t, d, "docs/spec.md", "SPECMARKER\n")
		mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  spec_file: docs/spec.md\n")
		s := runContextCapture(t)
		if !strings.Contains(s, `<document type="spec">`) {
			t.Fatalf("spec doc missing when enabled+present:\n%s", s)
		}
		if !strings.Contains(s, "<source>docs/spec.md</source>") {
			t.Errorf("spec doc source not the configured path:\n%s", s)
		}
		if !strings.Contains(s, "SPECMARKER") {
			t.Errorf("spec body missing:\n%s", s)
		}
		iSpec := strings.Index(s, `type="spec"`)
		iFS := strings.Index(s, `type="file-structure"`)
		iTL := strings.Index(s, `type="decision-timeline"`)
		if !(iSpec >= 0 && iSpec < iFS && iFS < iTL) {
			t.Errorf("spec not first (spec=%d fs=%d tl=%d):\n%s", iSpec, iFS, iTL, s)
		}
	})
}

// TestContext_Spec_EnabledPresent_StatsTerm: --stats breaks out the spec
// term (size only — no density claim, since a spec distills nothing).
func TestContext_Spec_EnabledPresent_StatsTerm(t *testing.T) {
	withTempCwd(t, func(d string) {
		specFixture(t, d)
		mustWriteUnder(t, d, "docs/spec.md", "SPECMARKER\n")
		mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  spec_file: docs/spec.md\n")
		s := runContextCapture(t, "--stats")
		if !strings.Contains(s, "spec ") {
			t.Errorf("--stats missing the spec term:\n%s", s)
		}
		if strings.Contains(s, "spec distills") || strings.Contains(s, "x denser") && strings.Contains(s, "spec") {
			t.Errorf("--stats must not claim spec density:\n%s", s)
		}
	})
}

// TestContext_Spec_OmissionCases_ByteIdenticalToDisabled: every omission
// path (missing file, empty/whitespace file, absolute path, out-of-root
// relative path) must produce a payload BYTE-IDENTICAL to spec_file being
// entirely unset — no absent-marker, no error, no partial fold-in.
func TestContext_Spec_OmissionCases_ByteIdenticalToDisabled(t *testing.T) {
	var disabled string
	withTempCwd(t, func(d string) {
		specFixture(t, d)
		disabled = runContextCapture(t)
	})

	cases := []struct {
		name  string
		setup func(t *testing.T, d string)
	}{
		{
			name: "configured-but-missing",
			setup: func(t *testing.T, d string) {
				mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  spec_file: docs/spec.md\n")
				// docs/spec.md intentionally absent.
			},
		},
		{
			name: "configured-but-empty",
			setup: func(t *testing.T, d string) {
				mustWriteUnder(t, d, "docs/spec.md", "")
				mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  spec_file: docs/spec.md\n")
			},
		},
		{
			name: "configured-but-whitespace-only",
			setup: func(t *testing.T, d string) {
				mustWriteUnder(t, d, "docs/spec.md", "   \n\t\n  \n")
				mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  spec_file: docs/spec.md\n")
			},
		},
		{
			name: "configured-absolute-path",
			setup: func(t *testing.T, d string) {
				mustWriteUnder(t, d, "docs/spec.md", "SPECMARKER\n")
				mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  spec_file: "+filepath.Join(d, "docs", "spec.md")+"\n")
			},
		},
		{
			name: "configured-out-of-root-escape",
			setup: func(t *testing.T, d string) {
				mustWriteUnder(t, d, "docs/spec.md", "SPECMARKER\n")
				mustWriteUnder(t, d, ".logmind/config.yml", "context:\n  spec_file: ../evil.md\n")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			withTempCwd(t, func(d string) {
				specFixture(t, d)
				tc.setup(t, d)
				got = runContextCapture(t)
			})
			if got != disabled {
				t.Errorf("%s: payload not byte-identical to spec_file-unset\n--- disabled ---\n%s\n--- got ---\n%s",
					tc.name, disabled, got)
			}
		})
	}
}
