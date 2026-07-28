package skill

// sync.go implements the `logmind sync` loop-closer (B5b / G4.a).
//
// The skill suggestion cycle:
//
//   1. `logmind skill suggest` proposes new skills from decision patterns.
//   2. A human / agent promotes draft → `.claude/skills/<name>/SKILL.md`
//      (which also emits a sibling PROVENANCE.md skeleton — see
//      provenance.go).
//   3. clud-bug-review consumes those skills during PR review and writes
//      its findings to `docs/reviews/PR-<n>.md` per SPEC §6.2.
//   4. `logmind sync` (this file) walks those review files and updates
//      each cited skill's PROVENANCE.md so the author sees a measurable
//      signal that the skill is paying its keep.
//
// The format of `docs/reviews/PR-<n>.md` is fixed by SPEC §1.8.1
// (NORMATIVE template). We parse two surfaces:
//
//   - `<!-- review-sha: <40-char-sha> -->`: the load-bearing idempotency
//     key. We track which SHAs have already been applied to each
//     skill's PROVENANCE.md so re-running `logmind sync` after no new
//     reviews is a no-op.
//   - `**Skills cited:**` block: a markdown list of `- skill-name (N
//     findings)` lines that we sum into the `cited-by-clud-bug` counter.
//
// Citations that target a skill name not present under
// `.claude/skills/<name>/` are skipped with a warning rather than
// erroring out — the review file is the source of truth and may cite
// skills installed elsewhere (or removed since the review was written).
//
// This file also implements the three SPEC §3.9 flags the above loop
// shipped without: --since (ParseSinceDuration, plus the `Since` field
// on SyncOptions), --update-provenance (UpdateProvenance), and
// --write-drafts (WriteSkillDrafts). Each is a separate, additive
// entry point — none of them replace or gate the default Sync() path
// above, which is why a bare `logmind sync` (no flags) is untouched by
// their presence. See each function's doc comment for why it's a
// distinct code path rather than a rename/extension of Sync().

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/thrillmade/logmind/internal/atomicio"
	"github.com/thrillmade/logmind/internal/decisions"
	"github.com/thrillmade/logmind/internal/timeline"
)

// reviewSHARE matches the `<!-- review-sha: <40 hex chars> -->` comment
// from SPEC §1.8.1. The 40-char length is the SPEC contract (the
// committed HEAD SHA at review time); we anchor on it so a truncated
// SHA in a malformed file is rejected at parse time.
var reviewSHARE = regexp.MustCompile(`<!--\s*review-sha:\s*([0-9a-fA-F]{40})\s*-->`)

// skillsCitedHeaderRE matches the `**Skills cited:**` opening of the
// citation block. The trailing colon is required so we don't grab
// `**Skills cited a great deal**` from prose.
var skillsCitedHeaderRE = regexp.MustCompile(`^\*\*Skills cited:\*\*\s*$`)

// citationRE matches a single citation line. SPEC §1.8.1 template line:
//
//   - skill-name-1 (N findings)
//
// We accept any whitespace after the dash + a positive integer in the
// findings count. The name is the kebab-slug per SPEC §1.10.1
// frontmatter rules (`^[a-z][a-z0-9-]{0,62}$`); we don't re-validate
// the slug pattern here because the citation might use legacy formats
// the parser shouldn't reject.
var citationRE = regexp.MustCompile(`^-\s+([A-Za-z0-9_.-]+)\s+\((\d+)\s+findings?\)\s*$`)

// prFilenameRE matches `PR-<number>.md` (and only that) so we ignore
// stray markdown files in docs/reviews/ that aren't review writebacks.
// The number is captured so we can include it in warnings.
var prFilenameRE = regexp.MustCompile(`^PR-(\d+)\.md$`)

// appliedSHARE matches the bookkeeping entry we append to PROVENANCE.md
// to record which review SHAs have already been folded into the
// counter. Format: `applied-review-shas: [<sha1>, <sha2>, ...]` lives
// inside the YAML block.
var appliedSHARE = regexp.MustCompile(`^applied-review-shas:\s*\[(.*)\]\s*$`)

// citedCounterRE matches the `cited-by-clud-bug: <n>` line inside the
// YAML block. Captures the integer so we can read + bump it.
var citedCounterRE = regexp.MustCompile(`^cited-by-clud-bug:\s*(\d+)\s*$`)

// lastRefinedRE matches the `last-refined: "<iso-date>"` line. The
// quotes are optional (the skeleton uses `""`; an updated file may use
// the bare ISO date).
var lastRefinedRE = regexp.MustCompile(`^last-refined:\s*"?([\d-]*)"?\s*$`)

// Review represents a parsed `docs/reviews/PR-<n>.md` file.
//
// We deliberately keep this struct small — sync only needs the SHA
// (idempotency key) and the per-skill counts. Future tooling that
// wants to surface the full finding text can extend Review without
// touching the sync loop.
type Review struct {
	// PRNumber is the integer pulled from `PR-<n>.md`.
	PRNumber int
	// Path is the absolute path to the review file.
	Path string
	// HeadSHA is the 40-character hex string from `<!-- review-sha:
	// ... -->`. Empty when the comment was missing or malformed —
	// such files are skipped by `Sync` since they can't be
	// deduplicated.
	HeadSHA string
	// Citations maps `skill-name` → finding count for that PR.
	Citations map[string]int
}

// ParseReview reads a `docs/reviews/PR-<n>.md` file and returns the
// Review struct. Returns an error wrapping a sentinel when the file is
// malformed enough that nothing reliable can be extracted.
//
// Tolerance policy: we surface errors only for the load-bearing
// fields (PR number from filename, review-sha comment). Empty
// citation lists are valid — a clean PR with zero findings has no
// `**Skills cited:**` block.
func ParseReview(path string) (Review, error) {
	r := Review{Path: path, Citations: map[string]int{}}

	base := filepath.Base(path)
	m := prFilenameRE.FindStringSubmatch(base)
	if m == nil {
		return r, fmt.Errorf("filename %q does not match PR-<n>.md: %w",
			base, ErrMalformedReview)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		// Should be unreachable: prFilenameRE only captures digits.
		// Returning a wrapped sentinel keeps callers' branching
		// uniform.
		return r, fmt.Errorf("filename %q has non-numeric PR number: %w",
			base, ErrMalformedReview)
	}
	r.PRNumber = n

	data, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	body := string(data)

	if sha := reviewSHARE.FindStringSubmatch(body); sha != nil {
		// Normalise to lowercase so two reviewers that disagree on
		// hex casing produce the same idempotency key.
		r.HeadSHA = strings.ToLower(sha[1])
	} else {
		return r, fmt.Errorf("missing or malformed `<!-- review-sha: ... -->` in %s: %w",
			path, ErrMalformedReview)
	}

	parseCitations(body, r.Citations)
	return r, nil
}

// parseCitations walks `body` looking for the `**Skills cited:**`
// header and accumulates each subsequent `- skill-name (N findings)`
// line into out. Stops at the first blank line or non-citation line —
// SPEC §1.8.1 places the block immediately after the Summary line and
// terminates it with a blank line.
//
// We accumulate into the supplied map rather than returning a new one
// so callers control allocation. Subsequent occurrences of the same
// skill add their counts together (in case a review accidentally lists
// the same skill twice).
func parseCitations(body string, out map[string]int) {
	lines := strings.Split(body, "\n")
	in := false
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if !in {
			if skillsCitedHeaderRE.MatchString(trimmed) {
				in = true
			}
			continue
		}
		if strings.TrimSpace(trimmed) == "" {
			// Empty line terminates the citation block. Any later
			// dash-prefixed list (e.g., `### 🔴 Critical` body) is a
			// different surface and must not be folded in.
			return
		}
		m := citationRE.FindStringSubmatch(trimmed)
		if m == nil {
			// First non-citation line in the block also terminates.
			// Defensive against template drift while staying conservative.
			return
		}
		count, err := strconv.Atoi(m[2])
		if err != nil || count <= 0 {
			// Negative / zero / non-numeric counts are not meaningful
			// signal — skip rather than poison the counter.
			continue
		}
		out[m[1]] += count
	}
}

// SyncOptions controls behaviour of Sync.
type SyncOptions struct {
	// DryRun: when true, parse and aggregate but skip the
	// PROVENANCE.md writes. The summary still reflects what WOULD
	// have been written.
	DryRun bool
	// Now is the timestamp used for `last-refined`. Tests pin it to
	// a fixed instant; production callers pass time.Time{} to mean
	// "use time.Now()".
	Now time.Time
	// Warn receives one-line diagnostics for review files we skipped
	// or skills we couldn't update. Nil silences warnings — useful
	// for tests that pin success cases and don't want stderr noise.
	Warn func(string)
	// Since, when non-zero, restricts the docs/reviews/PR-*.md scan to
	// files whose mtime falls within [Now-Since, Now] — SPEC §3.9's
	// "limit scan to entries newer than <duration>". Zero (the
	// SyncOptions default) means "no filtering", which is exactly
	// today's unbounded-scan behaviour — so callers that never set
	// Since see byte-identical output to before this field existed.
	Since time.Duration
}

// SyncSummary captures what Sync did so the CLI layer can render a
// human-readable report.
type SyncSummary struct {
	// ReviewsScanned: total `PR-<n>.md` files we looked at.
	ReviewsScanned int
	// ReviewsApplied: PR review files whose SHA wasn't already in
	// the per-skill applied set (i.e., contributed at least one new
	// citation).
	ReviewsApplied int
	// SkillsUpdated: skills whose PROVENANCE.md was rewritten.
	SkillsUpdated int
	// CitationsAdded: sum of citation counts folded into provenance
	// across all updated skills.
	CitationsAdded int
	// Updates: per-skill diff, sorted by skill name for stable output.
	Updates []SkillSyncUpdate
}

// SkillSyncUpdate captures the per-skill diff that Sync recorded.
type SkillSyncUpdate struct {
	Name          string
	PreviousCount int
	NewCount      int
	NewReviewSHAs []string // sorted hex strings actually applied this run
}

// ErrMalformedReview is returned by ParseReview when a file under
// `docs/reviews/` can't be processed. Callers use errors.Is to
// distinguish "skip this file with a warning" from "fail loudly".
var ErrMalformedReview = errors.New("malformed review file")

// Sync walks `docs/reviews/PR-*.md`, parses each into a Review, and
// folds new citations into each cited skill's PROVENANCE.md. Returns
// a SyncSummary describing the work performed; per-file or per-skill
// problems are routed through opts.Warn and don't fail the whole run.
//
// Order matters here for determinism:
//
//   - Review files are sorted by filename so a multi-PR run is
//     reproducible across filesystems.
//   - Per-skill updates are sorted by skill name in the returned
//     summary so the CLI output is stable.
//   - applied-review-shas inside PROVENANCE.md are written in sorted
//     order so a re-run that adds one new SHA produces a minimal diff.
//
// Error semantics: returns a non-nil error only for filesystem failures
// that prevent the run from making forward progress (e.g., couldn't
// read the reviews dir at all). Per-file / per-skill issues route to
// opts.Warn.
func Sync(repoRoot string, opts SyncOptions) (SyncSummary, error) {
	summary := SyncSummary{}
	warn := opts.Warn
	if warn == nil {
		warn = func(string) {}
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	today := now.UTC().Format("2006-01-02")
	cutoff := sinceCutoff(now, opts.Since)

	reviewsDir := filepath.Join(repoRoot, "docs", "reviews")
	entries, err := os.ReadDir(reviewsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No reviews/ dir at all is a valid "nothing to do" state —
			// e.g., a freshly-init'd repo before clud-bug has run.
			return summary, nil
		}
		return summary, fmt.Errorf("read %s: %w", reviewsDir, err)
	}

	// Sort entries for deterministic order across filesystems
	// (macOS APFS is case-insensitive; Linux ext4 isn't).
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	// Bucket citations by skill so each skill's PROVENANCE.md is
	// rewritten once even when many PRs cite it. The bucket carries
	// the per-PR SHA so we can dedup against the on-disk applied set.
	type pending struct {
		Count int
		SHAs  []string
	}
	buckets := map[string]*pending{}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !prFilenameRE.MatchString(name) {
			// Quiet: docs/reviews/README.md or similar is allowed.
			continue
		}
		if !cutoff.IsZero() {
			// --since: a file outside the window is outside the SCAN,
			// per SPEC §3.9 — it's not merely excluded from the result,
			// it's never opened. mtime is the only recency signal a
			// PR-<n>.md file carries (§1.8.1's template has no embedded
			// date field), so we use it as-is. A stat failure here is
			// surprising enough (the entry just came from ReadDir) that
			// we warn and skip rather than silently including it, which
			// would quietly widen the window the caller asked for.
			info, ierr := e.Info()
			if ierr != nil {
				warn(fmt.Sprintf("skipping %s: stat: %v", name, ierr))
				continue
			}
			if info.ModTime().Before(cutoff) {
				continue
			}
		}
		summary.ReviewsScanned++
		path := filepath.Join(reviewsDir, name)

		rv, err := ParseReview(path)
		if err != nil {
			warn(fmt.Sprintf("skipping %s: %v", name, err))
			continue
		}
		if len(rv.Citations) == 0 {
			// A clean PR with zero citations doesn't bump anyone's
			// counter and doesn't need to be marked applied (since
			// it leaves no trace). Still counts as scanned.
			continue
		}
		for skillName, count := range rv.Citations {
			b, ok := buckets[skillName]
			if !ok {
				b = &pending{}
				buckets[skillName] = b
			}
			b.Count += count
			b.SHAs = append(b.SHAs, rv.HeadSHA)
		}
	}

	// Sort skill names for stable summary order.
	skillNames := make([]string, 0, len(buckets))
	for n := range buckets {
		skillNames = append(skillNames, n)
	}
	sort.Strings(skillNames)

	appliedReviewSet := map[string]struct{}{}
	for _, skillName := range skillNames {
		bucket := buckets[skillName]
		provPath := filepath.Join(SkillDir(repoRoot, skillName), "PROVENANCE.md")
		if _, err := os.Stat(provPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				warn(fmt.Sprintf(
					"skill %q cited in reviews but %s does not exist (skipping; create the skill or run `logmind skill new %s`)",
					skillName, provPath, skillName))
				continue
			}
			warn(fmt.Sprintf("skill %q: stat %s: %v", skillName, provPath, err))
			continue
		}

		body, err := os.ReadFile(provPath)
		if err != nil {
			warn(fmt.Sprintf("skill %q: read %s: %v", skillName, provPath, err))
			continue
		}
		prev := parseProvenance(string(body))

		// Idempotency gate: filter out SHAs we've already applied.
		// dedupSHAs also sorts the survivors so the on-disk applied
		// list stays canonical.
		newSHAs := filterNewSHAs(bucket.SHAs, prev.AppliedSHAs)
		if len(newSHAs) == 0 {
			// Nothing new to fold in for this skill — re-run no-op.
			continue
		}

		// Only the citations from NEW reviews increment the counter.
		// Rebuild the additive count by re-summing citations from
		// the filtered SHAs. Since the bucket lost which-citation-
		// came-from-which-SHA detail, we approximate by reading the
		// reviews again — cheaper than maintaining a parallel slice.
		addedCount, applied := recountForSHAs(reviewsDir, skillName, newSHAs, warn)
		if addedCount == 0 || len(applied) == 0 {
			continue
		}

		newCount := prev.CitedByCludBug + addedCount
		updated := rewriteProvenance(string(body), provenanceUpdate{
			NewCount:    newCount,
			LastRefined: today,
			AppliedSHAs: mergeAndSortSHAs(prev.AppliedSHAs, applied),
		})

		if !opts.DryRun {
			if err := atomicWriteFile(provPath, []byte(updated), 0o644); err != nil {
				warn(fmt.Sprintf("skill %q: write %s: %v", skillName, provPath, err))
				// Persist failed — leave appliedReviewSet, SkillsUpdated,
				// CitationsAdded, and Updates unchanged for this skill so
				// the summary reflects what actually landed on disk.
				continue
			}
		}

		// Track set of applied SHAs across the whole run for the
		// reviews-applied counter. Only counted after a successful
		// persist (or in --dry-run where the write is intentionally
		// skipped, not failed).
		for _, s := range applied {
			appliedReviewSet[s] = struct{}{}
		}

		summary.SkillsUpdated++
		summary.CitationsAdded += addedCount
		summary.Updates = append(summary.Updates, SkillSyncUpdate{
			Name:          skillName,
			PreviousCount: prev.CitedByCludBug,
			NewCount:      newCount,
			NewReviewSHAs: applied,
		})
	}

	summary.ReviewsApplied = len(appliedReviewSet)
	return summary, nil
}

// provenanceState captures the values we read out of PROVENANCE.md
// before rewriting. AppliedSHAs is the on-disk dedup set.
type provenanceState struct {
	CitedByCludBug int
	LastRefined    string
	AppliedSHAs    []string
}

// parseProvenance extracts the counter and applied-shas list from a
// PROVENANCE.md body. Missing fields default to zero / empty so a
// pristine skeleton parses cleanly.
//
// We deliberately do NOT use a full YAML parser here even though
// gopkg.in/yaml.v3 is on the dep list — the file is intentionally a
// hybrid (YAML block embedded in markdown narrative) and treating it
// as a free-form text rewrite preserves the human-edited prose below
// the block. A future migration to structured YAML would force users
// to choose between machine-readable + lossy prose, vs the current
// machine-friendly-enough-and-lossless approach.
func parseProvenance(body string) provenanceState {
	st := provenanceState{}
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if m := citedCounterRE.FindStringSubmatch(trimmed); m != nil {
			n, err := strconv.Atoi(m[1])
			if err == nil {
				st.CitedByCludBug = n
			}
			continue
		}
		if m := lastRefinedRE.FindStringSubmatch(trimmed); m != nil {
			st.LastRefined = m[1]
			continue
		}
		if m := appliedSHARE.FindStringSubmatch(trimmed); m != nil {
			// Parse the inline JSON-ish list of hex strings. Trim
			// quotes/spaces around each element; empty list is fine.
			raw := strings.TrimSpace(m[1])
			if raw == "" {
				continue
			}
			parts := strings.Split(raw, ",")
			for _, p := range parts {
				cleaned := strings.Trim(strings.TrimSpace(p), `"'`)
				if cleaned != "" {
					st.AppliedSHAs = append(st.AppliedSHAs, strings.ToLower(cleaned))
				}
			}
		}
	}
	return st
}

// provenanceUpdate carries the new field values into rewriteProvenance.
type provenanceUpdate struct {
	NewCount    int
	LastRefined string
	AppliedSHAs []string
}

// rewriteProvenance returns the body with the cited-by-clud-bug,
// last-refined, and applied-review-shas lines updated to reflect
// upd. If applied-review-shas was absent from the input, we inject it
// just after the refinement-history line so subsequent re-reads pick
// it up.
func rewriteProvenance(body string, upd provenanceUpdate) string {
	lines := strings.Split(body, "\n")
	sawApplied := false
	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if citedCounterRE.MatchString(trimmed) {
			lines[i] = fmt.Sprintf("cited-by-clud-bug: %d", upd.NewCount)
			continue
		}
		if lastRefinedRE.MatchString(trimmed) {
			lines[i] = fmt.Sprintf(`last-refined: "%s"`, upd.LastRefined)
			continue
		}
		if appliedSHARE.MatchString(trimmed) {
			lines[i] = fmt.Sprintf("applied-review-shas: [%s]",
				renderSHAs(upd.AppliedSHAs))
			sawApplied = true
		}
	}
	if !sawApplied {
		// Inject the bookkeeping line into the YAML block. The block
		// is delimited by ```yaml ... ``` per the skeleton template.
		// We add the line just before the closing fence so it lives
		// inside the YAML block (and so the doc above stays human-
		// readable).
		out := make([]string, 0, len(lines)+1)
		injected := false
		for _, line := range lines {
			trimmed := strings.TrimRight(line, "\r")
			if !injected && trimmed == "```" {
				// First closing fence after the opening ```yaml is
				// the one we want. We don't try to be too clever —
				// PROVENANCE.md only has one fenced block per the
				// skeleton, and a hand-edited file with multiple
				// blocks will still get the line injected somewhere
				// readable.
				out = append(out,
					fmt.Sprintf("applied-review-shas: [%s]", renderSHAs(upd.AppliedSHAs)))
				injected = true
			}
			out = append(out, line)
		}
		return strings.Join(out, "\n")
	}
	return strings.Join(lines, "\n")
}

// renderSHAs returns the comma-separated list rendering used inside
// the `applied-review-shas: [...]` line. The SHAs are quoted so they
// parse as strings even when fed to a real YAML reader.
func renderSHAs(shas []string) string {
	if len(shas) == 0 {
		return ""
	}
	out := make([]string, len(shas))
	for i, s := range shas {
		out[i] = fmt.Sprintf(`"%s"`, s)
	}
	return strings.Join(out, ", ")
}

// filterNewSHAs returns the elements of incoming that aren't already
// in applied, with each SHA deduplicated within incoming and sorted
// for stable output.
func filterNewSHAs(incoming, applied []string) []string {
	if len(incoming) == 0 {
		return nil
	}
	have := map[string]struct{}{}
	for _, s := range applied {
		have[strings.ToLower(s)] = struct{}{}
	}
	seen := map[string]struct{}{}
	var out []string
	for _, s := range incoming {
		l := strings.ToLower(s)
		if _, ok := have[l]; ok {
			continue
		}
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// mergeAndSortSHAs returns the union of prior + added, dedup'd and
// sorted. Used when persisting the new applied-review-shas list.
func mergeAndSortSHAs(prior, added []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range append(append([]string{}, prior...), added...) {
		l := strings.ToLower(s)
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// recountForSHAs re-walks the reviews dir picking up just the citation
// counts for `skillName` from PR files whose `<!-- review-sha -->`
// matches one of `shas`. Returns the additive count + the SHA list
// actually applied (a SHA in `shas` whose review either was missing or
// no longer cites the skill is dropped).
//
// This is structurally O(reviews × shas) which is fine — both factors
// stay tiny in practice (a busy repo has dozens of reviews, not
// millions) and the alternative (carrying per-citation provenance in
// the in-memory bucket) costs more code than it saves cycles.
func recountForSHAs(reviewsDir, skillName string, shas []string, warn func(string)) (int, []string) {
	if len(shas) == 0 {
		return 0, nil
	}
	want := map[string]struct{}{}
	for _, s := range shas {
		want[strings.ToLower(s)] = struct{}{}
	}
	entries, err := os.ReadDir(reviewsDir)
	if err != nil {
		warn(fmt.Sprintf("recount: %v", err))
		return 0, nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	total := 0
	hitSet := map[string]struct{}{}
	for _, e := range entries {
		if e.IsDir() || !prFilenameRE.MatchString(e.Name()) {
			continue
		}
		rv, err := ParseReview(filepath.Join(reviewsDir, e.Name()))
		if err != nil {
			// Already warned on the first pass.
			continue
		}
		if _, ok := want[rv.HeadSHA]; !ok {
			continue
		}
		if cnt, ok := rv.Citations[skillName]; ok {
			total += cnt
			hitSet[rv.HeadSHA] = struct{}{}
		}
	}
	out := make([]string, 0, len(hitSet))
	for s := range hitSet {
		out = append(out, s)
	}
	sort.Strings(out)
	return total, out
}

// atomicWriteFile writes to a temp sibling + renames, via the shared
// internal/atomicio.WriteFile primitive. Avoids leaving a half-written
// PROVENANCE.md if the process dies mid-write — same discipline as
// Python v0.6.16's atomic_io.write_text.
//
// Held as a package-level var so tests can swap in a failing writer
// that exercises the "persist failed, don't count the SHA" path in
// Sync (review #135 / Bug 1).
var atomicWriteFile func(path string, data []byte, perm os.FileMode) error = atomicio.WriteFile

// FormatSummary renders a human-readable report of a Sync run for the
// CLI layer. Kept here (alongside the data shape) so future tooling
// reusing the package gets a consistent render.
func FormatSummary(w io.Writer, s SyncSummary, dryRun bool) {
	prefix := ""
	if dryRun {
		prefix = "(dry-run) "
	}
	if s.SkillsUpdated == 0 {
		fmt.Fprintf(w, "%sno skills updated (%d review file(s) scanned)\n",
			prefix, s.ReviewsScanned)
		return
	}
	fmt.Fprintf(w,
		"%s%d skill(s) updated · %d citation(s) added · %d/%d review(s) applied\n",
		prefix, s.SkillsUpdated, s.CitationsAdded, s.ReviewsApplied, s.ReviewsScanned)
	for _, u := range s.Updates {
		shasPart := ""
		if len(u.NewReviewSHAs) > 0 {
			shortShas := make([]string, len(u.NewReviewSHAs))
			for i, sha := range u.NewReviewSHAs {
				if len(sha) >= 7 {
					shortShas[i] = sha[:7]
				} else {
					shortShas[i] = sha
				}
			}
			shasPart = fmt.Sprintf(" [+%s]", strings.Join(shortShas, ", "))
		}
		fmt.Fprintf(w, "  - %s: %d → %d%s\n",
			u.Name, u.PreviousCount, u.NewCount, shasPart)
	}
}

// --- --since ---------------------------------------------------------
//
// SPEC §3.9: `logmind sync [--since <duration>] ...` — "limit scan to
// entries newer than <duration> (e.g. 7d, 30d)". Go's time.ParseDuration
// has no "d" (day) unit, so the shorthand SPEC actually uses (`7d`,
// `30d`) isn't a valid Go duration string on its own. We handle the
// day-count form explicitly and fall back to time.ParseDuration for
// everything else (`24h`, `90m`, `1h30m`, ...) so a caller who happens
// to already think in Go-duration syntax isn't punished for it.
//
// sinceDurationRE deliberately does NOT accept w/m/y suffixes the way
// `logmind skill suggest --since` does (internal/cli/skill.go's
// sinceRE): "m" would collide with Go's own duration unit for minutes,
// and the SPEC text for `sync` only specifies day-shorthand examples.
// Silently guessing "m" means "month" here — when time.ParseDuration
// would otherwise have parsed "5m" as five minutes — is exactly the
// kind of ambiguity that produces a flag which lies about what it did.
var sinceDurationRE = regexp.MustCompile(`^(\d+)d$`)

// ParseSinceDuration parses a `--since` value per SPEC §3.9. Malformed
// input (empty string, non-positive count, unrecognized unit, garbage)
// is a hard error — never silently normalized to zero. A `--since`
// value that's silently ignored would make `logmind sync --since 7d`
// quietly scan the entire history while claiming to have honored the
// 7-day window; that's the lying-flag failure mode this build must
// avoid (see PR #247, which deleted five flags for exactly this
// reason).
func ParseSinceDuration(s string) (time.Duration, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return 0, fmt.Errorf("--since: empty duration (want e.g. \"7d\", \"30d\", \"24h\")")
	}
	if m := sinceDurationRE.FindStringSubmatch(trimmed); m != nil {
		days, err := strconv.Atoi(m[1])
		if err != nil || days <= 0 {
			return 0, fmt.Errorf("--since %q: day count must be a positive integer", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("--since %q: not a valid duration (want e.g. \"7d\", \"30d\", \"24h\"): %w", s, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("--since %q: duration must be positive", s)
	}
	return d, nil
}

// sinceCutoff turns a (now, since) pair into the absolute instant a
// scan should stop at. Zero Duration means "no filtering", represented
// by the zero time.Time so callers can test cutoff.IsZero() rather than
// re-deriving the "was Since even set" question.
func sinceCutoff(now time.Time, since time.Duration) time.Time {
	if since <= 0 {
		return time.Time{}
	}
	return now.Add(-since)
}

// --- --update-provenance ----------------------------------------------
//
// UpdateProvenance implements the SPEC §3.9 / §1.11 "refreshed
// PROVENANCE.md" surface. Unlike the legacy Sync() above (which folds
// docs/reviews/PR-*.md citations into a pre-existing, non-SPEC
// `<!-- logmind:provenance v1 -->` skeleton — see provenance.go — this
// writes the NORMATIVE §1.11.1 template byte-for-byte, sourced from all
// three surfaces §3.9 names:
//
//   - `.claude/skills/.clud-bug.json` `usage[<slug>].citations` (§1.12)
//   - `docs/decisions.md` + `docs/decisions-branches/*.md` (§1.3/§1.4),
//     matched against each skill's slug to build "Derived from
//     decisions"
//   - docs/reviews/PR-*.md is deliberately NOT re-read here: its
//     citation signal is already the legacy Sync() path above, and
//     folding it into a *second*, differently-shaped counter here would
//     just create two disagreeing numbers for the same thing. The
//     `.clud-bug.json` usage counters are the SPEC's own primary
//     citation source for PROVENANCE.md (§1.11.1 sources `usage[<slug>]
//     .citations` explicitly); docs/reviews/PR-*.md is the legacy
//     bridge Sync() was built around before that JSON surface existed.
//
// This is a genuinely additive code path, gated entirely behind
// --update-provenance: bare `logmind sync` never calls this function,
// so its default-no-flags behaviour (the legacy skeleton format) is
// untouched.
type ProvenanceOptions struct {
	// DryRun: compute the refresh but don't write PROVENANCE.md.
	DryRun bool
	// Now is the clock used for "Last refined" and --since. Zero means
	// time.Now().
	Now time.Time
	// Since restricts which decision entries count toward "Derived
	// from decisions" (by entry date) and which `.clud-bug.json` usage
	// counters count toward "Cited by clud-bug" (by last_cited) to the
	// [Now-Since, Now] window. Zero means unrestricted — the full
	// history.
	Since time.Duration
	// Warn receives one-line diagnostics. Nil silences them.
	Warn func(string)
}

// ProvenanceUpdate is the per-skill result of a refresh, for CLI
// reporting.
type ProvenanceUpdate struct {
	Name          string
	PrevCitations int
	NewCitations  int
	DecisionRefs  int
	// Migrated is true when the skill had no prior SPEC §1.11.1
	// PROVENANCE.md (either none at all, or the pre-existing
	// non-SPEC `<!-- logmind:provenance v1 -->` skeleton) and this run
	// wrote the first conformant one.
	Migrated bool
}

// ProvenanceSummary captures what UpdateProvenance did.
type ProvenanceSummary struct {
	SkillsScanned   int
	SkillsRefreshed int
	Updates         []ProvenanceUpdate
}

// cludBugUsageEntry mirrors the `usage[<slug>]` object in SPEC §1.12.1.
// We only need the two fields PROVENANCE.md's template surfaces;
// unknown/absent fields (this repo's own `.claude/skills/.clud-bug.json`
// is schema v1 and has no `usage` block at all) round-trip as zero
// values rather than erroring, since a manifest that predates a field
// is not malformed — it's just older.
type cludBugUsageEntry struct {
	Citations int    `json:"citations"`
	LastCited string `json:"last_cited"`
}

// cludBugManifest is the subset of SPEC §1.12.1's `.clud-bug.json`
// schema UpdateProvenance reads.
type cludBugManifest struct {
	Usage map[string]cludBugUsageEntry `json:"usage"`
}

// loadCludBugManifest reads `.claude/skills/.clud-bug.json`. A missing
// file is not an error — a repo that hasn't run clud-bug yet simply has
// zero citations for every skill.
func loadCludBugManifest(repoRoot string) (cludBugManifest, error) {
	path := filepath.Join(repoRoot, ".claude", "skills", ".clud-bug.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cludBugManifest{}, nil
		}
		return cludBugManifest{}, fmt.Errorf("read %s: %w", path, err)
	}
	var m cludBugManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return cludBugManifest{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, nil
}

// listSkillDirs returns the sorted names of every directory under
// SkillsDir(repoRoot) that has a SKILL.md — i.e., every installed
// skill, regardless of whether `.clud-bug.json` also lists it in
// `installed[]` (a repo's clud-bug manifest can drift from the skills
// actually on disk; PROVENANCE.md is a per-skill file, so we drive off
// the filesystem, the same source loadExistingSkillNames in suggest.go
// uses).
func listSkillDirs(repoRoot string) []string {
	skillsDir := SkillsDir(repoRoot)
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if st, err := os.Stat(filepath.Join(skillsDir, e.Name(), "SKILL.md")); err == nil && st.Mode().IsRegular() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// skillFrontmatterSourceRE matches the `source:` key inside a SKILL.md
// frontmatter block (SPEC §1.10.1).
var skillFrontmatterSourceRE = regexp.MustCompile(`(?m)^source:\s*(\S+)\s*$`)

// readSkillSource returns the skill's own `source:` frontmatter value
// (SPEC §1.10.1: manual | logmind-derived | skills-sh | clud-bug-baseline)
// for use as PROVENANCE.md's `**Source:**` field. Falls back to
// "manual" when the SKILL.md is unreadable or doesn't declare one —
// every real SKILL.md in this repo predates the `source:` key (the
// v0.4.0 basicTemplate in scaffold.go doesn't emit it either), so
// "manual" (the most common real-world case: a human wrote it) is a
// more honest default than leaving the field blank.
func readSkillSource(repoRoot, name string) string {
	data, err := os.ReadFile(MDPath(repoRoot, name))
	if err != nil {
		return "manual"
	}
	fm := extractFrontmatter(string(data))
	if m := skillFrontmatterSourceRE.FindStringSubmatch(fm); m != nil {
		return m[1]
	}
	return "manual"
}

// extractFrontmatter returns the content between the opening and
// closing `---` fences of a SKILL.md, or "" when the file doesn't open
// with a frontmatter block. Scoping the source-field regex to just this
// slice (rather than the whole file) avoids a false match on the word
// "source" appearing in prose below the fence.
func extractFrontmatter(body string) string {
	if !strings.HasPrefix(body, "---\n") {
		return ""
	}
	rest := body[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end == -1 {
		return ""
	}
	return rest[:end]
}

// warnWriter adapts a `func(string)` warn callback to io.Writer so it
// can be handed to APIs (like internal/decisions.Collect) that want a
// writer for diagnostics.
type warnWriter struct {
	warn func(string)
}

func (w warnWriter) Write(p []byte) (int, error) {
	if w.warn != nil {
		if s := strings.TrimRight(string(p), "\n"); s != "" {
			w.warn(s)
		}
	}
	return len(p), nil
}

// decisionRefsForSkill scans docs/decisions.md + docs/decisions-branches/
// *.md + docs/decisions-archive.md (via internal/decisions.Collect, the
// same multi-source reader `logmind timeline` uses) for entries whose
// title mentions the skill by name, and returns SPEC §1.11.1 "Derived
// from decisions" anchor lines (`docs/<file>#<title-slug>`), sorted for
// stable output.
//
// Matching heuristic: case-insensitive substring match of the skill's
// slug (`avoid-naked-fetch`) OR its space-separated form
// (`avoid naked fetch`) against the decision's title. This is a
// best-effort heuristic — the SPEC only says the list is "computed from
// docs/decisions*.md heading anchors" without specifying a match rule —
// but it directly matches how a human would title a decision that
// introduced or refined a named skill ("Add avoid-naked-fetch skill",
// "Refine the avoid naked fetch guidance", ...), and it errs toward
// under-matching (an honest empty list) rather than over-matching
// (fabricating provenance).
//
// The anchor slug uses internal/timeline.Slugify on the title alone
// (not the full "YYYY-MM-DD HH:MM - title" heading line) to match the
// SPEC template's literal `<title-slug>` placeholder name.
func decisionRefsForSkill(repoRoot, skillName string, cutoff time.Time, warn func(string)) []string {
	entries, err := decisions.Collect(filepath.Join(repoRoot, "docs"), warnWriter{warn})
	if err != nil {
		if warn != nil {
			warn(fmt.Sprintf("derived-from-decisions: %v", err))
		}
		return nil
	}
	needleHyphen := strings.ToLower(skillName)
	needleSpaced := strings.ToLower(strings.ReplaceAll(skillName, "-", " "))

	seen := map[string]struct{}{}
	var refs []string
	for _, e := range entries {
		if !cutoff.IsZero() && e.Date.Before(cutoff) {
			continue
		}
		titleLower := strings.ToLower(e.Title)
		if !strings.Contains(titleLower, needleHyphen) && !strings.Contains(titleLower, needleSpaced) {
			continue
		}
		anchor := fmt.Sprintf("docs/%s#%s", e.SourcePath, timeline.Slugify(e.Title))
		if _, dup := seen[anchor]; dup {
			continue
		}
		seen[anchor] = struct{}{}
		refs = append(refs, anchor)
	}
	sort.Strings(refs)
	return refs
}

// provenanceSpecState is what parseSpecProvenance extracts from an
// existing PROVENANCE.md that's already in the SPEC §1.11.1 format.
type provenanceSpecState struct {
	IsSpecFormat bool
	Source       string
	Citations    int
	DecisionRefs []string
	History      []string
}

var (
	specMarkerRE     = regexp.MustCompile(`<!--\s*maintained-by:\s*logmind sync\s*-->`)
	specSourceRE     = regexp.MustCompile(`^\*\*Source:\*\*\s*(.*)$`)
	specCitationsRE  = regexp.MustCompile(`^\*\*Cited by clud-bug:\*\*\s*(\d+)\s+times?\s*$`)
	specDerivedHdrRE = regexp.MustCompile(`^\*\*Derived from decisions:\*\*\s*$`)
	specHistoryHdrRE = regexp.MustCompile(`^\*\*Refinement history:\*\*\s*$`)
	specBulletRE     = regexp.MustCompile(`^-\s+(.*)$`)
)

// parseSpecProvenance reads an existing PROVENANCE.md body. Files that
// don't carry the `<!-- maintained-by: logmind sync -->` marker —
// because they're the pre-existing `<!-- logmind:provenance v1 -->`
// skeleton (provenance.go), or because no file exists yet — report
// IsSpecFormat=false and every other field at its zero value, which
// UpdateProvenance treats as "first SPEC-conformant refresh" (a
// migration, not an update).
func parseSpecProvenance(body string) provenanceSpecState {
	st := provenanceSpecState{}
	if !specMarkerRE.MatchString(body) {
		return st
	}
	st.IsSpecFormat = true
	section := ""
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimRight(raw, "\r")
		switch {
		case specSourceRE.MatchString(line):
			st.Source = specSourceRE.FindStringSubmatch(line)[1]
			section = ""
		case specCitationsRE.MatchString(line):
			n, _ := strconv.Atoi(specCitationsRE.FindStringSubmatch(line)[1])
			st.Citations = n
			section = ""
		case specDerivedHdrRE.MatchString(line):
			section = "derived"
		case specHistoryHdrRE.MatchString(line):
			section = "history"
		case section == "derived" && specBulletRE.MatchString(line):
			st.DecisionRefs = append(st.DecisionRefs, specBulletRE.FindStringSubmatch(line)[1])
		case section == "history" && specBulletRE.MatchString(line):
			st.History = append(st.History, specBulletRE.FindStringSubmatch(line)[1])
		}
	}
	return st
}

// equalStringSlices reports whether a and b hold the same elements in
// the same order. Both DecisionRefs inputs to this comparison are
// always sorted before being written, so order-sensitivity here doesn't
// cause spurious "changed" diffs across re-runs.
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// diffProvenance decides whether a refresh actually changes anything
// worth writing, and if so, the one-line refinement-history summary to
// append. Re-running UpdateProvenance with no new citations, no new
// matching decisions, and no source change is a no-op — same
// idempotency discipline as the legacy Sync() above.
func diffProvenance(prev provenanceSpecState, citations int, refs []string, source string) (changed bool, summary string) {
	if !prev.IsSpecFormat {
		return true, "Migrated to SPEC §1.11.1 provenance format via `logmind sync --update-provenance`"
	}
	var parts []string
	if prev.Citations != citations {
		parts = append(parts, fmt.Sprintf("cited-by-clud-bug %d → %d", prev.Citations, citations))
	}
	if !equalStringSlices(prev.DecisionRefs, refs) {
		parts = append(parts, fmt.Sprintf("derived-from-decisions %d → %d entries", len(prev.DecisionRefs), len(refs)))
	}
	if prev.Source != source {
		parts = append(parts, fmt.Sprintf("source %q → %q", prev.Source, source))
	}
	if len(parts) == 0 {
		return false, ""
	}
	return true, strings.Join(parts, "; ")
}

// renderProvenanceMD renders the SPEC §1.11.1 NORMATIVE template
// byte-for-byte.
func renderProvenanceMD(name, source string, citations int, lastRefined string, refs, history []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Provenance for %s\n", name)
	b.WriteString("<!-- maintained-by: logmind sync -->\n\n")
	fmt.Fprintf(&b, "**Source:** %s\n", source)
	fmt.Fprintf(&b, "**Last refined:** %s\n", lastRefined)
	fmt.Fprintf(&b, "**Cited by clud-bug:** %d times\n\n", citations)
	b.WriteString("**Derived from decisions:**\n")
	for _, r := range refs {
		fmt.Fprintf(&b, "- %s\n", r)
	}
	b.WriteString("\n**Refinement history:**\n")
	for _, h := range history {
		fmt.Fprintf(&b, "- %s\n", h)
	}
	return b.String()
}

// UpdateProvenance refreshes every installed skill's PROVENANCE.md into
// the SPEC §1.11.1 format. See the package doc comment above this type
// block for the source-reconciliation design.
func UpdateProvenance(repoRoot string, opts ProvenanceOptions) (ProvenanceSummary, error) {
	warn := opts.Warn
	if warn == nil {
		warn = func(string) {}
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	today := now.UTC().Format("2006-01-02")
	cutoff := sinceCutoff(now, opts.Since)

	manifest, err := loadCludBugManifest(repoRoot)
	if err != nil {
		return ProvenanceSummary{}, err
	}

	names := listSkillDirs(repoRoot)
	summary := ProvenanceSummary{}
	for _, name := range names {
		summary.SkillsScanned++

		citations := 0
		if u, ok := manifest.Usage[name]; ok {
			switch {
			case cutoff.IsZero():
				citations = u.Citations
			case u.LastCited != "":
				if lc, perr := time.Parse(time.RFC3339, u.LastCited); perr == nil && !lc.Before(cutoff) {
					citations = u.Citations
				}
				// Parseable-but-stale, or unparseable, both fall through
				// to citations staying 0: --since asked us to limit the
				// scan, and we can't honestly claim a citation count is
				// "newer than <duration>" without a usable timestamp.
			}
		}

		refs := decisionRefsForSkill(repoRoot, name, cutoff, warn)
		source := readSkillSource(repoRoot, name)

		provPath := filepath.Join(SkillDir(repoRoot, name), "PROVENANCE.md")
		existing, readErr := os.ReadFile(provPath)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			warn(fmt.Sprintf("skill %q: read %s: %v", name, provPath, readErr))
			continue
		}
		prev := parseSpecProvenance(string(existing))

		changed, historyNote := diffProvenance(prev, citations, refs, source)
		if !changed {
			continue
		}
		history := append(append([]string{}, prev.History...), fmt.Sprintf("%s: %s", today, historyNote))
		rendered := renderProvenanceMD(name, source, citations, today, refs, history)

		if !opts.DryRun {
			if err := atomicWriteFile(provPath, []byte(rendered), 0o644); err != nil {
				warn(fmt.Sprintf("skill %q: write %s: %v", name, provPath, err))
				continue
			}
		}

		summary.SkillsRefreshed++
		summary.Updates = append(summary.Updates, ProvenanceUpdate{
			Name:          name,
			PrevCitations: prev.Citations,
			NewCitations:  citations,
			DecisionRefs:  len(refs),
			Migrated:      !prev.IsSpecFormat,
		})
	}
	return summary, nil
}

// FormatProvenanceSummary renders a human-readable report of an
// UpdateProvenance run.
func FormatProvenanceSummary(w io.Writer, s ProvenanceSummary, dryRun bool) {
	prefix := ""
	if dryRun {
		prefix = "(dry-run) "
	}
	fmt.Fprintf(w, "%s%d skill(s) refreshed (%d scanned)\n", prefix, s.SkillsRefreshed, s.SkillsScanned)
	for _, u := range s.Updates {
		tag := ""
		if u.Migrated {
			tag = " [migrated to SPEC §1.11.1]"
		}
		fmt.Fprintf(w, "  - %s: cited-by-clud-bug %d → %d, %d decision ref(s)%s\n",
			u.Name, u.PrevCitations, u.NewCitations, u.DecisionRefs, tag)
	}
	fmt.Fprintf(w, "ok sync-provenance: %s%d skill(s) refreshed/%d scanned\n",
		prefix, s.SkillsRefreshed, s.SkillsScanned)
}

// --- --write-drafts ----------------------------------------------------
//
// WriteSkillDrafts implements the SPEC §3.9 / §1.9 "skill candidate
// drafts under docs/skills-derived/" surface. It reuses
// SuggestFromDecisions (suggest.go) — the same decision-pattern
// heuristic `logmind skill suggest` uses — rather than re-implementing
// pattern detection, since §1.9 already names both `logmind skill
// suggest --write-drafts` and `logmind sync --write-drafts` as
// producers of the same file shape and the SPEC gives no reason to
// expect them to disagree on WHICH patterns are skill-worthy.
//
// It does NOT reuse suggest_llm.go's WriteDrafts/FormatIssueDraft: those
// render a GH-issue-body (no frontmatter at all) into an
// arbitrary caller-chosen directory — the shape `logmind skill suggest
// --write-drafts <dir>` promises. SPEC §1.9 requires a different, fixed
// shape: Markdown with YAML frontmatter setting `source: logmind-derived`
// and `status: candidate`, at the fixed path
// docs/skills-derived/<name>.md. Reusing the issue-body renderer here
// would produce a file at the right path with the wrong contents — the
// same "flag that doesn't do what it says" failure this build is
// required to avoid, just relocated from "doesn't write" to "writes the
// wrong thing".
const (
	// syncDraftsDefaultSinceDays mirrors `logmind skill suggest`'s own
	// --since default (internal/cli/skill.go newSkillSuggestCmd) so the
	// two producers of docs/skills-derived/*.md agree on "recent"
	// absent an explicit --since override.
	syncDraftsDefaultSinceDays = 30
	// syncDraftsMinDecisions mirrors `logmind skill suggest`'s
	// --min-decisions default.
	syncDraftsMinDecisions = 3
	// syncDraftsTopN mirrors `logmind skill suggest`'s --top default.
	syncDraftsTopN = 5
)

// DraftOptions controls WriteSkillDrafts.
type DraftOptions struct {
	// DryRun: compute the candidate list but don't write files.
	DryRun bool
	// Now is the clock used for --since. Zero means time.Now().
	Now time.Time
	// Since restricts the decision-log lookback window. Zero means the
	// syncDraftsDefaultSinceDays fallback (30d), matching `logmind
	// skill suggest`'s own default.
	Since time.Duration
	// Warn receives one-line diagnostics. Nil silences them.
	Warn func(string)
}

// DraftSummary captures what WriteSkillDrafts did.
type DraftSummary struct {
	CandidatesConsidered int
	DraftsWritten        []string // skill slugs, in the order suggested
}

// draftsDir is the SPEC §1.9 canonical location for skill-candidate
// drafts.
func draftsDir(repoRoot string) string {
	return filepath.Join(repoRoot, "docs", "skills-derived")
}

// renderSkillDraftMD renders a SPEC §1.9/§1.10.1-conformant draft: YAML
// frontmatter (source: logmind-derived, status: candidate — the two
// keys §1.9 says a draft's frontmatter MUST set) followed by a
// human-readable body carrying the same evidence
// `logmind skill suggest` shows on stdout.
func renderSkillDraftMD(s Suggestion) string {
	// Frontmatter values are single-line YAML scalars; draft_description
	// is generated text (not user input) but collapsing defensively
	// costs nothing and avoids ever emitting frontmatter that doesn't
	// parse.
	desc := strings.ReplaceAll(strings.TrimSpace(s.DraftDescription), "\n", " ")

	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", s.Slug)
	fmt.Fprintf(&b, "description: %s\n", desc)
	b.WriteString("source: logmind-derived\n")
	b.WriteString("status: candidate\n")
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s\n\n", s.Slug)
	fmt.Fprintf(&b, "Trigger: when working on `%s` — pattern emerged in %d recent decisions.\n\n",
		s.Phrase, s.DecisionCount)
	b.WriteString("## Evidence (auto-extracted from decision log)\n\n")
	for _, e := range s.Evidence {
		fmt.Fprintf(&b, "- `%s`: %s\n", e.File, e.Snippet)
	}
	b.WriteString("\n_Generated by `logmind sync --write-drafts` (SPEC §1.9). This is a " +
		"candidate awaiting human review — promote it into `.claude/skills/<name>/` " +
		"only after editing the trigger and body; tools MUST NOT load draft skills " +
		"as active skills._\n")
	return b.String()
}

// WriteSkillDrafts scans recent decisions for skill-worthy patterns and
// writes one docs/skills-derived/<slug>.md per candidate.
func WriteSkillDrafts(repoRoot string, opts DraftOptions) (DraftSummary, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	sinceDays := syncDraftsDefaultSinceDays
	if opts.Since > 0 {
		sinceDays = int(math.Ceil(opts.Since.Hours() / 24))
		if sinceDays < 1 {
			sinceDays = 1
		}
	}

	suggestions := SuggestFromDecisions(repoRoot, sinceDays, syncDraftsMinDecisions, syncDraftsTopN, now)
	summary := DraftSummary{CandidatesConsidered: len(suggestions)}
	if len(suggestions) == 0 {
		return summary, nil
	}

	if !opts.DryRun {
		if err := os.MkdirAll(draftsDir(repoRoot), 0o755); err != nil {
			return summary, fmt.Errorf("mkdir %s: %w", draftsDir(repoRoot), err)
		}
	}

	for _, s := range suggestions {
		path := filepath.Join(draftsDir(repoRoot), s.Slug+".md")
		body := renderSkillDraftMD(s)
		if !opts.DryRun {
			if err := atomicWriteFile(path, []byte(body), 0o644); err != nil {
				if opts.Warn != nil {
					opts.Warn(fmt.Sprintf("draft %q: write %s: %v", s.Slug, path, err))
				}
				continue
			}
		}
		summary.DraftsWritten = append(summary.DraftsWritten, s.Slug)
	}
	return summary, nil
}

// FormatDraftSummary renders a human-readable report of a
// WriteSkillDrafts run.
func FormatDraftSummary(w io.Writer, s DraftSummary, dryRun bool) {
	prefix := ""
	if dryRun {
		prefix = "(dry-run) "
	}
	fmt.Fprintf(w, "%s%d draft(s) written (%d candidate(s) considered)\n",
		prefix, len(s.DraftsWritten), s.CandidatesConsidered)
	for _, slug := range s.DraftsWritten {
		fmt.Fprintf(w, "  - docs/skills-derived/%s.md\n", slug)
	}
	fmt.Fprintf(w, "ok sync-drafts: %s%d draft(s) written/%d candidate(s)\n",
		prefix, len(s.DraftsWritten), s.CandidatesConsidered)
}
