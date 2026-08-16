// redirect_ownership_test.go — logmind#336 / protocol#77 / SPEC:1101:
// `logmind init` replaced the per-tool redirect files whole, so it destroyed a
// hand-written CLAUDE.md, another component's entry, and — in the two
// repositories that publish this toolchain — the `@AGENTS.md` import line that
// is how Claude Code loads AGENTS.md at all. Silently, on exit 0.
//
// SPEC:1101 is two sentences and the FIRST one governs here:
//
//	An installer MUST merge rather than replace: it writes only the entry it
//	owns and leaves every other entry — including one the user wrote by hand —
//	exactly as it found it.
//
//	An artifact carrying no marker at all belongs to the user and MUST NOT be
//	overwritten.
//
// Merge-rather-than-replace binds whether or not logmind's marker is present,
// so "the file carries our marker" is not permission to rewrite the file. Five
// states, and the fix is only correct if all five hold in the same run — two
// that logmind LEAVES a file alone, three that it still writes into one.
// "Stop writing the redirect files" satisfies the first half and breaks SPEC
// §0.1's promise that a logmind-only repository still gets a working
// CLAUDE.md.
//
// Asserted through `logmind init` rather than against the inserter helper: the
// harm was the bytes on disk after the command the user ran, and a test on the
// helper goes green again the day a caller stops routing through it.
package cli

import (
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/templates"
)

// stubBody is logmind's entry as this binary ships it, without the trailing
// newline — the shape it takes when spliced INTO a file rather than being the
// whole of one.
func stubBody() string { return strings.TrimRight(templates.Stub(), "\n") }

// redirectCase is one row of SPEC §1.2's table as `init` finds it on disk.
type redirectCase struct {
	name  string
	agent string // registry name passed to --agents
	rel   string // repo-root-relative path init writes
	seed  string // bytes planted before init; "" means the file is absent
	want  string // EXACT bytes required afterwards
	// refused says logmind must leave the file alone AND say so on stderr.
	refused bool
	// survives is a byte sequence whose loss is the reported harm — named so a
	// failure says WHAT was destroyed rather than dumping two files.
	survives string
}

// redirectCases is the five-way control, run as ONE `logmind init`. One run,
// because the halves have to hold simultaneously: a fix that refuses
// everything passes the three refusal rows on its own, and the run that proves
// it wrong has to be the same run.
var redirectCases = []redirectCase{{
	// ROW 1 — ABSENT. Nothing but logmind writes these files in a repository
	// that has not installed the harness, so this row is SPEC §0.1's
	// independence clause.
	name:  "absent is created",
	agent: "cline",
	rel:   ".clinerules",
	seed:  "",
	want:  templates.Stub(),
}, {
	// ROW 2 — LOGMIND'S ENTRY, ALONE. Still refreshed, so the fix is not
	// "stop writing": the stale body must be gone afterwards.
	name:  "logmind entry alone is refreshed",
	agent: "aider",
	rel:   "CONVENTIONS.md",
	seed:  "<!-- logmind-stub: AI agent instructions for this project live in AGENTS.md -->\nSTALE LOGMIND STUB BODY\n",
	want:  templates.Stub(),
}, {
	// ROW 3 — THE ONE WITH TEETH. This is `agent-skills`' and `reporulez`'
	// CLAUDE.md byte for byte. Line 1 is Claude Code's import directive: not
	// prose, not part of the stub, and not something anyone would think to
	// re-add. A whole-file replace here does not cost a paragraph, it costs
	// every rule in AGENTS.md being loaded by the agent that reads it.
	name:     "logmind entry under an @import is refreshed around it",
	agent:    "claude",
	rel:      "CLAUDE.md",
	seed:     "@AGENTS.md\n\n<!-- logmind-stub: AI agent instructions for this project live in AGENTS.md -->\nSTALE STUB BODY UNDER AN IMPORT\n",
	want:     "@AGENTS.md\n\n" + stubBody() + "\n",
	survives: "@AGENTS.md",
}, {
	// ROW 4 — FOREIGN MARKER, NO LOGMIND ENTRY. protocol#77: nobody
	// overwrites a file carrying someone else's marker.
	name:     "foreign marker is left alone",
	agent:    "cursor",
	rel:      ".cursorrules",
	seed:     "<!-- skdd-stub: AI agent instructions live in AGENTS.md -->\nFOREIGN STUB CONTENT\n",
	want:     "<!-- skdd-stub: AI agent instructions live in AGENTS.md -->\nFOREIGN STUB CONTENT\n",
	refused:  true,
	survives: "FOREIGN STUB CONTENT",
}, {
	// ROW 5 — NO MARKER AT ALL. The reported defect: a CLAUDE.md-shaped file a
	// person wrote. SPEC:1101's second sentence.
	name:     "unmarked file is left alone",
	agent:    "windsurf",
	rel:      ".windsurfrules",
	seed:     "# My own rules\n\nHAND WRITTEN BY THE USER\n",
	want:     "# My own rules\n\nHAND WRITTEN BY THE USER\n",
	refused:  true,
	survives: "HAND WRITTEN BY THE USER",
}, {
	// ROW 5, JSON. A real .zed/settings.json is where a Zed user's whole
	// configuration lives, and it can carry no HTML comment — so this row is
	// what proves ownership is decided for every entry in the registry rather
	// than for the two markdown files named in the issue.
	name:     "unmarked JSON settings are left alone",
	agent:    "zed",
	rel:      ".zed/settings.json",
	seed:     "{\n  \"theme\": \"One Dark\",\n  \"USER_ZED_SETTING\": true\n}\n",
	want:     "{\n  \"theme\": \"One Dark\",\n  \"USER_ZED_SETTING\": true\n}\n",
	refused:  true,
	survives: "USER_ZED_SETTING",
}}

// seedRedirectCases plants every row's `seed` and returns the --agents value
// covering all of them.
func seedRedirectCases(t *testing.T) string {
	t.Helper()
	var names []string
	for _, c := range redirectCases {
		names = append(names, c.agent)
		if c.seed != "" {
			writeRel(t, c.rel, c.seed, 0o644)
		}
	}
	return strings.Join(names, ",")
}

// TestInit_RedirectFileOwnership is the whole ruling in one run.
func TestInit_RedirectFileOwnership(t *testing.T) {
	withTempCwd(t, func(_ string) {
		agentList := seedRedirectCases(t)

		_, stderr := runInitCapture(t, []string{"init", "--no-git", "--agents", agentList})

		for _, c := range redirectCases {
			t.Run(c.name, func(t *testing.T) {
				got := readRel(t, c.rel)
				// THE HARM, asserted where the user saw it: the bytes. An
				// exact compare rather than a sentinel search, because
				// "somewhere in the file" is not what merge promises — every
				// byte outside logmind's own entry has to be where it was.
				if got != c.want {
					t.Errorf("%s:\n got: %q\nwant: %q", c.rel, got, c.want)
				}
				if c.survives != "" && !strings.Contains(got, c.survives) {
					t.Errorf("%s lost %q — that is the reported harm", c.rel, c.survives)
				}
				if !c.refused {
					return
				}
				// SPEC §3.4's rule for the analogous fail-open case: a refusal
				// the user is never told about is the failure mode, not the
				// remedy. protocol#77 says it in as many words — "leave it,
				// and say so on stderr".
				if !strings.Contains(stderr, c.rel) {
					t.Errorf("nothing on stderr names %s; the refusal was silent.\nstderr: %s", c.rel, stderr)
				}
			})
		}
	})
}

// TestInit_RedirectEntryRefreshIsIdempotent — the second run must write
// nothing at all. A merge that re-splices its own output (an extra newline, a
// dropped one) shows up here and nowhere else, and a repository that gets a
// one-line diff on every `init` learns to ignore the diff.
func TestInit_RedirectEntryRefreshIsIdempotent(t *testing.T) {
	withTempCwd(t, func(_ string) {
		agentList := seedRedirectCases(t)
		runInitCapture(t, []string{"init", "--no-git", "--agents", agentList})
		after := map[string]string{}
		for _, c := range redirectCases {
			after[c.rel] = readRel(t, c.rel)
		}

		// A second fresh-install run (init is idempotent by design).
		runInitCapture(t, []string{"init", "--no-git", "--agents", agentList})

		for _, c := range redirectCases {
			if got := readRel(t, c.rel); got != after[c.rel] {
				t.Errorf("%s changed on the second init:\n got: %q\nwant: %q", c.rel, got, after[c.rel])
			}
		}
	})
}

// TestInit_PreservesAnotherComponentsEntryBelowOurOwn is `protocol`'s
// CLAUDE.md: logmind's stub first, clud-bug's whole marked block under it.
// Merge means logmind's three lines move and clud-bug's block does not.
func TestInit_PreservesAnotherComponentsEntryBelowOurOwn(t *testing.T) {
	withTempCwd(t, func(_ string) {
		const cludBug = "\n<!-- clud-bug-start -->\n<!-- clud-bug-block-version: v2 -->\n" +
			"## clud-bug — Claude PR review\n\nCLUD_BUG_ENTRY_SENTINEL\n<!-- clud-bug-end -->\n"
		writeRel(t, "CLAUDE.md",
			"<!-- logmind-stub: AI agent instructions for this project live in AGENTS.md -->\n"+
				"STALE LOGMIND BODY\n"+cludBug, 0o644)

		runInitCapture(t, []string{"init", "--no-git", "--agents", "claude"})

		got := readRel(t, "CLAUDE.md")
		if want := stubBody() + "\n" + cludBug; got != want {
			t.Errorf("CLAUDE.md:\n got: %q\nwant: %q", got, want)
		}
		if !strings.Contains(got, "CLUD_BUG_ENTRY_SENTINEL") {
			t.Error("another component's entry was destroyed")
		}
	})
}

// TestInit_IgnoresAMarkerInsideACodeFence — a repository quoting the stub in
// its own CLAUDE.md (this repo's docs/ai-agent-files.md does exactly that) is
// showing an example, not claiming an entry. Detection that cannot tell the
// difference hands logmind a write span in the middle of somebody's fenced
// block.
func TestInit_IgnoresAMarkerInsideACodeFence(t *testing.T) {
	withTempCwd(t, func(_ string) {
		const quoted = "# House rules\n\nlogmind writes this:\n\n```markdown\n" +
			"<!-- logmind-stub: AI agent instructions for this project live in AGENTS.md -->\n" +
			"FENCED_EXAMPLE_SENTINEL\n```\n"
		writeRel(t, "CLAUDE.md", quoted, 0o644)

		_, stderr := runInitCapture(t, []string{"init", "--no-git", "--agents", "claude"})

		if got := readRel(t, "CLAUDE.md"); got != quoted {
			t.Errorf("a fenced quotation was treated as logmind's entry:\n got: %q\nwant: %q", got, quoted)
		}
		if !strings.Contains(stderr, "CLAUDE.md") {
			t.Errorf("the refusal was silent.\nstderr: %s", stderr)
		}
	})
}

// TestInit_RedirectRefusalIsNotAFailure — the refusals are legitimate
// repository states, not errors: `init` finishes, writes everything else, and
// exits 0. Same contract the workflow-template refusals hold to (#286, #306).
func TestInit_RedirectRefusalIsNotAFailure(t *testing.T) {
	withTempCwd(t, func(_ string) {
		agentList := seedRedirectCases(t)

		stdout, _, err := tryInitCapture(t, []string{"init", "--no-git", "--agents", agentList})
		if err != nil {
			t.Fatalf("init exited non-zero over an ownership refusal: %v\n%s", err, stdout)
		}
		mustContain(t, stdout, "logmind initialized successfully!")
		// The rest of the install still landed — a refusal costs its own file,
		// not the run.
		readRel(t, "docs/timeline.md")
		readRel(t, ".logmind/config.yml")
	})
}

// TestInit_DoesNotClaimToHaveCreatedARefusedFile: "✓ Created CLAUDE.md" over a
// file init did not touch is the cheapest kind of lie a receipt can tell, and
// it is what made the original defect invisible — the user saw the line and
// had no reason to look.
func TestInit_DoesNotClaimToHaveCreatedARefusedFile(t *testing.T) {
	withTempCwd(t, func(_ string) {
		agentList := seedRedirectCases(t)

		stdout, _ := runInitCapture(t, []string{"init", "--no-git", "--agents", agentList})

		for _, c := range redirectCases {
			switch {
			case c.refused:
				if strings.Contains(stdout, "✓ Created "+c.rel) {
					t.Errorf("stdout claims to have created %s, which was left alone:\n%s", c.rel, stdout)
				}
			case c.seed != "":
				// Refreshed in place, not created — the file was already there.
				if strings.Contains(stdout, "✓ Created "+c.rel) {
					t.Errorf("stdout claims to have created %s, which already existed:\n%s", c.rel, stdout)
				}
			}
		}
	})
}

// TestInit_CodexDoesNotOverwriteAGENTSMD is the eleventh row of the table.
// `--agents codex` maps to AGENTS.md itself, and the per-agent writer used to
// render the bundled template straight over it — undoing, in the same run, the
// careful marker-block insertion EnsureAgentsMD had just done.
func TestInit_CodexDoesNotOverwriteAGENTSMD(t *testing.T) {
	withTempCwd(t, func(_ string) {
		writeRel(t, "AGENTS.md", "# AGENTS.md\n\nUSER PROSE IN AGENTS\n", 0o644)

		runInitCapture(t, []string{"init", "--no-git", "--agents", "codex"})

		got := readRel(t, "AGENTS.md")
		if !strings.Contains(got, "USER PROSE IN AGENTS") {
			t.Errorf("init destroyed the user's AGENTS.md prose:\n%s", got)
		}
		// And the logmind block still went in — the row is "don't overwrite",
		// not "don't install".
		if !strings.Contains(got, "<!-- logmind-start -->") {
			t.Errorf("AGENTS.md carries no logmind block:\n%s", got)
		}
	})
}
