package skill

import (
	"errors"
	"fmt"
	"strings"
)

// visibility.go — §8.2 privacy gate, layer 4: repo-visibility check.
//
// The cross-visibility leak shape: a developer working in a PRIVATE
// company repo authors a useful skill, runs `logmind skill push`, and
// the default catalog target is the PUBLIC `thrillmade/agent-skills`.
// Without this layer, layers 1+2+3 would only catch markers the
// developer remembered to set or content patterns we know to look for
// — anything the scanner doesn't recognise (e.g., a domain we have no
// rule for) ships to the public catalog. Layer 4 adds a categorical
// "source is private, target is public — explicitly opt in" gate.
//
// Mechanism: `gh api /repos/<owner>/<repo> --jq .visibility` returns
// "public" / "private" / "internal" (the GitHub-Enterprise tier). If
// source is non-public and target is public, we reject UNLESS the user
// has set `allow_promote_from_private: true` in `.logmind/config.yml`.
// The flag is opt-out (defaults off) so a fresh install rejects
// cross-visibility pushes by default.
//
// Even with the opt-out flag set, layers 1-3 still run — layer 4 is a
// gate against CATEGORY mismatch, not a substitute for the content
// scanner. A user who flips the flag is saying "yes, I knowingly want
// to push from private to public", not "skip every check".

var (
	// ErrCrossVisibilityPush fires when source is private (or
	// "internal", in the GitHub Enterprise sense) and target is public,
	// and `allow_promote_from_private` is not set. Wrapped via
	// newPrivateSkillError so it also satisfies errors.Is(err,
	// ErrPrivateSkill) and errors.Is(err, clierr.ErrSilent) — the
	// rejection lands in the same shape as the layer 1+2 markers.
	ErrCrossVisibilityPush = errors.New("cross-visibility push blocked (§8.2 privacy gate)")

	// ErrPrivacyScannerHit fires when ScanContent returns at least one
	// "block"-severity hit. Same wrap shape as ErrCrossVisibilityPush
	// so the cli layer's translation table stays small.
	ErrPrivacyScannerHit = errors.New("skill content matched a privacy-scanner block pattern (§8.2 privacy gate)")
)

// VisibilityCheckResult bundles the outcome of CheckRepoVisibility so
// callers can both inspect what we found AND get the rejection error
// in one call. Fields:
//
//   - SourceVisibility, TargetVisibility: the raw `gh api` responses,
//     either "public", "private", "internal", or "" when the lookup
//     failed (no remote, gh not authed for that repo, transient
//     network error). Empty source visibility is treated as "unknown
//     — not private" so a brand-new repo with no remote doesn't get
//     blocked from pushing to the public catalog.
//   - Blocked: true when the result is a hard reject. Always false
//     when AllowPromoteFromPrivate is set.
//   - Reason: human-readable rejection message for the printable
//     stderr line. Empty when Blocked == false.
type VisibilityCheckResult struct {
	SourceVisibility string
	TargetVisibility string
	Blocked          bool
	Reason           string
}

// VisibilityCheckOptions bundles the inputs to CheckRepoVisibility so
// the signature stays stable when the option set grows.
//
// Fields:
//
//   - SourceRepo: <owner>/<repo> slug. May be "" when the source repo
//     has no remote (already resolved by resolveSourceRepoSlug). In
//     that case visibility lookup is skipped and SourceVisibility is
//     left empty.
//   - CatalogTarget: <owner>/<repo> slug for the catalog. Must be
//     non-empty by the time this runs (caller already validated).
//   - AllowPromoteFromPrivate: the config flag. When true, the gate
//     records visibility but always returns Blocked: false.
type VisibilityCheckOptions struct {
	SourceRepo              string
	CatalogTarget           string
	AllowPromoteFromPrivate bool
}

// CheckRepoVisibility queries gh for source + target visibility and
// returns a VisibilityCheckResult. Never returns an error — the gh
// runner failures degrade to empty-string visibility (treated as
// "unknown / public-enough to allow the push") so a network glitch
// doesn't block legitimate work. The Reason field tells the caller
// what the gate decided.
//
// Why we don't return error on gh failures: layer 4 is a belt-and-braces
// guard. If gh is unavailable, layers 1-3 still ran and would have
// caught the most likely leak shapes. A noisy "couldn't check
// visibility" error here would train users to ignore the warning and
// would fail-closed for offline CI runs — neither outcome serves the
// privacy goal better than the degraded check.
func CheckRepoVisibility(opts VisibilityCheckOptions, gh ghRunner) VisibilityCheckResult {
	res := VisibilityCheckResult{}

	// No source repo slug → no remote → no visibility to check. A
	// brand-new repo (no origin) is treated as "unknown / not
	// detectably private" so layer 4 doesn't block first-time skill
	// authors. Layers 1-3 still ran.
	if opts.SourceRepo != "" {
		res.SourceVisibility = fetchVisibility(gh, opts.SourceRepo)
	}
	if opts.CatalogTarget != "" {
		res.TargetVisibility = fetchVisibility(gh, opts.CatalogTarget)
	}

	// Decision matrix. Block path: source is non-public AND target is
	// public AND the user hasn't set the opt-out flag.
	srcPrivate := isNonPublic(res.SourceVisibility)
	tgtPublic := res.TargetVisibility == VisibilityPublic
	if srcPrivate && tgtPublic && !opts.AllowPromoteFromPrivate {
		res.Blocked = true
		res.Reason = fmt.Sprintf(
			"source repo %s is %s; target catalog %s is public. "+
				"Set `allow_promote_from_private: true` in .logmind/config.yml "+
				"to acknowledge cross-visibility promotion (layers 1-3 still run).",
			opts.SourceRepo, res.SourceVisibility, opts.CatalogTarget,
		)
	}
	return res
}

// Visibility constants. The gh API returns these exact strings (plus
// "internal" for GHEC); we pin them here so callers don't compare
// against magic strings.
const (
	VisibilityPublic   = "public"
	VisibilityPrivate  = "private"
	VisibilityInternal = "internal" // GitHub Enterprise tier
)

// isNonPublic returns true when the visibility string represents a
// non-public repo. We treat both "private" and "internal" (GHEC's
// "private but visible to anyone in your org") as non-public — both
// shapes count as "outside the public web" for our purposes.
//
// Empty string is treated as PUBLIC (not blocking) — see the Reason
// comment above on why we fail-open on gh lookup failures.
func isNonPublic(visibility string) bool {
	switch strings.ToLower(strings.TrimSpace(visibility)) {
	case VisibilityPrivate, VisibilityInternal:
		return true
	default:
		return false
	}
}

// fetchVisibility shells out via `gh api /repos/<slug> --jq .visibility`.
// Returns the lowercase string on success, "" on any failure.
//
// Why --jq + .visibility rather than parsing JSON ourselves: gh's
// built-in jq reduces the call to a single line of output, sidestepping
// json encoding work in this binary and keeping the runner contract
// (stdout/stderr/err) simple. The dependency on `jq` syntax is fine —
// gh ships it bundled.
func fetchVisibility(gh ghRunner, slug string) string {
	out, _, err := gh.Run("",
		"api",
		"/repos/"+slug,
		"--jq", ".visibility",
	)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(out))
}
