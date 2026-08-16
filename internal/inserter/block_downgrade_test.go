// block_downgrade_test.go — logmind#267: EnsureAgentsMD must never
// silently replace an AGENTS.md block whose version id this binary can't
// read or can't move forward.
//
// The pre-#267 classifier recognised markers by membership in a hardcoded
// {v5,v6,v7,v8,v7-pointer,v8-pointer,v9-pointer} set, so every FUTURE
// generation came back "unrecognised" — and EnsureAgentsMD read that as
// "install the slim default". A repository refreshed by a newer binary
// was walked BACKWARDS by an older one, reported as
// "Refreshed AGENTS.md logmind block to current template".
//
// Mixed binary versions is the defining condition of a staggered fleet
// rollout (#257), not an edge case, so these are ordering tests rather
// than enum tests: they plant a marker generation ABOVE whatever this
// binary ships and re-derive that number from the bundled template, so
// they keep testing the same thing across every future bump.
package inserter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/templates"
)

// aheadMarker returns a block-version token `ahead` generations NEWER than
// the one this binary ships for the given flavour: slim + 1 →
// "v11-pointer" while the bundled slim marker is v10-pointer.
//
// ahead=1 is the load-bearing distance: it is the generation an enum
// extended by one would have swallowed, so a test at that distance is what
// separates ORDERING from a longer hardcoded list.
func aheadMarker(t *testing.T, templateBody string, pointer bool, ahead int) string {
	t.Helper()
	bundled := bundledBlockMarker(templateBody)
	gen, ok := ParseMarkerGeneration(bundled)
	if !ok {
		t.Fatalf("bundled template carries no readable block-version marker; got %q", bundled)
	}
	suffix := ""
	if pointer {
		suffix = pointerSuffix
	}
	return fmt.Sprintf("v%d%s", gen+ahead, suffix)
}

// plantBlock writes an AGENTS.md whose marker block carries `marker`,
// wrapped in user prose above and below so the byte-comparison also
// proves the outer content survived. Returns the exact bytes written.
func plantBlock(t *testing.T, dir, marker string) string {
	t.Helper()
	content := "# AGENTS.md\n\nUser prose above.\n\n" + startMarker +
		"\n<!-- logmind-block-version: " + marker + " -->\n" +
		"## Decision logging\nbody written by a binary this one has never met\n" +
		endMarker + "\n\nUser prose below.\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	return content
}

// TestEnsureAgentsMD_RefusesNewerBlock is the #267 reproduction: a slim
// block ahead of this binary is left BYTE-IDENTICAL and the refusal is
// returned, not swallowed.
//
// Run at TWO distances on purpose. "the next generation" is what an enum
// extended by one (v10, v11, ...) would still swallow, so it is the case
// that separates an ordering guard from a longer hardcoded list; "many
// generations on" is the same fact at the far end.
func TestEnsureAgentsMD_RefusesNewerBlock(t *testing.T) {
	for _, ahead := range []int{1, 90} {
		t.Run(fmt.Sprintf("bundled+%d", ahead), func(t *testing.T) {
			dir := t.TempDir()
			marker := aheadMarker(t, templates.AgentsSlimTemplate(), true, ahead)
			before := plantBlock(t, dir, marker)

			msg, declined, err := EnsureAgentsMD(dir)
			if err != nil {
				t.Fatalf("EnsureAgentsMD: %v", err)
			}

			after, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
			if err != nil {
				t.Fatalf("read AGENTS.md: %v", err)
			}
			if string(after) != before {
				t.Errorf("a block newer than this binary was rewritten:\n got: %q\nwant: %q", after, before)
			}
			if msg != "" {
				t.Errorf("a refusal must not report a write; got %q", msg)
			}
			if declined == nil {
				t.Fatal("declined = nil; the refusal must be reported upward, not swallowed")
			}
			if declined.Installed != marker {
				t.Errorf("declined.Installed = %q; want the installed marker %q", declined.Installed, marker)
			}
			if want := bundledBlockMarker(templates.AgentsSlimTemplate()); declined.Bundled != want {
				t.Errorf("declined.Bundled = %q; want the bundled slim marker %q", declined.Bundled, want)
			}
			if !declined.Ahead {
				t.Errorf("declined.Ahead = false; a parseable newer generation is a downgrade, not an unknown id")
			}
			if declined.Path != filepath.Join(dir, "AGENTS.md") {
				t.Errorf("declined.Path = %q; want the AGENTS.md path", declined.Path)
			}
		})
	}
}

// TestEnsureAgentsMD_RefusesNewerFullBlock — same for the full flavour, so
// the guard isn't accidentally slim-only (slim is the binary's default, so
// a slim-only guard would still leave every full repo downgradeable).
func TestEnsureAgentsMD_RefusesNewerFullBlock(t *testing.T) {
	dir := t.TempDir()
	marker := aheadMarker(t, templates.AgentsTemplate(), false, 1)
	before := plantBlock(t, dir, marker)

	_, declined, err := EnsureAgentsMD(dir)
	if err != nil {
		t.Fatalf("EnsureAgentsMD: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if string(after) != before {
		t.Errorf("a newer FULL block was rewritten:\n got: %q\nwant: %q", after, before)
	}
	if declined == nil || !declined.Ahead {
		t.Fatalf("declined = %+v; want an Ahead refusal naming the full flavour", declined)
	}
	if want := bundledBlockMarker(templates.AgentsTemplate()); declined.Bundled != want {
		t.Errorf("declined.Bundled = %q; want the bundled FULL marker %q — a newer full block must "+
			"not be compared against (or replaced by) the slim default", declined.Bundled, want)
	}
}

// TestEnsureAgentsMD_RefusesUnreadableMarker — an id that isn't v<digits>,
// a flavour suffix this binary doesn't know, and no marker at all. None of
// them may be guessed at: "unrecognised" meaning "install the slim
// default" IS #267.
func TestEnsureAgentsMD_RefusesUnreadableMarker(t *testing.T) {
	for _, tc := range []struct {
		name, marker string
	}{
		{"non-numeric generation", "vNEXT-pointer"},
		{"unknown flavour suffix", "v3-sketch"},
		{"no v prefix", "9-pointer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			before := plantBlock(t, dir, tc.marker)

			msg, declined, err := EnsureAgentsMD(dir)
			if err != nil {
				t.Fatalf("EnsureAgentsMD: %v", err)
			}
			after, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
			if err != nil {
				t.Fatalf("read AGENTS.md: %v", err)
			}
			if string(after) != before {
				t.Errorf("an unreadable id was overwritten:\n got: %q\nwant: %q", after, before)
			}
			if msg != "" {
				t.Errorf("a refusal must not report a write; got %q", msg)
			}
			if declined == nil {
				t.Fatal("declined = nil; an unreadable id must be reported, not silently skipped")
			}
			if declined.Ahead {
				t.Errorf("declined.Ahead = true for %q; an unreadable id is not an ordering fact", tc.marker)
			}
			// Both bundled markers named — with no readable flavour there
			// is no single one to compare against (SPEC §3.4: say what you
			// looked for and what you found).
			for _, want := range []string{
				bundledBlockMarker(templates.AgentsTemplate()),
				bundledBlockMarker(templates.AgentsSlimTemplate()),
			} {
				if !strings.Contains(declined.Bundled, want) {
					t.Errorf("declined.Bundled = %q; want it to name %q", declined.Bundled, want)
				}
			}
		})
	}
}

// TestEnsureAgentsMD_MarkerlessBlockRefused — a marker block carrying no
// block-version id at all. SPEC §5.2's neighbouring rule ("an artifact
// carrying no marker at all belongs to the user") points the same way:
// there is nothing to compare, so leave it.
func TestEnsureAgentsMD_MarkerlessBlockRefused(t *testing.T) {
	dir := t.TempDir()
	before := "# AGENTS.md\n\n" + startMarker + "\nhand-written block, no version id\n" + endMarker + "\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(before), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	msg, declined, err := EnsureAgentsMD(dir)
	if err != nil {
		t.Fatalf("EnsureAgentsMD: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(after) != before {
		t.Errorf("a markerless block was overwritten:\n got: %q\nwant: %q", after, before)
	}
	if msg != "" {
		t.Errorf("a refusal must not report a write; got %q", msg)
	}
	if declined == nil || declined.Installed != "" {
		t.Fatalf("declined = %+v; want a refusal whose Installed is empty (no marker found)", declined)
	}
}

// TestFindOutdatedMarkerBlocks_RefusesNewerBlock — the sibling caller took
// the right ACTION before #267 (it skipped), but silently, so
// `agents update` reported an ahead-of-binary block as current. It must
// skip AND report.
func TestFindOutdatedMarkerBlocks_RefusesNewerBlock(t *testing.T) {
	dir := t.TempDir()
	marker := aheadMarker(t, templates.AgentsSlimTemplate(), true, 1)
	plantBlock(t, dir, marker)

	out, declined, err := FindOutdatedMarkerBlocks(dir)
	if err != nil {
		t.Fatalf("FindOutdatedMarkerBlocks: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("a newer block must not be listed as outdated; got %v", out)
	}
	if declined == nil || !declined.Ahead || declined.Installed != marker {
		t.Fatalf("declined = %+v; want an Ahead refusal naming %q", declined, marker)
	}
}

// TestEnsureAgentsMD_KnownOlderStillRefreshesForward pins the forward
// direction the refusal must not block: the generation BELOW the bundled
// one refreshes into the current body of its own flavour. Derived from the
// bundled markers so it survives future bumps.
func TestEnsureAgentsMD_KnownOlderStillRefreshesForward(t *testing.T) {
	for _, tc := range []struct {
		name     string
		template string
		pointer  bool
	}{
		{"full", templates.AgentsTemplate(), false},
		{"slim", templates.AgentsSlimTemplate(), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bundled := bundledBlockMarker(tc.template)
			gen, ok := ParseMarkerGeneration(bundled)
			if !ok || gen < 1 {
				t.Fatalf("bundled %s marker %q must parse to a generation above 0", tc.name, bundled)
			}
			suffix := ""
			if tc.pointer {
				suffix = pointerSuffix
			}
			dir := t.TempDir()
			plantBlock(t, dir, fmt.Sprintf("v%d%s", gen-1, suffix))

			msg, declined, err := EnsureAgentsMD(dir)
			if err != nil {
				t.Fatalf("EnsureAgentsMD: %v", err)
			}
			if declined != nil {
				t.Fatalf("an OLDER block must refresh, not be refused; got %+v", declined)
			}
			if msg != "Refreshed AGENTS.md logmind block to current template" {
				t.Errorf("status = %q; want the refresh status", msg)
			}
			got, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			// Refreshed into ITS OWN flavour, and the user prose outside the
			// markers survived.
			if marker := bundledBlockMarker(string(got)); marker != bundled {
				t.Errorf("marker after refresh = %q; want %q (flavour must not flip)", marker, bundled)
			}
			for _, prose := range []string{"User prose above.", "User prose below."} {
				if !strings.Contains(string(got), prose) {
					t.Errorf("content outside the markers was lost: %q missing", prose)
				}
			}
		})
	}
}

// TestEnsureAgentsMD_EqualGenerationIsNotAhead — the bundled generation
// itself is refreshable (a drifted body at the current id still gets its
// bytes restored). Pins the boundary as strictly-greater, not
// greater-or-equal.
func TestEnsureAgentsMD_EqualGenerationIsNotAhead(t *testing.T) {
	dir := t.TempDir()
	plantBlock(t, dir, bundledBlockMarker(templates.AgentsSlimTemplate()))

	msg, declined, err := EnsureAgentsMD(dir)
	if err != nil {
		t.Fatalf("EnsureAgentsMD: %v", err)
	}
	if declined != nil {
		t.Fatalf("the bundled generation must not be refused; got %+v", declined)
	}
	if msg != "Refreshed AGENTS.md logmind block to current template" {
		t.Errorf("status = %q; want the drifted body at the current id to be restored", msg)
	}
}

// TestParseBlockMarker covers the ordering key and the flavour half
// together: the generation orders numerically ("v11" > "v9", where a
// string compare says the opposite), and the suffix is what separates
// slim from full.
func TestParseBlockMarker(t *testing.T) {
	for _, tc := range []struct {
		marker     string
		wantGen    int
		wantSuffix string
		wantOK     bool
	}{
		{"v8", 8, "", true},
		{"v9-pointer", 9, "-pointer", true},
		{"v10-pointer", 10, "-pointer", true},
		{"v11", 11, "", true},
		{"v0-FAKE", 0, "-FAKE", true},
		{" v11 ", 11, "", true},
		{"", 0, "", false},
		{"v", 0, "", false},
		{"v-3", 0, "", false},
		{"v+5", 0, "", false},
		{"vNOPE", 0, "", false},
		{"v 4", 0, "", false},
		{"11", 0, "", false},
		{"latest", 0, "", false},
	} {
		gen, suffix, ok := parseBlockMarker(tc.marker)
		if gen != tc.wantGen || suffix != tc.wantSuffix || ok != tc.wantOK {
			t.Errorf("parseBlockMarker(%q) = (%d, %q, %v); want (%d, %q, %v)",
				tc.marker, gen, suffix, ok, tc.wantGen, tc.wantSuffix, tc.wantOK)
		}
	}
	// The trap itself, asserted: a string compare disagrees with the
	// numeric one for exactly the generation-rollover pair.
	if !("v10-pointer" < "v9-pointer") {
		t.Fatal("premise changed: \"v10-pointer\" is no longer lexically less than \"v9-pointer\"")
	}
	a, _ := ParseMarkerGeneration("v10-pointer")
	b, _ := ParseMarkerGeneration("v9-pointer")
	if !(a > b) {
		t.Errorf("v10-pointer must order AFTER v9-pointer; got %d vs %d", a, b)
	}
}
