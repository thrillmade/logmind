package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/config"
	"github.com/thrillmade/logmind/internal/repomap"
	"github.com/thrillmade/logmind/internal/tokens"
)

// newContextCmd wires `logmind context [--stats]`.
//
// Prints the agent cold-start payload — the two pre-baked derived docs
// (docs/file-structure.md = the "what", docs/timeline.md = the "why") — to
// stdout in one read, structured for prompt caching. This is the single
// command an agent runs at the start of a task to orient, instead of burning
// tokens reconstructing context from `git log` / `ls -R` / `grep`. It is the
// practical expression of the logmind thesis (and the token-killer): a repo
// self-describing on the edge of inference.
//
// TOKEN-SAVING DESIGN (grounded in the research in the plan):
//   - Byte-stable + deterministic → an ideal prompt-cache prefix; cached
//     re-reads cost ~0.1× (≈90% off).
//   - Stable content first (file-structure), volatile last (the newest-first
//     timeline) → the cache prefix stays byte-identical longest (a churning
//     top would bust the cache from the first breakpoint).
//   - Both docs in the Anthropic-prescribed
//     <document><source><document_content> envelope (unambiguous parsing;
//     denser than stacked markdown prose).
//   - The framing is ONE machine line, not human-facing boilerplate.
//
// Read-only. A missing derived doc is noted as a comment and omitted, never an
// error — `context` is a convenience, not a gate.
func newContextCmd() *cobra.Command {
	var stats bool
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Print the cache-optimal agent cold-start context (file-structure + timeline) in one read",
		Long: `Print the agent cold-start context in one read, structured for prompt caching.

Emits the two pre-baked derived docs an agent reads to orient:

    docs/file-structure.md   the WHAT — the repo tree    (stable → cache prefix)
    docs/timeline.md         the WHY  — decision timeline (newest-first)

wrapped in a <document><source><document_content> envelope. Run it at the start
of a task instead of piecing context together from git log / ls / grep; open a
linked source file for full reasoning on demand.

Caching (the ~10x lever): this output is byte-stable and re-read across turns,
so treat it as a cacheable PREFIX —
  - place it ABOVE your task/query (long stable context first improves quality
    up to ~30% and is what caches),
  - set a prompt-cache breakpoint at the end of this block (cache_control),
  - use the 1-hour TTL for multi-step / multi-agent sessions, and pre-warm it,
  - read it ONCE per task; byte-identical re-reads hit the cache at ~0.1x input.

Set 'context.repomap: true' in .logmind/config.yml to also fold in the Go
signature skeleton (see 'logmind repomap') as a third, stable document — the
repo's API surface, placed between the file map and the timeline. Default off
keeps the payload byte-identical.

Set 'context.spec_file: docs/spec.md' (repo-relative; see 'logmind init
--spec') to fold in the project's canonical, forward-looking spec as the
FIRST document — it's the most stable doc, so it makes the best cache
prefix. Missing, empty, or a path outside the repo root is silently omitted
(never an error). Default "" keeps the payload byte-identical.

--stats prints a deterministic token receipt (est. ~4 chars/token) instead of
the payload.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			if stats {
				return runContextStats(cwd, cmd.OutOrStdout())
			}
			return runContext(cwd, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&stats, "stats", false,
		"Print a deterministic token receipt (payload size + density vs. raw sources) instead of the payload.")
	return cmd
}

// contextDoc is one document in the cold-start payload.
type contextDoc struct {
	rel   string // repo-relative path to the derived doc (file-backed docs)
	typ   string // <document type=…> — machine label
	regen string // command to regenerate it when missing
	// gen, when non-nil, produces the document body IN-MEMORY instead of
	// reading `rel` off disk (e.g. the repomap skeleton). It returns the body
	// and a source label; an empty body omits the document entirely.
	gen func(cwd string, cfg config.Config) (body, source string)
	// enabled, when non-nil, gates whether this doc participates. Used for
	// opt-in additive docs so the default payload stays byte-stable. nil =
	// always included.
	enabled func(cfg config.Config) bool
}

// contextDocs is the ordered payload, most-stable first so the cacheable
// prefix stays byte-identical for as long as possible: the hand-authored spec
// (opt-in; most stable of all — it's never regenerated) → the file map
// (what) → the repomap API surface (also stable; opt-in) → the volatile
// newest-first timeline (why).
var contextDocs = []contextDoc{
	{typ: "spec", gen: specDoc, enabled: func(c config.Config) bool { return c.Context.SpecFile != "" }},
	{rel: "docs/file-structure.md", typ: "file-structure", regen: "logmind file-structure --write docs/file-structure.md"},
	{typ: "repomap", gen: repomapDoc, enabled: func(c config.Config) bool { return c.Context.Repomap }},
	{rel: "docs/timeline.md", typ: "decision-timeline", regen: "logmind timeline --write docs/timeline.md"},
}

// specDoc renders the canonical spec file (context.spec_file) for the
// context payload. config.ResolveSpecFile already enforces the path-safety
// rule (unset/absolute/out-of-root all collapse to "not resolved"); this
// function additionally treats a missing or all-whitespace file as nothing
// to contribute. Every one of those cases is a silent, error-free omission —
// mirroring repomapDoc's gen-returning-empty convention, NOT the file-backed
// doc's absent-element branch below (a spec is a nice-to-have fold-in, never
// a gate, and its absence carries no actionable "regenerate" command since
// it's hand-authored). The body is never truncated.
func specDoc(cwd string, cfg config.Config) (body, source string) {
	path, ok := config.ResolveSpecFile(cwd, cfg)
	if !ok {
		return "", ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", ""
	}
	return string(data), cfg.Context.SpecFile
}

// repomapDoc renders the Go signature skeleton for the context payload. An
// empty result (no Go symbols, or an extraction error) omits the document — the
// repomap is a convenience, never a gate.
func repomapDoc(cwd string, cfg config.Config) (body, source string) {
	text, files, err := repomap.Generate(cwd, cfg.FileStructure.IgnorePatterns)
	if err != nil || len(files) == 0 {
		return "", ""
	}
	return text, "logmind repomap"
}

const contextPreface = "Pre-baked repo cold-start context: the file map (what) + the decision " +
	"timeline (why). Read this to orient instead of running git log / ls / grep; " +
	"open a linked source file for detail. Byte-stable — cache it as a prefix ABOVE your task.\n"

// contextPayload builds the deterministic, cache-optimal payload: the machine
// preface + the two derived docs in an XML document envelope, stable-first.
// Deterministic by construction (fixed strings + on-disk doc bytes, stable
// order) — the property prompt caching depends on.
func contextPayload(cwd string) string {
	cfg, _ := config.Load(cwd)
	var b strings.Builder
	b.WriteString(contextPreface)
	b.WriteString("\n<repo_context>\n")
	for _, d := range contextDocs {
		if d.enabled != nil && !d.enabled(cfg) {
			continue // opt-in additive doc, disabled → omit (default byte-stable)
		}
		if d.gen != nil {
			body, source := d.gen(cwd, cfg)
			if body == "" {
				continue // nothing to contribute — omit cleanly
			}
			writeDoc(&b, d.typ, source, []byte(body))
			continue
		}
		data, err := os.ReadFile(filepath.Join(cwd, d.rel))
		if err != nil {
			// A missing doc becomes a self-closing element carrying the
			// regenerate command in an attribute — well-formed (an XML comment
			// would hit the "--" double-hyphen rule via `--write`) and still
			// tells the agent what's absent and how to restore it.
			fmt.Fprintf(&b, "<document type=%q source=%q status=\"absent\" regenerate=%q/>\n", d.typ, d.rel, d.regen)
			continue
		}
		writeDoc(&b, d.typ, d.rel, data)
	}
	b.WriteString("</repo_context>\n")
	return b.String()
}

// writeDoc emits one <document> envelope with a trailing-newline guarantee on
// the body. Shared by the file-backed and generated doc paths so both render
// byte-identically.
func writeDoc(b *strings.Builder, typ, source string, data []byte) {
	fmt.Fprintf(b, "<document type=%q>\n<source>%s</source>\n<document_content>\n", typ, source)
	b.Write(data)
	if n := len(data); n == 0 || data[n-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteString("</document_content>\n</document>\n")
}

func runContext(cwd string, stdout io.Writer) error {
	_, err := io.WriteString(stdout, contextPayload(cwd))
	return err
}

// runContextStats prints a deterministic token receipt: the payload size and
// how much denser the timeline is than the raw decision logs it distills.
func runContextStats(cwd string, stdout io.Writer) error {
	cfg, _ := config.Load(cwd)
	payload := contextPayload(cwd)
	fsTok := tokens.Estimate(ctxReadOrEmpty(filepath.Join(cwd, "docs", "file-structure.md")))
	tlTok := tokens.Estimate(ctxReadOrEmpty(filepath.Join(cwd, "docs", "timeline.md")))
	rawWhy := rawDecisionTokens(cwd)

	fmt.Fprint(stdout, "logmind context — token receipt (est. ~4 chars/token, deterministic)\n\n")

	// The spec term appears only when spec_file actually contributed to the
	// payload (configured, resolved in-root, present, non-whitespace) —
	// matching what contextPayload/specDoc emits. Unlike repomap/timeline, a
	// spec distills nothing (it's hand-authored, not derived from a larger
	// source) so there is deliberately no density claim for it below.
	specTok := 0
	if body, _ := specDoc(cwd, cfg); body != "" {
		specTok = tokens.Estimate(body)
	}

	// The repomap term appears only when it is enabled AND has symbols to
	// contribute (matching what contextPayload actually emits).
	mapTok, rawGo := 0, 0
	if cfg.Context.Repomap {
		if mapText, files, err := repomap.Generate(cwd, cfg.FileStructure.IgnorePatterns); err == nil && len(files) > 0 {
			mapTok = tokens.Estimate(mapText)
			for _, f := range files {
				rawGo += tokens.Estimate(ctxReadOrEmpty(filepath.Join(cwd, filepath.FromSlash(f.Path))))
			}
		}
	}

	var parts []string
	if specTok > 0 {
		parts = append(parts, fmt.Sprintf("spec %d", specTok))
	}
	parts = append(parts, fmt.Sprintf("file-structure %d", fsTok))
	if mapTok > 0 {
		parts = append(parts, fmt.Sprintf("repomap %d", mapTok))
	}
	parts = append(parts, fmt.Sprintf("timeline %d", tlTok))
	parts = append(parts, "framing")
	fmt.Fprintf(stdout, "  payload total:  %6d tok  (%s)\n", tokens.Estimate(payload), strings.Join(parts, " + "))

	// Only claim density when the skeleton is genuinely smaller than the source
	// it summarizes — on a trivial repo the `# Repomap` framing can exceed a
	// one-file source, and "0.6x denser" reads worse than saying nothing.
	if mapTok > 0 && rawGo > mapTok {
		fmt.Fprintf(stdout, "  the repomap distills %d tok of Go source -> %.1fx denser.\n",
			rawGo, float64(rawGo)/float64(mapTok))
	}
	if rawWhy > 0 && tlTok > 0 {
		fmt.Fprintf(stdout, "  the timeline distills %d tok of raw decision logs -> %.1fx denser.\n",
			rawWhy, float64(rawWhy)/float64(tlTok))
	}
	fmt.Fprint(stdout, "  reading this pre-baked payload replaces a git log / ls -R / grep cold-start.\n")
	fmt.Fprint(stdout, "  cache it as a stable prefix -> every re-read costs ~0.1x (about 90% off).\n")
	return nil
}

func ctxReadOrEmpty(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// rawDecisionTokens estimates the tokens of the raw decision sources the
// timeline distills (decisions.md + archive + every branch log) — the "why" an
// agent would otherwise reconstruct from git log.
func rawDecisionTokens(cwd string) int {
	total := 0
	for _, p := range []string{"decisions.md", "decisions-archive.md"} {
		total += tokens.Estimate(ctxReadOrEmpty(filepath.Join(cwd, "docs", p)))
	}
	branches, _ := filepath.Glob(filepath.Join(cwd, "docs", "decisions-branches", "*.md"))
	for _, p := range branches {
		total += tokens.Estimate(ctxReadOrEmpty(p))
	}
	return total
}
