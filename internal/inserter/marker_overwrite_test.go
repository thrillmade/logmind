// marker_overwrite_test.go — logmind#297: a partial write must never land on
// a whole user-owned artifact.
//
// The defect these pin: `self-update` called ReplaceMarkerBlock(entry.OldBody,
// entry.NewBody) — a BLOCK BODY where the WHOLE FILE belonged — and wrote the
// result over AGENTS.md. ReplaceMarkerBlock returns its first argument
// unchanged when it finds no markers, and a block body never contains the
// markers that bracket it, so the "refresh" wrote the fragment over the file:
// project overview, development commands, every line the repository wrote
// itself, gone.
//
// These tests deliberately do NOT assert what arguments anything is called
// with. An argument test passes its own mutation and still goes green the day
// the bug ships again; the invariant that actually protects the user is that
// the bytes outside the marker block survive the write.
package inserter

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/templates"
)

// realisticAgentsMD builds an AGENTS.md shaped like one a repository actually
// carries: prose ABOVE the logmind block and prose BELOW it, with the block in
// the middle. Both halves are what #297 destroyed.
func realisticAgentsMD(t *testing.T, blockBody string) string {
	t.Helper()
	const above = "# AGENTS.md\n\nThis is the canonical instruction file for AI coding agents.\n\n"
	const below = "\n\n## Project Overview\n\nlogmind is a decision-logging CLI.\n\n" +
		"## Development Commands\n\n```bash\ngo build ./cmd/logmind\n```\n\n" +
		"## clud-bug — Claude PR review\n\nReviews are automated.\n"
	return above + startMarker + blockBody + endMarker + below
}

// TestRefreshMarkerBlockFile_PreservesBytesOutsideBlock is the #297 invariant
// at the level the user was harmed: after a refresh, every byte above and
// below the marker block is exactly what it was.
func TestRefreshMarkerBlockFile_PreservesBytesOutsideBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	staleBody := "\n<!-- logmind-block-version: v1-pointer -->\nSTALE BLOCK BODY\n"
	original := realisticAgentsMD(t, staleBody)
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	freshBody, ok := ExtractMarkerBlock(templates.AgentsSlimTemplate())
	if !ok {
		t.Fatal("the bundled slim template carries no marker block")
	}
	if err := RefreshMarkerBlockFile(path, freshBody); err != nil {
		t.Fatalf("RefreshMarkerBlockFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	got := string(data)

	// The whole file must still be a whole file — this is the assertion that
	// goes red when a fragment is written over it.
	want := realisticAgentsMD(t, freshBody)
	if got != want {
		t.Fatalf("refresh did not rewrite the block in place:\n got: %q\nwant: %q", got, want)
	}

	// Named sentinels, so a failure says WHICH half was lost rather than
	// dumping two large strings.
	for _, sentinel := range []string{
		"This is the canonical instruction file for AI coding agents.", // above
		"## Project Overview",            // below
		"go build ./cmd/logmind",         // below
		"## clud-bug — Claude PR review", // below
	} {
		if !strings.Contains(got, sentinel) {
			t.Errorf("content outside the marker block was destroyed: %q is gone", sentinel)
		}
	}
	if strings.Contains(got, "STALE BLOCK BODY") {
		t.Error("the stale block body survived; the refresh did not happen")
	}
}

// TestRefreshMarkerBlockFile_RefusesMarkerlessFile is SPEC §5.2's rule made
// executable: "An artifact carrying no marker at all belongs to the user and
// MUST NOT be overwritten." The bytes must be identical afterwards, and the
// caller must get an error rather than a silent no-op.
func TestRefreshMarkerBlockFile_RefusesMarkerlessFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	// No logmind markers anywhere — entirely the user's file.
	original := "# My own AGENTS.md\n\nI wrote every line of this myself.\n\nUSER_SENTINEL\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := RefreshMarkerBlockFile(path, "SOME NEW BODY")
	if !errors.Is(err, ErrNoMarkerBlock) {
		t.Fatalf("RefreshMarkerBlockFile err = %v; want ErrNoMarkerBlock", err)
	}
	// The error has to name the file — a refusal the user cannot locate is
	// not a report (SPEC §3.4).
	if !strings.Contains(err.Error(), path) {
		t.Errorf("refusal %q does not name the file %q", err, path)
	}

	data, err2 := os.ReadFile(path)
	if err2 != nil {
		t.Fatalf("read back: %v", err2)
	}
	if string(data) != original {
		t.Fatalf("a markerless file was modified:\n got: %q\nwant: %q", string(data), original)
	}
}

// TestRefreshMarkerBlockFile_RefusesInvertedMarkers — end-before-start is
// malformed, never a legitimate state. replaceMarkerBlock returns such input
// unchanged, which is safe only if nobody writes the result; the file path
// must refuse it outright rather than rewrite the file with itself.
func TestRefreshMarkerBlockFile_RefusesInvertedMarkers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	original := "head\n" + endMarker + "\ninverted\n" + startMarker + "\ntail\nUSER_SENTINEL\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := RefreshMarkerBlockFile(path, "NEW BODY"); !errors.Is(err, ErrNoMarkerBlock) {
		t.Fatalf("RefreshMarkerBlockFile err = %v; want ErrNoMarkerBlock", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != original {
		t.Fatalf("a malformed file was modified:\n got: %q\nwant: %q", string(data), original)
	}
}

// TestExtractTemplateMarker_OwnershipStates pins the three-way answer #299
// turned on. The two extractors it replaced could not both be right, and the
// bug was that they were consulted separately.
func TestExtractTemplateMarker_OwnershipStates(t *testing.T) {
	for _, tc := range []struct {
		name    string
		text    string
		want    MarkerOwnership
		version string
		line    int
	}{
		{
			name:    "marker on line 1 is ours",
			text:    "# logmind-template-version: v5\nname: check-decisions\n",
			want:    MarkerOwned,
			version: "v5",
			line:    1,
		},
		{
			name: "no marker anywhere belongs to the user",
			text: "name: my own workflow\njobs: {}\n",
			want: MarkerAbsent,
		},
		{
			// The #299 reproduction. Under the deleted any-line extractor this
			// was "versioned"; under doctor's it was "markerless". It is now
			// neither — it is displaced, and every write path refuses it.
			name:    "marker on line 2 is displaced",
			text:    "# my org header — do not remove\n# logmind-template-version: v1\nname: x\n",
			want:    MarkerDisplaced,
			version: "v1",
			line:    2,
		},
		{
			name:    "marker far down the file is still displaced",
			text:    "a\nb\nc\n# logmind-template-version: v9\n",
			want:    MarkerDisplaced,
			version: "v9",
			line:    4,
		},
		{
			// Indented is not line-1-anchored: the regex is anchored at ^, so
			// leading whitespace makes it not ours. A user's YAML comment
			// nested inside a block must never claim the file for logmind.
			name: "indented marker is not ours",
			text: "  # logmind-template-version: v5\nname: x\n",
			want: MarkerAbsent,
		},
		{
			// The version token is the first non-space run, not the rest of
			// the line — the deleted prefix extractor returned "v1 junk" here
			// and doctor's regex returned "v1". One answer now.
			name:    "trailing junk is not part of the version",
			text:    "# logmind-template-version: v1 junk here\n",
			want:    MarkerOwned,
			version: "v1",
			line:    1,
		},
		{
			name:    "CRLF line endings do not leak into the token",
			text:    "# logmind-template-version: v5\r\nname: x\r\n",
			want:    MarkerOwned,
			version: "v5",
			line:    1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractTemplateMarker(tc.text)
			if got.Ownership != tc.want {
				t.Errorf("Ownership = %v; want %v", got.Ownership, tc.want)
			}
			if got.Version != tc.version {
				t.Errorf("Version = %q; want %q", got.Version, tc.version)
			}
			if got.Line != tc.line {
				t.Errorf("Line = %d; want %d", got.Line, tc.line)
			}
			if want := tc.want == MarkerOwned; got.Writable() != want {
				t.Errorf("Writable() = %v; want %v", got.Writable(), want)
			}
		})
	}
}

// TestExtractTemplateMarker_BundledTemplatesAreOwned is the control on the
// strict semantics: first-line-only is only the right rule if it recognises
// everything logmind actually writes. If a template ever grows a header above
// its marker, this fails loudly instead of that template silently becoming
// un-refreshable in every repo that has it.
func TestExtractTemplateMarker_BundledTemplatesAreOwned(t *testing.T) {
	names := templates.ListWorkflowTemplates()
	if len(names) == 0 {
		t.Fatal("no bundled workflow templates found — this control proves nothing")
	}
	for _, name := range names {
		got := ExtractTemplateMarker(templates.Workflow(name))
		if !got.Writable() {
			t.Errorf("bundled %s is not recognised as logmind's own: %+v "+
				"(its `# logmind-template-version:` marker must be line 1)", name, got)
		}
	}
}
