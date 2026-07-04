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

// TestRepomap_MapTokensBudget: --map-tokens packs to a budget and marks the
// omitted files; the receipt reports the omitted count. Uses several
// substantial files so omitting some genuinely shrinks the output (past the
// never-worse passthrough).
func TestRepomap_MapTokensBudget(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, "go.mod", "module x\n")
		for _, name := range []string{"a", "b", "c", "d", "e"} {
			mustWriteUnder(t, d, name+".go",
				"package p\nfunc "+name+"1(x int) error { return nil }\nfunc "+name+"2(y string) bool { return false }\nfunc "+name+"3() {}\n")
		}
		s := runRepomapCapture(t, "--map-tokens", "60")
		if !strings.Contains(s, "omitted to fit the token budget") {
			t.Errorf("budget marker missing:\n%s", s)
		}
		if strings.Contains(s, "0 omitted,") {
			t.Errorf("expected some files omitted at a tight budget:\n%s", s)
		}
	})
}

// TestRepomap_NoBudgetNoOmittedMarker: without --map-tokens, all files emit and
// no truncation marker appears (the byte-stable default).
func TestRepomap_NoBudgetNoOmittedMarker(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustWriteUnder(t, d, "go.mod", "module x\n")
		mustWriteUnder(t, d, "a.go", "package p\nfunc A() {}\n")
		s := runRepomapCapture(t)
		if strings.Contains(s, "omitted to fit") {
			t.Errorf("no-budget run must not emit a truncation marker:\n%s", s)
		}
		if !strings.Contains(s, "0 omitted") {
			t.Errorf("receipt should report 0 omitted:\n%s", s)
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
