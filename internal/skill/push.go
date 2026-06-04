package skill

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/thrillmade/logmind/internal/clierr"
)

// Push errors. Wrapped via fmt.Errorf("... : %w", ...) downstream so
// the CLI layer can distinguish "user supplied bad input" from
// "external command failed mid-flight" with errors.Is.
var (
	// ErrSkillNotFound is returned when no `.claude/skills/<name>/SKILL.md`
	// exists locally. The Go CLI surface treats this as ErrSilent because
	// we've already printed the user-facing message.
	ErrSkillNotFound = errors.New("skill not found")

	// ErrInvalidCatalogTarget is returned when the resolved catalog is
	// not a recognisable `<owner>/<repo>` slug.
	ErrInvalidCatalogTarget = errors.New("invalid catalog target")

	// ErrGhNotFound is returned when the `gh` binary is missing from
	// PATH. Push needs it to authenticate against the catalog repo and
	// open the PR.
	ErrGhNotFound = errors.New("gh CLI not found on PATH")

	// ErrGhNotAuthed is returned when `gh auth status` exits non-zero.
	ErrGhNotAuthed = errors.New("gh CLI not authenticated (run `gh auth login`)")

	// ErrInvalidSkillName is returned when the caller passes a skill
	// name that contains path-traversal characters (`/`, `\`, or `..`).
	// Without this guard, a malicious or accidental `../foo` would
	// escape both the local `.claude/skills/<name>/` tree and the
	// catalog clone's `skills/<name>/` tree via filepath.Join. See
	// review #136 / Bug 4. Skill names also drive branch names,
	// PR titles, and provenance YAML fields, so we tighten the rule
	// here at the entry point rather than scattering downstream
	// validation across the helpers.
	ErrInvalidSkillName = errors.New("invalid skill name")

	// ErrPrivateSkill is returned when the local skill is marked private
	// — either by frontmatter (`private: true` / `do-not-promote: true`)
	// or by directory convention (`.claude/skills-private/<name>/`).
	//
	// First slice of the §8.2 privacy gate (master plan: belt-and-braces
	// layer 1 + layer 2). Wraps clierr.ErrSilent via `fmt.Errorf("%w: %w",
	// ErrPrivateSkill, clierr.ErrSilent)` at the call site so the CLI
	// surface exits non-zero without re-printing on stderr — the
	// human-facing rejection message is written to stdout before the
	// error propagates. No `--force` flag intentionally: these are guard
	// rails, not toggles. Users who genuinely need to push a
	// skills-private skill must move it to `.claude/skills/<name>/` and
	// clear the frontmatter markers (and accept the explicit promotion).
	ErrPrivateSkill = errors.New("skill marked private (§8.2 privacy gate)")
)

// silentPrivate wraps a privacy-gate rejection so that
// `errors.Is(err, ErrPrivateSkill)` and `errors.Is(err, clierr.ErrSilent)`
// both succeed without needing custom error types. Returned only via
// the helper below so the wrap chain is consistent across both layers.
//
// We use a dedicated helper (rather than `fmt.Errorf("%w: %w", ...)`
// with two %w verbs) because Go's errors.Is walks ONE Unwrap chain at
// a time — chained %w doesn't broadcast `Is` to both wrapped errors
// in all stdlib versions. The errors.Join return value, however, is a
// multi-error that errors.Is unwraps correctly across each element.
func newPrivateSkillError(format string, args ...any) error {
	return errors.Join(
		fmt.Errorf(format, args...),
		ErrPrivateSkill,
		clierr.ErrSilent,
	)
}

// skillNameRE constrains skill slugs to a kebab-safe shape: lowercase
// alnum start, then alnum + `.`, `_`, `-`. Matches SPEC §1.10.1
// frontmatter rules. Crucially, it disallows `/`, `\`, `..`, and any
// whitespace — the three vectors that could rewrite `filepath.Join`
// into an escape. We anchor on the full string (Go's regexp.MatchString
// is implicitly unanchored, so we add `^…$`).
var skillNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// catalogTargetRE constrains catalog slugs to the GitHub shape
// `<owner>/<repo>`. Owner + repo must each match GitHub's allowed
// chars (alnum + `_` + `-` + `.`); the leading char must be alnum.
var catalogTargetRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9._-]+$`)

// PushOptions bundles the inputs to Push so future fields (e.g.,
// reviewer mentions, draft-PR flag) extend cleanly.
type PushOptions struct {
	// SkillName: the local skill slug (e.g., "critical-issues-only").
	// Resolves to .claude/skills/<SkillName>/SKILL.md.
	SkillName string

	// CatalogTarget: `<owner>/<repo>` for the destination catalog repo.
	// Per plan §"Skill suggestion cycle §4" default is
	// "thrillmade/agent-skills"; the CLI flag --catalog and the config
	// key catalog_target both flow into this field.
	CatalogTarget string

	// DryRun: when true, validate + report what would happen but skip
	// all network-touching steps (clone, push, gh pr create).
	DryRun bool

	// Now: timestamp injected by tests; if zero, time.Now() is used.
	// Captured in the PROVENANCE metadata file so the catalog repo
	// audit trail records when the push was generated.
	Now time.Time

	// SourceRepoRoot: the consumer repo's local path. Resolved by the
	// CLI layer (typically os.Getwd()).
	SourceRepoRoot string

	// Stdout receives progress lines + the final "ok skill: push ..."
	// summary. Tests inject a buffer; production wires cmd.OutOrStdout().
	Stdout io.Writer
}

// PushResult captures what Push did so the CLI layer (and tests) can
// assert against the outcome without re-parsing stdout.
type PushResult struct {
	// SkillName echoed for callers that don't want to thread Options.
	SkillName string
	// CatalogTarget that was actually used (post-resolution).
	CatalogTarget string
	// CopiedFiles: relative paths inside the skill dir that were
	// staged for the catalog PR. Includes SKILL.md + companion files
	// (PROVENANCE.md, references/*, scripts/*, etc.).
	CopiedFiles []string
	// Branch: the catalog-side branch name. Format:
	//   skill/<name>-from-<source-repo>-<short-sha>
	Branch string
	// PRURL: the URL of the opened PR. Empty on dry-run.
	PRURL string
	// SourceRepo: the source repo slug (e.g., "thrillmade/logmind").
	// Captured for the provenance metadata.
	SourceRepo string
	// SourceCommit: the source HEAD commit SHA (long form).
	SourceCommit string
	// SourceAuthor: `git log -1 --format='%an <%ae>'` at HEAD.
	SourceAuthor string
}

// gitRunner abstracts subprocess invocation so tests can stub
// clone/push without an actual remote. Production callers pass nil
// and Push wires the default exec.Command-backed runner.
type gitRunner interface {
	Run(dir string, args ...string) (stdout string, stderr string, err error)
}

// ghRunner is the same shape for `gh` calls (auth status, pr create).
// Kept separate from gitRunner so tests can stub each independently.
type ghRunner interface {
	Run(dir string, args ...string) (stdout string, stderr string, err error)
}

// realGit is the production gitRunner. Lives at the bottom of this file.
type realGit struct{}

func (realGit) Run(dir string, args ...string) (string, string, error) {
	return runCmd(dir, "git", args...)
}

type realGh struct{}

func (realGh) Run(dir string, args ...string) (string, string, error) {
	return runCmd(dir, "gh", args...)
}

func runCmd(dir, name string, args ...string) (string, string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var so, se bytes.Buffer
	cmd.Stdout = &so
	cmd.Stderr = &se
	err := cmd.Run()
	return so.String(), se.String(), err
}

// Push publishes a local skill to a catalog repo via a GH PR.
//
// Direction: LOCAL → CATALOG. Skills are AUTHORED in the consumer repo
// (`.claude/skills/<name>/SKILL.md`); this command promotes a polished
// version to a catalog repo for community reuse. The catalog repo is
// downstream of every consumer — there is no inverse "pull from
// catalog" command, because that would invert the End State #5
// promise that repos always own their skills.
//
// Steps:
//
//  1. Resolve + validate the local skill path.
//  2. Resolve the catalog target (caller passes already-resolved value).
//  3. Resolve gh auth (must be logged in with write access to catalog).
//  4. Collect source-repo provenance (HEAD sha, source-repo slug, author).
//  5. Clone the catalog repo to a sanitised cache dir.
//  6. Create branch skill/<name>-from-<source-repo>-<short-sha>.
//  7. Copy the local skill files into catalog/skills/<name>/.
//  8. Generate the provenance metadata file (PROVENANCE-push.yml).
//  9. Commit + push the branch.
//  10. `gh pr create` against the catalog default branch.
//
// On --dry-run: steps 1-4 + a preview of 5-10. No clone, no push, no PR.
func Push(opts PushOptions) (PushResult, error) {
	return pushWith(opts, realGit{}, realGh{})
}

// pushWith is the testable core. Same surface as Push but takes
// injectable runners for git + gh.
func pushWith(opts PushOptions, git gitRunner, gh ghRunner) (PushResult, error) {
	res := PushResult{
		SkillName:     opts.SkillName,
		CatalogTarget: opts.CatalogTarget,
	}

	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}

	// 0. Validate skill name FIRST — before any filepath.Join uses it.
	// Per review #136 / Bug 4: `args[0]` flows in unsanitised, so a
	// caller passing `../../foo` would have SkillDir() escape the
	// `.claude/skills/` tree on read AND escape the catalog clone's
	// `skills/` tree on write. We reject the name at the door so the
	// downstream helpers can keep their join calls simple. Pattern is
	// the SPEC §1.10.1 frontmatter slug, plus an explicit empty-string
	// rejection (skillNameRE matches a leading [a-z0-9] so the empty
	// string already fails, but the explicit check makes intent obvious).
	if opts.SkillName == "" || !skillNameRE.MatchString(opts.SkillName) {
		fmt.Fprintf(opts.Stdout,
			"Error: skill name '%s' is not a valid slug (allowed: lowercase alnum, '.', '_', '-')\n",
			opts.SkillName)
		return res, fmt.Errorf("%w: %q", ErrInvalidSkillName, opts.SkillName)
	}

	// 0.5. Privacy gate — LAYER 2 (directory convention). §8.2 first
	// slice, belt-and-braces layer 2. Runs BEFORE any SKILL.md read so
	// that a skills-private/<name>/ skill is rejected without touching
	// its bytes — the directory placement itself is the signal. This
	// also means the override case (`private: false` frontmatter inside
	// skills-private/) is rejected at this gate WITHOUT us ever opening
	// the file: directory convention wins over explicit-false. Layer 2
	// fires regardless of whether `.claude/skills/<name>/` also exists;
	// if both trees contain the same skill, the user's signal is "I
	// have a private copy somewhere I don't want pushed" — we surface
	// the conflict rather than silently preferring the public copy.
	privateDir := filepath.Join(SkillsPrivateDir(opts.SourceRepoRoot), opts.SkillName)
	privateMD := filepath.Join(privateDir, "SKILL.md")
	if _, statErr := os.Stat(privateMD); statErr == nil {
		msg := fmt.Sprintf(
			"skill %s lives under .claude/skills-private/ — treated as private by convention; "+
				"directory placement wins (no override available at this layer). "+
				"Move to .claude/skills/%s/ if you intend to push it.",
			opts.SkillName, opts.SkillName)
		fmt.Fprintf(opts.Stdout, "Error: %s\n", msg)
		return res, newPrivateSkillError("%s", msg)
	}

	// 1. Validate skill exists + has valid frontmatter.
	skillDir := SkillDir(opts.SourceRepoRoot, opts.SkillName)
	mdPath := MDPath(opts.SourceRepoRoot, opts.SkillName)
	body, err := os.ReadFile(mdPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(opts.Stdout,
				"Error: skill '%s' not found at %s\n",
				opts.SkillName, mdPath)
			return res, fmt.Errorf("%w: %s", ErrSkillNotFound, mdPath)
		}
		return res, err
	}
	if check := CheckFrontmatter(string(body)); !check.OK {
		fmt.Fprintf(opts.Stdout, "Error: %s\n", check.Message)
		return res, fmt.Errorf("invalid frontmatter: %s", check.Message)
	}

	// 1.5. Privacy gate — LAYER 1 (frontmatter markers). §8.2 first
	// slice, belt-and-braces layer 1. Runs AFTER CheckFrontmatter so
	// the structural validation is well-formed before we look for the
	// privacy fields. Two equivalent markers — `private: true` and
	// `do-not-promote: true` — are both honoured because skill authors
	// adopt different conventions across the catalog ecosystem and
	// we'd rather catch both than litigate naming. No `--force` flag:
	// these markers are guard rails; users who genuinely want to push
	// remove the marker (and accept the explicit promotion). Layers 3
	// (content scanner) + 4 (repo-visibility check) are queued for
	// wave-2 (see master plan §8.2).
	if field, hit := scanPrivateFrontmatterField(string(body)); hit {
		catalog := opts.CatalogTarget
		if catalog == "" {
			catalog = "(catalog)"
		}
		msg := fmt.Sprintf(
			"skill %s is marked private (frontmatter %s: true); not pushing to %s. "+
				"Remove the marker OR move the skill to a different catalog target via --catalog or .logmind/config.yml.",
			opts.SkillName, field, catalog)
		fmt.Fprintf(opts.Stdout, "Error: %s\n", msg)
		return res, newPrivateSkillError("%s", msg)
	}

	// Companion files: anything under .claude/skills/<name>/ EXCEPT
	// PROVENANCE.md (which is the consumer-repo-internal counter file
	// — the catalog gets its own PROVENANCE-push.yml summarising the
	// push event).
	companions, err := listCompanionFiles(skillDir)
	if err != nil {
		return res, err
	}
	res.CopiedFiles = append([]string{"SKILL.md"}, companions...)

	// 2. Validate catalog target shape.
	if !catalogTargetRE.MatchString(opts.CatalogTarget) {
		fmt.Fprintf(opts.Stdout,
			"Error: catalog target '%s' is not a valid <owner>/<repo> slug\n",
			opts.CatalogTarget)
		return res, fmt.Errorf("%w: %s", ErrInvalidCatalogTarget, opts.CatalogTarget)
	}

	// 4. Collect source-repo provenance (runs even on dry-run — useful
	// preview info). Resolved BEFORE the gh-auth check so the user
	// sees the source context even when we abort on auth.
	res.SourceRepo = resolveSourceRepoSlug(git, opts.SourceRepoRoot)
	res.SourceCommit = trimOrEmpty(runOK(git, opts.SourceRepoRoot, "rev-parse", "HEAD"))
	res.SourceAuthor = trimOrEmpty(runOK(git, opts.SourceRepoRoot, "log", "-1", "--format=%an <%ae>"))
	shortSHA := safeShortSHA(res.SourceCommit)
	branchSuffix := safeBranchSegment(res.SourceRepo)
	if branchSuffix == "" {
		branchSuffix = "local"
	}
	res.Branch = fmt.Sprintf("skill/%s-from-%s-%s",
		safeBranchSegment(opts.SkillName), branchSuffix, shortSHA)

	// Print the pre-flight plan. Same shape on real + dry-run so the
	// user can read what's coming.
	fmt.Fprintf(opts.Stdout, "→ Pushing skill '%s' to %s\n",
		opts.SkillName, opts.CatalogTarget)
	fmt.Fprintf(opts.Stdout, "  source: %s @ %s\n",
		nonEmpty(res.SourceRepo, "(local, no remote)"), shortSHA)
	fmt.Fprintf(opts.Stdout, "  branch: %s\n", res.Branch)
	fmt.Fprintf(opts.Stdout, "  files: %s\n", strings.Join(res.CopiedFiles, ", "))

	if opts.DryRun {
		fmt.Fprintln(opts.Stdout, "→ Dry-run: skipping clone, push, and PR creation.")
		fmt.Fprintf(opts.Stdout, "ok skill: push %s dry-run\n", opts.SkillName)
		return res, nil
	}

	// 3. Verify gh is authed. We do this AFTER the file-validation /
	// catalog-validation steps so users hit input bugs first; the auth
	// check stays second-line.
	if err := ensureGhAuthed(gh); err != nil {
		fmt.Fprintf(opts.Stdout, "Error: %s\n", err.Error())
		return res, err
	}

	// 5. Clone the catalog repo to a sanitised cache dir.
	cacheDir, err := catalogCacheDir(opts.CatalogTarget)
	if err != nil {
		return res, err
	}
	// Fresh clone every time — keeps catalog-repo state aligned with the
	// remote's default branch so we never push a branch built on a stale
	// base. Cost is small (catalog repos are pure-markdown, sub-MB).
	if err := os.RemoveAll(cacheDir); err != nil {
		return res, fmt.Errorf("clean cache dir %s: %w", cacheDir, err)
	}
	if err := os.MkdirAll(filepath.Dir(cacheDir), 0o755); err != nil {
		return res, err
	}
	cloneURL := fmt.Sprintf("https://github.com/%s.git", opts.CatalogTarget)
	if _, stderr, err := git.Run(filepath.Dir(cacheDir),
		"clone", "--depth=1", cloneURL, filepath.Base(cacheDir)); err != nil {
		return res, fmt.Errorf("git clone %s: %w (%s)", cloneURL, err, strings.TrimSpace(stderr))
	}

	// 6. Create the working branch.
	if _, stderr, err := git.Run(cacheDir, "checkout", "-b", res.Branch); err != nil {
		return res, fmt.Errorf("git checkout -b %s: %w (%s)", res.Branch, err, strings.TrimSpace(stderr))
	}

	// 7. Copy local skill files → catalog/skills/<name>/.
	targetDir := filepath.Join(cacheDir, "skills", opts.SkillName)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return res, err
	}
	if err := copyTree(skillDir, targetDir, res.CopiedFiles); err != nil {
		return res, fmt.Errorf("copy skill files: %w", err)
	}

	// 8. Provenance metadata. Written next to SKILL.md inside the
	// catalog so reviewers see source context inline. Filename:
	// PROVENANCE-push.yml (distinct from the in-repo PROVENANCE.md so
	// the two can co-exist if someone copies the catalog skill back
	// into a consumer repo).
	provPath := filepath.Join(targetDir, "PROVENANCE-push.yml")
	provBody := renderProvenance(opts.SkillName, res, opts.Now, summarizeDescription(string(body)))
	if err := os.WriteFile(provPath, []byte(provBody), 0o644); err != nil {
		return res, err
	}

	// 9. Stage, commit, push.
	if _, stderr, err := git.Run(cacheDir, "add", filepath.Join("skills", opts.SkillName)); err != nil {
		return res, fmt.Errorf("git add: %w (%s)", err, strings.TrimSpace(stderr))
	}
	commitMsg := fmt.Sprintf("skill(%s): propose from %s @ %s",
		opts.SkillName, nonEmpty(res.SourceRepo, "local"), shortSHA)
	if _, stderr, err := git.Run(cacheDir, "commit", "-m", commitMsg); err != nil {
		return res, fmt.Errorf("git commit: %w (%s)", err, strings.TrimSpace(stderr))
	}
	if _, stderr, err := git.Run(cacheDir, "push", "-u", "origin", res.Branch); err != nil {
		return res, fmt.Errorf("git push: %w (%s)", err, strings.TrimSpace(stderr))
	}

	// 10. Open the PR. Title + body shaped from the source context so
	// catalog reviewers see what they're accepting at a glance.
	prTitle := fmt.Sprintf("skill(%s): propose from %s",
		opts.SkillName, nonEmpty(res.SourceRepo, "local"))
	prBody := renderPRBody(opts.SkillName, res, opts.Now, summarizeDescription(string(body)))
	prURL, stderr, err := gh.Run(cacheDir,
		"pr", "create",
		"--repo", opts.CatalogTarget,
		"--title", prTitle,
		"--body", prBody,
		"--head", res.Branch,
	)
	if err != nil {
		return res, fmt.Errorf("gh pr create: %w (%s)", err, strings.TrimSpace(stderr))
	}
	res.PRURL = strings.TrimSpace(prURL)

	fmt.Fprintf(opts.Stdout, "✓ PR opened: %s\n", res.PRURL)
	fmt.Fprintf(opts.Stdout, "ok skill: push %s -> %s\n", opts.SkillName, res.PRURL)
	return res, nil
}

// listCompanionFiles enumerates the files inside the skill directory
// (recursively) that should travel alongside SKILL.md. Skips PROVENANCE.md
// (the consumer-repo counter file, which has its own catalog counterpart
// PROVENANCE-push.yml), hidden files, and SKILL.md itself (already
// implied). Sorted for deterministic output.
func listCompanionFiles(skillDir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(skillDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(skillDir, path)
		if relErr != nil {
			return relErr
		}
		base := filepath.Base(rel)
		if base == "SKILL.md" {
			return nil
		}
		// Skip the in-repo counter file. The catalog gets a different,
		// push-specific provenance file.
		if base == "PROVENANCE.md" {
			return nil
		}
		// Skip hidden / dotfiles — they shouldn't ship to the catalog.
		// (E.g., .DS_Store on macOS, editor swap files.)
		if strings.HasPrefix(base, ".") {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// copyTree replicates the listed paths from src to dst.
// paths is the same list returned by listCompanionFiles + the implicit
// "SKILL.md" — every entry is a slash-separated path relative to src.
//
// Preserves the source file's permission bits (in particular, the
// executable bit) so scripts shipped under `scripts/*.sh` stay runnable
// in the catalog repo. Without this, a `chmod +x` source file would
// land in the catalog as 0o644 and consumers cloning the catalog would
// have to re-`chmod +x` after install. (Review #136 / Bug 3.)
func copyTree(src, dst string, paths []string) error {
	for _, p := range paths {
		sp := filepath.Join(src, filepath.FromSlash(p))
		dp := filepath.Join(dst, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dp), 0o755); err != nil {
			return err
		}
		// Read the source mode BEFORE the body so we don't truncate the
		// dest if Lstat fails (e.g., race vs concurrent unlink).
		// Lstat (not Stat) so symlinks in the skill dir report their own
		// mode rather than the target's — matches `cp -p` semantics.
		info, err := os.Lstat(sp)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(sp)
		if err != nil {
			return err
		}
		// info.Mode().Perm() is the low 9 perm bits; the executable bit
		// for any of user/group/other survives the copy. Other mode
		// bits (setuid, sticky, etc.) are intentionally NOT propagated
		// — they don't have a portable meaning in a markdown-skill
		// catalog and dropping them keeps the copy minimal-surprise.
		perm := info.Mode().Perm()
		if err := os.WriteFile(dp, data, perm); err != nil {
			return err
		}
		// os.WriteFile only honours `perm` when it creates a new file
		// via OpenFile(O_CREATE). If `dp` already existed (e.g., the
		// dest tree was pre-staged), the create flag is a no-op for
		// mode. An explicit Chmod after the write makes the mode
		// outcome consistent regardless of whether dp was new or
		// pre-existing — the executable bit lands either way.
		if err := os.Chmod(dp, perm); err != nil {
			return err
		}
	}
	return nil
}

// catalogCacheDir returns the per-catalog clone directory under the
// user cache dir. Sanitised: any '/' or whitespace in the slug becomes
// '_' so we never accidentally walk above the cache root.
func catalogCacheDir(target string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	safe := strings.NewReplacer("/", "_", " ", "_", "..", "_").Replace(target)
	return filepath.Join(base, "logmind", "skill-push", safe), nil
}

// ensureGhAuthed shells out to `gh auth status`. We don't parse the
// output; the exit code is the contract. Missing binary returns
// ErrGhNotFound so the CLI surface prints the actionable "install gh"
// hint.
func ensureGhAuthed(gh ghRunner) error {
	_, stderr, err := gh.Run("", "auth", "status")
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return ErrGhNotFound
	}
	// Heuristic: most "not authenticated" gh outputs include the word
	// "logged in" or "auth login" — surface as the actionable
	// ErrGhNotAuthed. Fall through to a generic wrap otherwise.
	lower := strings.ToLower(stderr)
	if strings.Contains(lower, "auth login") || strings.Contains(lower, "not logged in") {
		return ErrGhNotAuthed
	}
	return fmt.Errorf("gh auth status: %w (%s)", err, strings.TrimSpace(stderr))
}

// resolveSourceRepoSlug returns the `<owner>/<repo>` shape of the
// source repo's origin remote, or "" if no remote is configured / the
// URL isn't a recognisable GitHub URL. Empty is fine — the provenance
// just records "(local, no remote)" in that case.
func resolveSourceRepoSlug(git gitRunner, repoRoot string) string {
	out, _, err := git.Run(repoRoot, "config", "--get", "remote.origin.url")
	if err != nil {
		return ""
	}
	return parseGitHubSlug(strings.TrimSpace(out))
}

// parseGitHubSlug extracts <owner>/<repo> from a remote URL. Handles:
//   - https://github.com/owner/repo.git
//   - https://github.com/owner/repo
//   - git@github.com:owner/repo.git
//   - ssh://git@github.com/owner/repo.git
//
// Returns "" on anything else so the caller falls back to the
// "(local, no remote)" placeholder.
func parseGitHubSlug(url string) string {
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimSpace(url)
	if url == "" {
		return ""
	}
	// SSH form: git@github.com:owner/repo
	if strings.HasPrefix(url, "git@") {
		if idx := strings.Index(url, ":"); idx >= 0 {
			return url[idx+1:]
		}
		return ""
	}
	// https or ssh URL form
	for _, prefix := range []string{
		"https://github.com/",
		"http://github.com/",
		"ssh://git@github.com/",
		"git://github.com/",
	} {
		if strings.HasPrefix(url, prefix) {
			return strings.TrimPrefix(url, prefix)
		}
	}
	return ""
}

// safeShortSHA returns the first 7 chars of sha, or "unknown" when
// sha is empty / shorter than 7 chars. Keeps generated branch names
// usable even when the source repo has no commits yet.
func safeShortSHA(sha string) string {
	if len(sha) < 7 {
		return "unknown"
	}
	return sha[:7]
}

// safeBranchSegment maps a slash-bearing slug like "thrillmade/logmind"
// to a single git-branch-safe segment ("thrillmade-logmind"). Strips
// any character outside [A-Za-z0-9._-].
func safeBranchSegment(in string) string {
	var b strings.Builder
	for _, r := range in {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// summarizeDescription pulls the `description:` line out of SKILL.md
// frontmatter. Used in the catalog PR title + body so reviewers see
// the skill's discovery surface without opening SKILL.md.
//
// Returns "" when no description line is found — the PR body falls
// back to "(no description)" in that case.
func summarizeDescription(body string) string {
	if !strings.HasPrefix(body, "---") {
		return ""
	}
	end := strings.Index(body[4:], "\n---")
	if end == -1 {
		return ""
	}
	fm := body[4 : 4+end]
	for _, line := range strings.Split(fm, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "description:") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
		}
	}
	return ""
}

// privateFrontmatterFields holds the field NAMES (not values) we treat
// as the §8.2 layer-1 privacy markers. Both spellings are honoured —
// the catalog ecosystem hasn't settled on a single name and we'd
// rather catch both than wait for consensus. Add new aliases here as
// the convention evolves; the scanner is generic on the field list.
var privateFrontmatterFields = []string{"private", "do-not-promote"}

// privateFieldRE is constructed at init time from privateFrontmatterFields.
// Anchored multi-line so an "indented" private: true (one level of
// indent, as some authors prefer under a `metadata:` block) still
// triggers — same shape as frontmatterNameRE in validate.go.
//
// Why we accept only `true`/`yes`/`on` (and case-insensitive) for the
// value: YAML allows several boolean spellings, and the SPEC §1.10.1
// frontmatter uses plain `true`. We accept the YAML 1.1 trio so
// authors who pasted `private: yes` (common in older configs) still
// get the gate. We deliberately DON'T accept `1` — a stray
// `private: 1.5.0` (version string typed into the wrong field) would
// false-positive. Strings need to be unambiguous booleans.
var privateFieldRE = func() *regexp.Regexp {
	// Build pattern: `(?im)^\s*(private|do-not-promote)\s*:\s*(true|yes|on)\s*$`
	// (?i) case-insensitive on the keyword + value;
	// (?m) `^`/`$` anchor per-line.
	keys := strings.Join(privateFrontmatterFields, "|")
	return regexp.MustCompile(`(?im)^\s*(` + keys + `)\s*:\s*(true|yes|on)\s*$`)
}()

// scanPrivateFrontmatterField inspects SKILL.md frontmatter for any
// §8.2 layer-1 privacy marker. Returns (matched-field-name, true) on a
// hit and ("", false) otherwise. The matched-field-name is what we
// surface in the user-facing error so the rejection points at the
// exact line they need to edit.
//
// Parser shape mirrors summarizeDescription: pull the frontmatter
// slice, then run a per-line scan. We don't reach for a full YAML
// parser here — the SKILL.md frontmatter is well-behaved and CheckFrontmatter
// already validated structure; a regex match keeps the dependency
// footprint zero. If the spec ever grows nested privacy fields (e.g.
// `policy:\n  visibility: private`) we'll swap this for go-yaml.
//
// Returns ("", false) when:
//   - body has no frontmatter,
//   - the frontmatter is unterminated,
//   - no privacy marker line is present, or
//   - the value isn't an unambiguous boolean-true.
func scanPrivateFrontmatterField(body string) (string, bool) {
	if !strings.HasPrefix(body, "---") {
		return "", false
	}
	end := strings.Index(body[4:], "\n---")
	if end == -1 {
		return "", false
	}
	fm := body[4 : 4+end]
	m := privateFieldRE.FindStringSubmatch(fm)
	if m == nil {
		return "", false
	}
	// m[1] is the captured field name. Normalise to lowercase so the
	// error message uses canonical spelling regardless of source casing.
	return strings.ToLower(m[1]), true
}

// renderProvenance writes the YAML doc the catalog ships next to
// SKILL.md. Plain YAML so reviewers + tools can grep it. The file is
// independent of the in-repo PROVENANCE.md (which counts citations
// over time); this one records a single push event.
func renderProvenance(name string, res PushResult, now time.Time, description string) string {
	var b strings.Builder
	b.WriteString("# Provenance for catalog entry: ")
	b.WriteString(name)
	b.WriteString("\n# Generated by `logmind skill push`. Do not edit by hand.\n\n")
	b.WriteString("skill: ")
	b.WriteString(name)
	b.WriteString("\n")
	if description != "" {
		b.WriteString("description: ")
		b.WriteString(quoteYAMLString(description))
		b.WriteString("\n")
	}
	b.WriteString("source_repo: ")
	b.WriteString(nonEmpty(res.SourceRepo, "(local)"))
	b.WriteString("\n")
	b.WriteString("source_commit: ")
	b.WriteString(nonEmpty(res.SourceCommit, "(unknown)"))
	b.WriteString("\n")
	if res.SourceAuthor != "" {
		b.WriteString("source_author: ")
		b.WriteString(quoteYAMLString(res.SourceAuthor))
		b.WriteString("\n")
	}
	b.WriteString("pushed_at: ")
	b.WriteString(now.UTC().Format(time.RFC3339))
	b.WriteString("\n")
	b.WriteString("pushed_via: logmind skill push\n")
	return b.String()
}

// renderPRBody is the body text passed to `gh pr create`. Catalog repo
// doesn't have a PULL_REQUEST_TEMPLATE.md (verified 2026-06-03), so we
// ship the full context here.
func renderPRBody(name string, res PushResult, now time.Time, description string) string {
	var b strings.Builder
	b.WriteString("## Skill proposal: `")
	b.WriteString(name)
	b.WriteString("`\n\n")
	if description != "" {
		b.WriteString("> ")
		b.WriteString(description)
		b.WriteString("\n\n")
	}
	b.WriteString("### Provenance\n\n")
	b.WriteString("- **Source repo:** ")
	if res.SourceRepo != "" {
		b.WriteString("[`")
		b.WriteString(res.SourceRepo)
		b.WriteString("`](https://github.com/")
		b.WriteString(res.SourceRepo)
		b.WriteString(")")
	} else {
		b.WriteString("(local — no remote configured)")
	}
	b.WriteString("\n")
	b.WriteString("- **Source commit:** ")
	if res.SourceRepo != "" && res.SourceCommit != "" {
		b.WriteString("[`")
		b.WriteString(safeShortSHA(res.SourceCommit))
		b.WriteString("`](https://github.com/")
		b.WriteString(res.SourceRepo)
		b.WriteString("/commit/")
		b.WriteString(res.SourceCommit)
		b.WriteString(")")
	} else {
		b.WriteString(nonEmpty(res.SourceCommit, "(unknown)"))
	}
	b.WriteString("\n")
	if res.SourceAuthor != "" {
		b.WriteString("- **Source author:** ")
		b.WriteString(res.SourceAuthor)
		b.WriteString("\n")
	}
	b.WriteString("- **Pushed at:** ")
	b.WriteString(now.UTC().Format(time.RFC3339))
	b.WriteString("\n\n")
	b.WriteString("### Notes for catalog reviewers\n\n")
	b.WriteString("Skills are AUTHORED in the consumer repo first — this PR ")
	b.WriteString("promotes a polished local skill to the shared catalog. ")
	b.WriteString("Per the SkDD model, the catalog is a downstream surface; ")
	b.WriteString("the source repo retains ownership of the skill.\n\n")
	b.WriteString("Acceptance checklist:\n\n")
	b.WriteString("- [ ] Skill has a clear single responsibility.\n")
	b.WriteString("- [ ] Frontmatter `description:` is a precise trigger ")
	b.WriteString("(this is the discovery surface).\n")
	b.WriteString("- [ ] No project-specific paths or tokens leaked into ")
	b.WriteString("the body.\n")
	b.WriteString("- [ ] Size is within the catalog's per-skill cap.\n\n")
	b.WriteString("Generated by `logmind skill push`.\n")
	return b.String()
}

// quoteYAMLString wraps s in double quotes and escapes embedded `"`
// and `\` so the resulting line parses as a YAML scalar regardless of
// what was in the source description.
func quoteYAMLString(s string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
	return `"` + escaped + `"`
}

// runOK runs args via the supplied git runner and returns stdout on
// success / "" on failure. Used for provenance lookups that should
// degrade gracefully (no remote, no commits, etc.).
func runOK(git gitRunner, dir string, args ...string) string {
	out, _, err := git.Run(dir, args...)
	if err != nil {
		return ""
	}
	return out
}

func trimOrEmpty(s string) string { return strings.TrimSpace(s) }

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
