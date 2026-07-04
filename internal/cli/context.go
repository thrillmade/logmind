package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

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
	rel   string // repo-relative path to the derived doc
	typ   string // <document type=…> — machine label
	regen string // command to regenerate it when missing
}

// contextDocs is the ordered payload: the stable "what" (file-structure)
// before the volatile "why" (newest-first timeline), so the cacheable prefix
// stays byte-identical for as long as possible.
var contextDocs = []contextDoc{
	{"docs/file-structure.md", "file-structure", "logmind file-structure --write docs/file-structure.md"},
	{"docs/timeline.md", "decision-timeline", "logmind timeline --write docs/timeline.md"},
}

const contextPreface = "Pre-baked repo cold-start context: the file map (what) + the decision " +
	"timeline (why). Read this to orient instead of running git log / ls / grep; " +
	"open a linked source file for detail. Byte-stable — cache it as a prefix ABOVE your task.\n"

// contextPayload builds the deterministic, cache-optimal payload: the machine
// preface + the two derived docs in an XML document envelope, stable-first.
// Deterministic by construction (fixed strings + on-disk doc bytes, stable
// order) — the property prompt caching depends on.
func contextPayload(cwd string) string {
	var b strings.Builder
	b.WriteString(contextPreface)
	b.WriteString("\n<repo_context>\n")
	for _, d := range contextDocs {
		data, err := os.ReadFile(filepath.Join(cwd, d.rel))
		if err != nil {
			// A missing doc becomes a self-closing element carrying the
			// regenerate command in an attribute — well-formed (an XML comment
			// would hit the "--" double-hyphen rule via `--write`) and still
			// tells the agent what's absent and how to restore it.
			fmt.Fprintf(&b, "<document type=%q source=%q status=\"absent\" regenerate=%q/>\n", d.typ, d.rel, d.regen)
			continue
		}
		fmt.Fprintf(&b, "<document type=%q>\n<source>%s</source>\n<document_content>\n", d.typ, d.rel)
		b.Write(data)
		if n := len(data); n == 0 || data[n-1] != '\n' {
			b.WriteByte('\n')
		}
		b.WriteString("</document_content>\n</document>\n")
	}
	b.WriteString("</repo_context>\n")
	return b.String()
}

func runContext(cwd string, stdout io.Writer) error {
	_, err := io.WriteString(stdout, contextPayload(cwd))
	return err
}

// runContextStats prints a deterministic token receipt: the payload size and
// how much denser the timeline is than the raw decision logs it distills.
func runContextStats(cwd string, stdout io.Writer) error {
	payload := contextPayload(cwd)
	fsTok := tokens.Estimate(ctxReadOrEmpty(filepath.Join(cwd, "docs", "file-structure.md")))
	tlTok := tokens.Estimate(ctxReadOrEmpty(filepath.Join(cwd, "docs", "timeline.md")))
	rawWhy := rawDecisionTokens(cwd)

	fmt.Fprint(stdout, "logmind context — token receipt (est. ~4 chars/token, deterministic)\n\n")
	fmt.Fprintf(stdout, "  payload total:  %6d tok  (file-structure %d + timeline %d + framing)\n",
		tokens.Estimate(payload), fsTok, tlTok)
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
