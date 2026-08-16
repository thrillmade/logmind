// dependabot.go — `logmind init` integration for `.github/dependabot.yml`.
//
// Goal of v1.1.0 (2026-06-05): ship Dependabot config alongside the
// new `thrillmade/setup-logmind@vX.Y.Z` workflow pattern so consumer
// repos get an automatic action-pin bumper out of the box. The
// installed block subscribes to the `github-actions` ecosystem and
// groups `thrillmade/*` action bumps into one PR per release.
//
// Two paths:
//
//   - Fresh install (no .github/dependabot.yml on disk) — write the
//     bundled template verbatim. Path comes from
//     templates.DependabotTemplate().
//
//   - Merge into existing config — when the consumer already has a
//     dependabot.yml (any reason: another tool installed it, the team
//     hand-rolled one), append a `github-actions` updates entry with a
//     `thrillmade` group ONLY if no entry already covers the same
//     ecosystem+directory pair. Idempotent: re-running on an existing
//     entry is a no-op. Conservative: never modifies, deletes, or
//     reorders existing entries — only adds the missing block.
//
// Detection strategy: the dependabot.yml schema is YAML, but parsing
// it would require pulling in a YAML lib for write-back that preserves
// comments and formatting (yaml.v3 reorders keys and strips comments).
// Instead, we detect the relevant lines with anchored substring matches
// — same approach the gitattr / gitignore helpers use. Cheap, robust,
// and visible from a diff.
//
// CAVEAT: A consumer who already has `package-ecosystem: "github-actions"`
// at directory `/` but WITHOUT a `thrillmade` group keeps their existing
// entry untouched. They miss the grouped-PR benefit, but their existing
// dependabot bumps continue to work. We surface a one-line notice in
// that case so the user can opt into the group manually.
package inserter

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/thrillmade/logmind/internal/atomicio"
	"github.com/thrillmade/logmind/internal/templates"
)

// DependabotResult describes the outcome of EnsureDependabot. The
// caller uses this to print the right "Created" / "Merged" / "Already
// current" line and to decide which files to stage for the init
// commit.
type DependabotResult int

const (
	// DependabotUnchanged — the file already had a github-actions
	// entry for the root directory with a thrillmade group, OR the
	// file existed but already covered the ecosystem (we leave the
	// existing entry alone). No write happened.
	DependabotUnchanged DependabotResult = iota

	// DependabotCreated — the file didn't exist; we wrote the bundled
	// template verbatim.
	DependabotCreated

	// DependabotMerged — the file existed without a github-actions
	// entry; we appended the thrillmade block while preserving the
	// rest of the file byte-for-byte.
	DependabotMerged

	// DependabotExistingEcosystem — the file already had a
	// github-actions ecosystem entry but no thrillmade group. We
	// didn't modify the file — the caller should surface a hint to
	// the user so they can opt in manually if they want grouped PRs.
	DependabotExistingEcosystem
)

// githubActionsBlockRE matches a `package-ecosystem: "github-actions"`
// updates entry. The match is anchored to the YAML list marker `- ` so
// it survives a re-ordered file. We accept either quote style or no
// quotes around the value.
var githubActionsBlockRE = regexp.MustCompile(`(?m)^\s*-\s*package-ecosystem:\s*["']?github-actions["']?`)

// thrillmadeGroupRE matches the `thrillmade:` group key under a
// `groups:` block. Used to detect whether an existing github-actions
// entry already groups thrillmade/* — if so, no merge needed.
var thrillmadeGroupRE = regexp.MustCompile(`(?m)^\s*thrillmade:\s*$`)

// EnsureDependabot writes or merges .github/dependabot.yml at
// repoRoot. Returns the DependabotResult so the caller can print the
// right "Created" / "Merged" / "Already current" line.
//
// Idempotency contract:
//
//   - Calling twice in a row returns DependabotCreated → DependabotUnchanged.
//   - Calling on a hand-rolled file with a github-actions entry
//     returns DependabotExistingEcosystem the first time and every
//     subsequent time (we never auto-merge into a user-owned block).
//   - Calling on a file without a github-actions entry returns
//     DependabotMerged the first time, DependabotUnchanged after.
func EnsureDependabot(repoRoot string) (DependabotResult, error) {
	path := filepath.Join(repoRoot, ".github", "dependabot.yml")

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		// Fresh install — write the bundled template verbatim. mkdir
		// the parent so `.github` is created when the repo is brand
		// new (logmind init can run before .github exists).
		// os.ReadFile follows a symlink, so a DANGLING symlink at path
		// (pointing at a target that doesn't exist) also returns
		// fs.ErrNotExist here — the file looks "absent" when it is really a
		// link elsewhere. A bare os.WriteFile would then follow that same
		// link and create the write target wherever it points, possibly
		// outside the repo. atomicio.WriteFile refuses instead
		// (atomicio.RefuseSymlink, #300) and makes its own parent directory,
		// so the explicit MkdirAll this replaced is no longer needed.
		if wErr := atomicio.WriteFile(path, []byte(templates.DependabotTemplate()), 0o644); wErr != nil {
			return DependabotUnchanged, fmt.Errorf("write dependabot.yml: %w", wErr)
		}
		return DependabotCreated, nil
	}
	if err != nil {
		return DependabotUnchanged, err
	}

	existing := string(data)

	// Already has a github-actions ecosystem entry?
	if githubActionsBlockRE.MatchString(existing) {
		// Same entry has a thrillmade group → idempotent no-op, no
		// notice needed. The thrillmade group is the load-bearing
		// signal that logmind installed (or merged with) this file
		// previously.
		if thrillmadeGroupRE.MatchString(existing) {
			return DependabotUnchanged, nil
		}
		// User-owned github-actions block without our group. Don't
		// touch it — Dependabot ITSELF errors on a duplicate
		// (ecosystem, directory) pair, so appending a second entry
		// would break the user's existing config. Return a status the
		// caller surfaces as a hint.
		return DependabotExistingEcosystem, nil
	}

	// File exists but doesn't cover github-actions yet. Append the
	// minimal github-actions block — just the entry, no `version: 2`
	// or `updates:` re-declaration. We rely on those keys already
	// being present (any valid dependabot.yml has them).
	merged, err := appendGithubActionsEntry(existing)
	if err != nil {
		return DependabotUnchanged, err
	}
	if merged == existing {
		// Defensive — shouldn't happen given the github-actions check
		// above, but if the regex falls through for an exotic shape
		// we don't want to corrupt the file.
		return DependabotUnchanged, nil
	}
	// path was read successfully above (existing, string(data)), so this is
	// a whole-file overwrite of a file the user may have reached through a
	// symlink — atomicio.WriteFile refuses that (atomicio.RefuseSymlink,
	// #300) rather than silently writing the merged body through the link.
	if err := atomicio.WriteFile(path, []byte(merged), 0o644); err != nil {
		return DependabotUnchanged, fmt.Errorf("write dependabot.yml: %w", err)
	}
	return DependabotMerged, nil
}

// appendGithubActionsEntry adds the minimal github-actions entry to an
// existing dependabot.yml body. Preserves the rest of the file
// byte-for-byte. The entry uses two-space indent (Dependabot's
// canonical style + matches our template).
//
// We don't try to re-anchor to a specific position inside the
// `updates:` list — append-at-end is the safe choice. Dependabot
// processes list order top-to-bottom but the order doesn't change
// semantics for distinct (ecosystem, directory) pairs.
func appendGithubActionsEntry(existing string) (string, error) {
	// Trim trailing whitespace once so the appended block always sits
	// directly under the last entry. Idempotent on a file ending in
	// `\n` already.
	trimmed := strings.TrimRight(existing, "\n")

	// The entry body matches dependabot.yml.template verbatim except
	// it omits the `version: 2` + `updates:` lines (those are already
	// present in the existing file). Two-space indent.
	const entry = `
  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "daily"
    groups:
      thrillmade:
        patterns:
          - "thrillmade/*"
`

	// Sanity check the existing file has an `updates:` key — if not,
	// the file is malformed and we shouldn't silently append into it.
	// Return the input unchanged so the caller falls through to the
	// no-op path; Dependabot will reject the file on its own.
	if !strings.Contains(existing, "updates:") {
		return existing, nil
	}
	return trimmed + entry, nil
}
