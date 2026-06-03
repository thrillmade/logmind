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

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
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

		// Track set of applied SHAs across the whole run for the
		// reviews-applied counter.
		for _, s := range applied {
			appliedReviewSet[s] = struct{}{}
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
				continue
			}
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

// atomicWriteFile writes to a temp sibling + renames. Avoids leaving a
// half-written PROVENANCE.md if the process dies mid-write — same
// discipline as Python v0.6.16's atomic_io.write_text.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

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
