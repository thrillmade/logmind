package timeline

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// recentLimitInProseRe extracts the integer a header prose string embeds
// immediately before "most recent" — the phrasing both header and
// archiveHeader use to state the SPEC §3.3 bound in words.
var recentLimitInProseRe = regexp.MustCompile(`(\d+) most recent`)

// recentLimitInProse parses the number out of prose (header/archiveHeader)
// via regex rather than checking for the presence of recentLimitStr — so a
// mutant that hardcodes a WRONG literal directly into the template (instead
// of interpolating recentLimitStr) is still caught on the actual digits, not
// waved through because *some* number is present.
func recentLimitInProse(t *testing.T, prose, name string) int {
	t.Helper()
	m := recentLimitInProseRe.FindStringSubmatch(prose)
	if m == nil {
		t.Fatalf("%s does not match %q; got:\n%s", name, recentLimitInProseRe.String(), prose)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("%s: unparseable number %q: %v", name, m[1], err)
	}
	return n
}

// TestHeaderProse_AgreesWithRecentLimit pins the HIGH finding from PR #301
// round 10: header and archiveHeader used to hardcode "50" as a literal
// string in the const templates, completely disconnected from RecentLimit.
// Bumping RecentLimit alone (e.g. `const RecentLimit = 50` -> `= 3`) left
// the prose asserting the OLD number in every generated docs/timeline.md
// and docs/timeline-archive.md, and no test caught it —
// `go test ./internal/timeline/...` stayed green while the rendered file
// lied about its own contents.
//
// header/archiveHeader are now derived from RecentLimit via recentLimitStr
// (timeline.go), so the two cannot drift apart by construction — this test
// extracts the NUMBER the rendered prose states and compares it against the
// constant, rather than merely checking recentLimitStr's presence, so it
// still catches a regression that reintroduces a hardcoded literal in
// either template while leaving RecentLimit itself untouched.
func TestHeaderProse_AgreesWithRecentLimit(t *testing.T) {
	if got := recentLimitInProse(t, header, "header"); got != RecentLimit {
		t.Errorf("header states %d most recent entries; RecentLimit = %d", got, RecentLimit)
	}
	if got := recentLimitInProse(t, archiveHeader, "archiveHeader"); got != RecentLimit {
		t.Errorf("archiveHeader states %d most recent entries; RecentLimit = %d", got, RecentLimit)
	}
}

// TestHandDocs_StateRecentLimit cross-checks the OTHER two restatements of
// the SPEC §3.3 bound that live in this repo's own hand-maintained
// documentation (not shipped, byte-frozen templates — see the round-10 fix
// report for why AGENTS.md.template / logmind-section.md / config.yml.template
// were deliberately left alone): docs/plan.md (architecture narrative) and
// docs/ai-agent-files.md (context-file reference). Both are plain repo docs
// this codebase fully owns, so a cheap substring check against RecentLimit
// is worth its cost — it fails loudly the next time RecentLimit moves and
// these sentences are not updated alongside it, rather than leaving a
// slow-drifting false claim for a reader to find.
func TestHandDocs_StateRecentLimit(t *testing.T) {
	checks := []struct {
		file string
		want string
	}{
		{"../../docs/plan.md", fmt.Sprintf("the %d most recent decisions", RecentLimit)},
		{"../../docs/plan.md", fmt.Sprintf("renders the %d most recent entries", RecentLimit)},
		{"../../docs/plan.md", fmt.Sprintf("caps the rendered timeline at %d entries", RecentLimit)},
		{"../../docs/ai-agent-files.md", fmt.Sprintf("older than the %d entries in", RecentLimit)},
	}
	for _, c := range checks {
		data, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatalf("read %s: %v", c.file, err)
		}
		if !strings.Contains(string(data), c.want) {
			t.Errorf("%s does not state RecentLimit (%d); want a substring %q", c.file, RecentLimit, c.want)
		}
	}
}
