// show_test.go — exercises `logmind show` against tmpdir fixtures.
//
// Coverage:
//   - default branch streams docs/decisions.md
//   - feature branch streams docs/decisions-branches/<branch>.md, NOT
//     docs/decisions.md (the "current branch" contract SKILL.md/AGENTS.md
//     document)
//   - no decisions logged yet on the current branch → friendly message
//   - --all appends a LEGACY docs/decisions-archive.md under an ARCHIVED
//     DECISIONS banner when that file exists, and never invents one when it
//     doesn't
//   - --all ALSO appends every other docs/decisions-branches/*.md file under
//     a BRANCH DECISIONS banner (SPEC section sec-3-2's "every branch
//     decisions file" half), without duplicating the current branch's own
//     file
//   - --quiet collapses stdout to exactly one `ok k=v` line
//   - docs/ missing → friendly error + ErrSilent
//   - --brief: title + timestamp only, grouped by "[source]" tag under --all
//   - --json: SPEC section sec-3-2's NORMATIVE schema — exact key set, exact
//     source grammar (main / archive / branch:<name>), machine-clean stdout
//   - --brief --json: full schema keys always present, body fields zeroed
package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustMkdir + mustWrite are tiny fixture helpers shared by show_test.go and
// search_test.go — both build docs/ trees directly (without going through
// `logmind init`/`logmind log`) so tests can pin exact file contents.
func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// runShowCmd runs `logmind show [extraArgs...]` and returns combined output.
func runShowCmd(t *testing.T, extraArgs ...string) string {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs(append([]string{"show"}, extraArgs...))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("show %v: %v\n%s", extraArgs, err, out.String())
	}
	return out.String()
}

// TestShow_DefaultBranch_StreamsMainBranchFile: on the default branch, `show`
// streams docs/decisions-branches/main.md — the file `logmind log` just wrote
// to, main being a branch like any other (§3.2) — and prints the ok-trailer.
func TestShow_DefaultBranch_StreamsMainBranchFile(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		withFakeTTY(t, false, func() { logOnce(t, "Use PostgreSQL") })

		body := runShowCmd(t)
		mustContain(t, body, "## ")
		mustContain(t, body, "Use PostgreSQL")
		mustContain(t, body, "ok show: docs/decisions-branches/main.md")
		mustContain(t, body, "bytes")
	})
}

// TestShow_FeatureBranch_StreamsBranchFile: on a feature branch, `show`
// streams docs/decisions-branches/<branch>.md — the SAME file `logmind log`
// just wrote to — not docs/decisions.md. This is the "current branch"
// contract the docs promise.
func TestShow_FeatureBranch_StreamsBranchFile(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		checkoutBranch(t, d, "feat/login")
		withFakeTTY(t, false, func() { logOnce(t, "Add JWT auth") })

		body := runShowCmd(t)
		mustContain(t, body, "Add JWT auth")
		mustContain(t, body, "ok show: docs/decisions-branches/feat__login.md")
		// main's own branch file must NOT leak in.
		if strings.Contains(body, "Initialize logmind decision tracking") {
			t.Errorf("feature-branch show leaked docs/decisions-branches/main.md content:\n%s", body)
		}
	})
}

// TestShow_NoDecisionsYetOnBranch: a fresh feature branch with no `logmind
// log` yet has no decisions-branches file on disk — `show` reports the
// friendly empty message rather than erroring.
func TestShow_NoDecisionsYetOnBranch(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		checkoutBranch(t, d, "feat/empty")

		body := runShowCmd(t)
		mustContain(t, body, "No decisions logged yet on this branch.")
		mustContain(t, body, "ok show:")
	})
}

// TestShow_All_StreamsALegacyArchive: §3.2 stopped rotation, so no repo grows
// a NEW docs/decisions-archive.md — but one left behind by a pre-§3.2 binary
// holds real decisions, and `--all` must stream it under its banner. A repo
// that never rotated has no archive, and its absence is not reported as a
// missing thing.
func TestShow_All_StreamsALegacyArchive(t *testing.T) {
	for _, writeArchive := range []bool{true, false} {
		name := "legacy archive present"
		if !writeArchive {
			name = "no archive file"
		}
		t.Run(name, func(t *testing.T) {
			withTempCwd(t, func(d string) {
				mustMkdir(t, filepath.Join(d, "docs"))
				mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
					"## 2026-06-01 10:00 - Main decision\n")
				if writeArchive {
					mustWrite(t, filepath.Join(d, "docs", "decisions-archive.md"),
						"## 2025-01-01 09:00 - Archived decision\n")
				}

				body := runShowCmd(t, "--all")
				mustContain(t, body, "Main decision")
				if writeArchive {
					mustContain(t, body, "ARCHIVED DECISIONS")
					mustContain(t, body, "Archived decision")
				} else {
					if strings.Contains(body, "ARCHIVED DECISIONS") {
						t.Errorf("--all streamed an ARCHIVED DECISIONS banner with no archive on disk:\n%s", body)
					}
					if strings.Contains(body, "Archived decision") {
						t.Errorf("--all invented archived content:\n%s", body)
					}
				}
			})
		})
	}
}

// TestShow_Quiet_EmitsOneOkLine: --quiet suppresses the verbatim body —
// matching `logmind repomap`'s stdout-sink precedent — leaving exactly one
// `ok k=v` line.
func TestShow_Quiet_EmitsOneOkLine(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - Quiet decision\n")

		body := runShowCmd(t, "--quiet")
		lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
		if len(lines) != 1 {
			t.Fatalf("quiet show: want exactly 1 line, got %d:\n%s", len(lines), body)
		}
		if !strings.HasPrefix(lines[0], "ok show ") {
			t.Errorf("quiet show line = %q; want prefix %q", lines[0], "ok show ")
		}
		if strings.Contains(body, "Quiet decision") {
			t.Errorf("quiet show leaked the decision body:\n%s", body)
		}
		mustContain(t, body, "path=docs/decisions.md")
	})
}

// TestShow_DocsMissingErrors: no docs/ → friendly error on the error
// channel + ErrSilent.
func TestShow_DocsMissingErrors(t *testing.T) {
	withTempCwd(t, func(_ string) {
		root := NewRootCmd()
		root.SetArgs([]string{"show"})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err == nil {
			t.Fatalf("expected ErrSilent when docs/ missing")
		}
		mustContain(t, out.String(), "docs/ directory not found")
	})
}

// TestShow_All_IncludesBranchFiles: SPEC section sec-3-2's "--all: include
// archive and EVERY branch decisions file" — the gap this build closes.
// Before this change, --all only appended the archive; a repo with a branch
// decisions file (from another in-flight branch) never surfaced it.
func TestShow_All_IncludesBranchFiles(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs", "decisions-branches"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - Main decision\n")
		mustWrite(t, filepath.Join(d, "docs", "decisions-branches", "feat__other.md"),
			"## 2026-06-02 11:00 - Other branch decision\n")

		body := runShowCmd(t, "--all")
		mustContain(t, body, "Main decision")
		mustContain(t, body, "BRANCH DECISIONS: feat/other")
		mustContain(t, body, "Other branch decision")
		mustContain(t, body, "+ 1 branch file(s)")
	})
}

// TestShow_All_ExcludesCurrentBranchFile: on a feature branch, the current
// branch's own decisions-branches file is already the primary body — --all
// must include every OTHER branch file without streaming the current one
// twice.
func TestShow_All_ExcludesCurrentBranchFile(t *testing.T) {
	withTempCwd(t, func(d string) {
		initLogTestGitRepo(t, d)
		scaffoldDocs(t)
		checkoutBranch(t, d, "feat/login")
		withFakeTTY(t, false, func() { logOnce(t, "Add JWT auth") })

		// An unrelated in-flight branch's file, simulating another agent's work.
		mustWrite(t, filepath.Join(d, "docs", "decisions-branches", "feat__other.md"),
			"## 2026-06-02 11:00 - Other branch decision\n")

		body := runShowCmd(t, "--all")
		mustContain(t, body, "Add JWT auth")
		mustContain(t, body, "BRANCH DECISIONS: feat/other")
		mustContain(t, body, "Other branch decision")

		// One banner per OTHER branch file — feat/other plus main, whose own
		// file carries the first decision `logmind init` logged (§3.2: main is
		// a branch like any other). The current branch's file must not ALSO
		// get a "BRANCH DECISIONS: feat/login" section, since its content is
		// already the primary body above.
		if n := strings.Count(body, "BRANCH DECISIONS:"); n != 2 {
			t.Errorf("want exactly 2 BRANCH DECISIONS banners (feat/other, main), got %d:\n%s", n, body)
		}
		mustContain(t, body, "BRANCH DECISIONS: main")
		if strings.Contains(body, "BRANCH DECISIONS: feat/login") {
			t.Errorf("current branch file re-shown under its own BRANCH DECISIONS banner:\n%s", body)
		}
	})
}

// TestShow_Brief_TitleAndTimestampOnly: --brief prints "TIMESTAMP - TITLE"
// only — the reasoning/alternatives/implications body must not leak into the
// text output.
func TestShow_Brief_TitleAndTimestampOnly(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - First decision\n\n"+
				"**Reasoning:** Because reasons\n\n"+
				"**Alternatives considered:** Option A, Option B\n\n"+
				"**Implications:**\n- Impact one\n\n---\n\n")

		body := runShowCmd(t, "--brief")
		mustContain(t, body, "2026-06-01 10:00 - First decision")
		mustContain(t, body, "ok show: 1 decision(s)")
		if strings.Contains(body, "Because reasons") {
			t.Errorf("--brief leaked the reasoning body:\n%s", body)
		}
		if strings.Contains(body, "Option A") {
			t.Errorf("--brief leaked the alternatives body:\n%s", body)
		}
	})
}

// TestShow_Brief_All_GroupsBySource: under --all, --brief groups lines under
// a "[source]" tag per source, in main → branch order, and the tag text
// matches the --json "source" value exactly.
func TestShow_Brief_All_GroupsBySource(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs", "decisions-branches"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - Main decision\n")
		mustWrite(t, filepath.Join(d, "docs", "decisions-branches", "feat__other.md"),
			"## 2026-06-02 11:00 - Branch decision\n")
		// A legacy archive from a pre-§3.2 binary — still a source, tagged
		// with the grammar's "archive" label.
		mustWrite(t, filepath.Join(d, "docs", "decisions-archive.md"),
			"## 2025-01-01 09:00 - Archived decision\n")

		body := runShowCmd(t, "--brief", "--all")
		mustContain(t, body, "[main]")
		mustContain(t, body, "[branch:feat/other]")
		mustContain(t, body, "[archive]")
		mustContain(t, body, "2026-06-01 10:00 - Main decision")
		mustContain(t, body, "2026-06-02 11:00 - Branch decision")
		mustContain(t, body, "2025-01-01 09:00 - Archived decision")

		mainIdx := strings.Index(body, "[main]")
		branchIdx := strings.Index(body, "[branch:feat/other]")
		if !(mainIdx >= 0 && mainIdx < branchIdx) {
			t.Errorf("want [main] < [branch:feat/other] ordering, got indices %d, %d:\n%s",
				mainIdx, branchIdx, body)
		}
	})
}

// TestShow_JSON_SchemaKeysAndValues pins SPEC section sec-3-2's NORMATIVE
// --json schema for a 2-decision fixture: exact key set (no more, no fewer —
// a future rename/add/drop of a key breaks this test) and correct values,
// including the alternatives/implications arrays parsed out of the
// **Alternatives considered:**/**Implications:** markdown sections.
func TestShow_JSON_SchemaKeysAndValues(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - First decision\n\n"+
				"**Reasoning:** Because reasons\n\n"+
				"**Alternatives considered:** Option A, Option B\n\n"+
				"**Implications:**\n- Impact one\n- Impact two\n\n---\n\n"+
				"## 2026-06-02 11:15 - Second decision\n\n"+
				"**Reasoning:** Another reason\n\n---\n\n")

		body := runShowCmd(t, "--json")

		var doc struct {
			Decisions []map[string]any `json:"decisions"`
		}
		if err := json.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatalf("--json output did not parse as JSON: %v\nbody:\n%s", err, body)
		}
		if len(doc.Decisions) != 2 {
			t.Fatalf("want 2 decisions, got %d: %v", len(doc.Decisions), doc.Decisions)
		}

		wantKeys := []string{"title", "timestamp", "reasoning", "alternatives", "implications", "source"}
		for _, entry := range doc.Decisions {
			if len(entry) != len(wantKeys) {
				t.Errorf("entry has %d keys, want %d (NORMATIVE schema): %v", len(entry), len(wantKeys), entry)
			}
			for _, k := range wantKeys {
				if _, ok := entry[k]; !ok {
					t.Errorf("entry missing NORMATIVE key %q: %v", k, entry)
				}
			}
		}

		first := doc.Decisions[0]
		if first["title"] != "First decision" {
			t.Errorf("title = %v, want %q", first["title"], "First decision")
		}
		if first["timestamp"] != "2026-06-01 10:00" {
			t.Errorf("timestamp = %v, want %q", first["timestamp"], "2026-06-01 10:00")
		}
		if first["reasoning"] != "Because reasons" {
			t.Errorf("reasoning = %v, want %q", first["reasoning"], "Because reasons")
		}
		if first["source"] != "main" {
			t.Errorf("source = %v, want %q", first["source"], "main")
		}
		if alts, ok := first["alternatives"].([]any); !ok || len(alts) != 2 || alts[0] != "Option A" || alts[1] != "Option B" {
			t.Errorf("alternatives = %v, want [Option A, Option B]", first["alternatives"])
		}
		if impls, ok := first["implications"].([]any); !ok || len(impls) != 2 || impls[0] != "Impact one" || impls[1] != "Impact two" {
			t.Errorf("implications = %v, want [Impact one, Impact two]", first["implications"])
		}

		second := doc.Decisions[1]
		if second["title"] != "Second decision" || second["reasoning"] != "Another reason" {
			t.Errorf("second entry mismatch: %v", second)
		}
		if secondAlts, ok := second["alternatives"].([]any); !ok || len(secondAlts) != 0 {
			t.Errorf("second.alternatives = %v, want empty array (present, not null, not omitted)", second["alternatives"])
		}
	})
}

// TestShow_JSON_MachineCleanOutput: --json's stdout must be the bare JSON
// document and nothing else — no ok trailer, no banner — so it is pipeable
// into `jq` unmodified.
func TestShow_JSON_MachineCleanOutput(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - Only decision\n")

		body := runShowCmd(t, "--json")
		trimmed := strings.TrimSpace(body)
		if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
			t.Fatalf("--json output is not a bare JSON document:\n%s", body)
		}
		if strings.Contains(body, "ok show") {
			t.Errorf("--json output leaked the ok trailer:\n%s", body)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatalf("json.Unmarshal failed on the full body (must be pure JSON): %v", err)
		}
	})
}

// TestShow_JSON_All_SourceValues: under --all --json, every decision's
// "source" value matches the SPEC section sec-3-2 grammar exactly:
// "main" | "archive" | "branch:<name>". A legacy docs/decisions-archive.md
// still produces "archive" — §3.2 stopped WRITING that file, it did not make
// the decisions in it stop counting.
func TestShow_JSON_All_SourceValues(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs", "decisions-branches"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-03 09:00 - Main decision\n")
		mustWrite(t, filepath.Join(d, "docs", "decisions-branches", "feat__widget.md"),
			"## 2026-06-02 08:00 - Branch decision\n")
		// A legacy archive from a pre-§3.2 binary — still a source.
		mustWrite(t, filepath.Join(d, "docs", "decisions-archive.md"),
			"## 2025-01-01 07:00 - Archived decision\n")

		body := runShowCmd(t, "--all", "--json")
		var doc struct {
			Decisions []map[string]any `json:"decisions"`
		}
		if err := json.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatalf("--all --json output did not parse: %v\n%s", err, body)
		}
		got := map[string]bool{}
		for _, e := range doc.Decisions {
			got[e["source"].(string)] = true
		}
		for _, want := range []string{"main", "archive", "branch:feat/widget"} {
			if !got[want] {
				t.Errorf("missing source %q; got sources %v", want, got)
			}
		}
		for src := range got {
			if src != "main" && src != "archive" && !strings.HasPrefix(src, "branch:") {
				t.Errorf("source %q is outside the NORMATIVE grammar; got %v", src, got)
			}
		}
	})
}

// TestShow_BriefJSON_ZeroesBodyFieldsKeepsSchema: --brief --json keeps the
// FULL NORMATIVE key set (never drops a key) but zeroes
// reasoning/alternatives/implications ("" / [] / []) rather than parsing
// them out of the entry body — the documented --brief+--json precedence.
func TestShow_BriefJSON_ZeroesBodyFieldsKeepsSchema(t *testing.T) {
	withTempCwd(t, func(d string) {
		mustMkdir(t, filepath.Join(d, "docs"))
		mustWrite(t, filepath.Join(d, "docs", "decisions.md"),
			"## 2026-06-01 10:00 - First decision\n\n"+
				"**Reasoning:** Because reasons\n\n"+
				"**Alternatives considered:** Option A, Option B\n\n"+
				"**Implications:**\n- Impact one\n\n---\n\n")

		body := runShowCmd(t, "--brief", "--json")
		var doc struct {
			Decisions []map[string]any `json:"decisions"`
		}
		if err := json.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatalf("--brief --json output did not parse: %v\n%s", err, body)
		}
		if len(doc.Decisions) != 1 {
			t.Fatalf("want 1 decision, got %d", len(doc.Decisions))
		}
		e := doc.Decisions[0]
		for _, k := range []string{"title", "timestamp", "reasoning", "alternatives", "implications", "source"} {
			if _, ok := e[k]; !ok {
				t.Errorf("--brief --json dropped NORMATIVE key %q: %v", k, e)
			}
		}
		if e["title"] != "First decision" {
			t.Errorf("title = %v, want %q (title/timestamp survive --brief)", e["title"], "First decision")
		}
		if e["reasoning"] != "" {
			t.Errorf("reasoning = %v, want zeroed \"\" under --brief", e["reasoning"])
		}
		if alts, ok := e["alternatives"].([]any); !ok || len(alts) != 0 {
			t.Errorf("alternatives = %v, want zeroed [] under --brief", e["alternatives"])
		}
		if impls, ok := e["implications"].([]any); !ok || len(impls) != 0 {
			t.Errorf("implications = %v, want zeroed [] under --brief", e["implications"])
		}
	})
}
