package cli

import (
	"bytes"
	"strings"
	"testing"
)

func runRepomapCapture(t *testing.T, args ...string) string {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs(append([]string{"repomap"}, args...))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("repomap %v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

// TestRepomap_Command: the skeleton lists a file's signatures with bodies
// dropped, and closes with the `ok repomap:` receipt.
func TestRepomap_Command(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, "svc.go", "package p\nfunc Serve(addr string) error { panic(\"body\") }\ntype Server struct{ p int }\n")
		s := runRepomapCapture(t)
		for _, want := range []string{
			"# Repomap",
			"svc.go",
			"func Serve(addr string) error",
			"type Server struct",
			"ok repomap: 1 files, 2 symbols,",
		} {
			if !strings.Contains(s, want) {
				t.Errorf("repomap output missing %q:\n%s", want, s)
			}
		}
		if strings.Contains(s, "panic(") || strings.Contains(s, "{ p int }") {
			t.Errorf("body/fields leaked into the skeleton:\n%s", s)
		}
	})
}

// TestRepomap_Quiet: --quiet prints exactly one chainable ok line, no skeleton.
func TestRepomap_Quiet(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, "svc.go", "package p\nfunc Serve() {}\n")
		s := runRepomapCapture(t, "--quiet")
		trimmed := strings.TrimRight(s, "\n")
		if strings.Contains(trimmed, "\n") {
			t.Fatalf("quiet repomap emitted >1 line:\n%q", s)
		}
		for _, want := range []string{"ok repomap", "files=1", "symbols=1", "sink=stdout"} {
			if !strings.Contains(trimmed, want) {
				t.Errorf("quiet ok line %q missing %q", trimmed, want)
			}
		}
		if strings.Contains(s, "# Repomap") {
			t.Errorf("quiet mode leaked the skeleton body: %q", s)
		}
	})
}

// TestRepomap_Deterministic: byte-identical across runs (caching invariant).
func TestRepomap_Deterministic(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, "a.go", "package p\nfunc A() {}\n")
		mustWriteUnder(t, d, "b.go", "package p\nfunc B() {}\n")
		if runRepomapCapture(t) != runRepomapCapture(t) {
			t.Error("repomap output not byte-deterministic")
		}
	})
}
