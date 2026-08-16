// redirect_entry_test.go — logmind#336: the unit-level half of SPEC:1101's
// merge rule. redirect_ownership_test.go in internal/cli pins the bytes on
// disk after the command the user ran; these pin the boundary decisions that
// command is made of, where a failure can say WHICH one moved.
package inserter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/agents"
	"github.com/thrillmade/logmind/internal/templates"
)

func writeAgentFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func readAgentFile(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// claudeAgent is the registry entry for the markdown row the issue reported.
func claudeAgent(t *testing.T) agents.Agent {
	t.Helper()
	a, ok := agents.Lookup("claude")
	if !ok {
		t.Fatal("the registry no longer has a claude entry")
	}
	return a
}

// TestLogmindEntrySpan pins where logmind's entry starts and stops. The span
// IS the fix: everything outside it is copied through byte-for-byte, so a
// boundary that moves is content that disappears.
func TestLogmindEntrySpan(t *testing.T) {
	const marker = "<!-- logmind-stub: AI agent instructions for this project live in AGENTS.md -->"
	for _, tc := range []struct {
		name       string
		content    string
		start, end int
		ok         bool
	}{{
		name:    "stub alone, trailing newline",
		content: marker + "\nSee AGENTS.md.\n",
		start:   0, end: 2, ok: true, // line 2 (the "" after the final \n) is blank and ends it
	}, {
		// agent-skills / reporulez / clud-bug / this repo.
		name:    "under an @import",
		content: "@AGENTS.md\n\n" + marker + "\nSee AGENTS.md.",
		start:   2, end: 4, ok: true,
	}, {
		// protocol: a blank line, then clud-bug's block.
		name:    "above another component's block",
		content: marker + "\nSee AGENTS.md.\n\n<!-- clud-bug-start -->\nx\n<!-- clud-bug-end -->\n",
		start:   0, end: 2, ok: true,
	}, {
		// No blank line between: the HTML comment itself ends the entry, so a
		// component that installs flush against ours still keeps its bytes.
		name:    "flush against another component's block",
		content: marker + "\nSee AGENTS.md.\n<!-- clud-bug-start -->\nx\n",
		start:   0, end: 2, ok: true,
	}, {
		name:    "marker is the last line",
		content: "@AGENTS.md\n\n" + marker,
		start:   2, end: 3, ok: true,
	}, {
		name:    "no logmind entry",
		content: "# mine\n\nnothing of yours here\n",
		ok:      false,
	}, {
		// Constraint the write side depends on: a quotation is not a claim.
		name:    "marker inside a code fence",
		content: "# mine\n\n```markdown\n" + marker + "\nexample\n```\n",
		ok:      false,
	}, {
		// A mention mid-sentence is prose, not a line-start claim.
		name:    "marker mid-line",
		content: "logmind writes " + marker + " at the top.\n",
		ok:      false,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			start, end, ok := logmindEntrySpan(strings.Split(tc.content, "\n"))
			if ok != tc.ok {
				t.Fatalf("ok = %v; want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if start != tc.start || end != tc.end {
				t.Errorf("span = [%d,%d); want [%d,%d)", start, end, tc.start, tc.end)
			}
		})
	}
}

// TestMergeStubEntry_PreservesEverythingOutsideTheSpan is SPEC:1101's first
// sentence at the level it is implemented, and its round trip: splicing the
// CURRENT body into a file that already carries it must change nothing at all.
func TestMergeStubEntry_PreservesEverythingOutsideTheSpan(t *testing.T) {
	const importLine = "@AGENTS.md\n\n"
	const below = "\n## My own notes\n\nKEEP_ME\n"
	stale := importLine + "<!-- logmind-stub: old -->\nOLD BODY\n" + below

	got := mergeStubEntry(stale, templates.Stub())

	// The blank line that TERMINATED the entry is outside the span, so it
	// survives too — the span ends AT it, not after it.
	want := importLine + strings.TrimRight(templates.Stub(), "\n") + "\n" + below
	if got != want {
		t.Fatalf("merge:\n got: %q\nwant: %q", got, want)
	}
	// Idempotence, which is what stops a repository seeing a diff per init.
	if again := mergeStubEntry(got, templates.Stub()); again != "" {
		t.Errorf("re-merging an already-current entry rewrote the file: %q", again)
	}
}

// TestPlanRedirectWrite covers the five states over the three file forms in
// SPEC §1.2's table, on the ONE classifier both write paths route through.
func TestPlanRedirectWrite(t *testing.T) {
	claude := claudeAgent(t)
	zed, ok := agents.Lookup("zed")
	if !ok {
		t.Fatal("the registry no longer has a zed entry")
	}

	t.Run("absent markdown is written", func(t *testing.T) {
		p := planRedirectWrite(claude, "", false)
		if p.Refusal != nil || p.Body != templates.Stub() {
			t.Fatalf("plan = %+v; want the bundled stub", p)
		}
	})

	t.Run("empty existing file is the user's", func(t *testing.T) {
		// An existing empty file is a file, not an invitation — `exists` is
		// passed separately from the content for exactly this case.
		p := planRedirectWrite(claude, "", true)
		if p.Refusal == nil {
			t.Fatalf("plan = %+v; want a refusal", p)
		}
	})

	t.Run("foreign marker refuses and names the owner", func(t *testing.T) {
		p := planRedirectWrite(claude, "<!-- skdd-stub: x -->\nbody\n", true)
		if p.Refusal == nil {
			t.Fatal("a file carrying skdd's marker was not refused")
		}
		if p.Refusal.Ownership != MarkerForeign || p.Refusal.Owner != "skdd" {
			t.Errorf("refusal = %+v; want MarkerForeign owned by skdd", p.Refusal)
		}
		if p.Body != "" {
			t.Errorf("a refusal carried bytes to write: %q", p.Body)
		}
	})

	t.Run("logmind entry below a foreign one is still ours to refresh", func(t *testing.T) {
		// Order must not decide ownership: whoever installed first is not
		// thereby the owner of the file.
		p := planRedirectWrite(claude,
			"<!-- skdd-stub: x -->\ntheirs\n\n<!-- logmind-stub: old -->\nSTALE\n", true)
		if p.Refusal != nil {
			t.Fatalf("refused a file carrying our own entry: %+v", p.Refusal)
		}
		if !strings.Contains(p.Body, "theirs") {
			t.Errorf("the other component's entry did not survive: %q", p.Body)
		}
		if strings.Contains(p.Body, "STALE") {
			t.Errorf("our own entry was not refreshed: %q", p.Body)
		}
	})

	t.Run("legacy in-place block is left to migrate", func(t *testing.T) {
		// A CLAUDE.md carrying the legacy `<!-- logmind-start -->` section is
		// ours, but it is not a stub — converting it is `agents migrate`'s
		// job, which folds the content into AGENTS.md first. Neither a write
		// nor a refusal.
		legacy := "# CLAUDE.md\n\n" + startMarker + "\nold section\n" + endMarker + "\n"
		p := planRedirectWrite(claude, legacy, true)
		if p.Refusal != nil || p.Body != "" {
			t.Fatalf("plan = %+v; want no write and no refusal", p)
		}
	})

	t.Run("JSON without our key is the user's", func(t *testing.T) {
		p := planRedirectWrite(zed, "{\n  \"theme\": \"One Dark\"\n}\n", true)
		if p.Refusal == nil {
			t.Fatal("a real settings.json was not refused")
		}
		if p.Refusal.Ownership != MarkerForeign && p.Refusal.Ownership != MarkerAbsent {
			t.Errorf("refusal = %+v; want an unmarked verdict", p.Refusal)
		}
	})

	t.Run("JSONC is unreadable and therefore unclaimable", func(t *testing.T) {
		// Zed's settings file routinely carries comments. json.Unmarshal
		// fails, and a file logmind cannot read is a file logmind cannot own.
		p := planRedirectWrite(zed, "{\n  // theme\n  \"theme\": \"One Dark\"\n}\n", true)
		if p.Refusal == nil {
			t.Fatal("a JSONC settings file was not refused")
		}
	})

	t.Run("JSON merges our key and keeps the others", func(t *testing.T) {
		p := planRedirectWrite(zed,
			"{\n  \"theme\": \"One Dark\",\n  \"logmind\": {\"enabled\": false}\n}\n", true)
		if p.Refusal != nil {
			t.Fatalf("refused a settings.json carrying our own key: %+v", p.Refusal)
		}
		if !strings.Contains(p.Body, `"theme"`) || !strings.Contains(p.Body, "One Dark") {
			t.Errorf("the user's own settings were dropped: %q", p.Body)
		}
		if !strings.Contains(p.Body, "docs/timeline.md") {
			t.Errorf("our own entry was not refreshed: %q", p.Body)
		}
	})

	t.Run("JSON we wrote alone stays byte-identical", func(t *testing.T) {
		if p := planRedirectWrite(zed, jsonAgentBody(), true); p.Body != "" || p.Refusal != nil {
			t.Fatalf("plan = %+v; want no write for an already-current file", p)
		}
	})
}

// TestMigrateToAgentsMD_LeavesAnotherComponentsFileAlone covers the SECOND
// writer of these paths. `agents migrate` reads an unmarked per-tool file as
// the user's own instructions and moves them into AGENTS.md, which preserves
// every byte and is the whole point of the command — so an absent marker is
// deliberately not refused there. Another component's entry is different: it
// is not the user's content to consolidate, and folding it into AGENTS.md
// under "## From Cursor" while stamping logmind's stub on the path is the
// silent re-ownership protocol#77 exists to stop.
func TestMigrateToAgentsMD_LeavesAnotherComponentsFileAlone(t *testing.T) {
	dir := t.TempDir()
	writeAgentFile(t, dir, "AGENTS.md", agentsMDTemplate())
	const foreign = "<!-- skdd-stub: instructions live in AGENTS.md -->\nFOREIGN_SENTINEL\n"
	writeAgentFile(t, dir, ".cursorrules", foreign)
	// A CONTROL in the same run: an ordinary hand-written file still migrates,
	// so this is not "migrate stopped touching things".
	writeAgentFile(t, dir, "CLAUDE.md", "# mine\n\nUSER_PROSE_TO_MIGRATE\n")

	_, _, refused, err := MigrateToAgentsMD(dir)
	if err != nil {
		t.Fatalf("MigrateToAgentsMD: %v", err)
	}

	if got := readAgentFile(t, dir, ".cursorrules"); got != foreign {
		t.Errorf(".cursorrules was re-owned:\n got: %q\nwant: %q", got, foreign)
	}
	if len(refused) != 1 || refused[0].Path != ".cursorrules" {
		t.Fatalf("refused = %+v; want exactly the .cursorrules entry", refused)
	}
	agentsMD := readAgentFile(t, dir, "AGENTS.md")
	if strings.Contains(agentsMD, "FOREIGN_SENTINEL") {
		t.Error("another component's entry was folded into AGENTS.md")
	}
	if !strings.Contains(agentsMD, "USER_PROSE_TO_MIGRATE") {
		t.Error("the control did not migrate — the guard is too wide")
	}
	if got := readAgentFile(t, dir, "CLAUDE.md"); got != templates.Stub() {
		t.Errorf("the control was not stubbed: %q", got)
	}
}

// TestCreateAgentFile_RefusesAndWritesNothing is the guard at the level a
// caller sees it: a refusal must leave the bytes exactly as found.
func TestCreateAgentFile_RefusesAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	const original = "# mine\n\nUSER_SENTINEL\n"
	writeAgentFile(t, dir, "CLAUDE.md", original)

	written, refused, err := CreateAgentFile("claude", dir)
	if err != nil {
		t.Fatalf("CreateAgentFile: %v", err)
	}
	if refused == nil {
		t.Fatal("an unmarked CLAUDE.md was not refused")
	}
	if written.Path != "" {
		t.Errorf("a refusal reported a written path: %q", written.Path)
	}
	if got := readAgentFile(t, dir, "CLAUDE.md"); got != original {
		t.Errorf("the file was modified:\n got: %q\nwant: %q", got, original)
	}
	// The refusal has to be reportable: a note that cannot name the file or
	// what was looked for is not a report (SPEC §3.4).
	if refused.Path != "CLAUDE.md" || refused.Marker == "" || refused.Display == "" {
		t.Errorf("refusal = %+v; want Path, Marker and Display set", refused)
	}
}
