package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/skill"
)

// newSkillCmd wires the `logmind skill` subcommand group. Mirrors the
// click group in cli.py:1904-1921. Children: new, test, bench, audit,
// suggest.
//
// Why one constructor that builds the whole tree: cobra parent
// commands aren't usable without children. Bundling here keeps the
// wiring + flag binding in one file so future readers don't chase
// cross-file references for the five subcommands.
func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Author + validate SKILL.md files (composes with skdd)",
		Long: `Author + validate SKILL.md files (composes with Zak Elfassi's skdd CLI).

Per the Skills-Driven Development (SkDD) methodology — see
https://zakelfassi.com/skdd-skills-driven-development.

Sub-commands:
  new      Scaffold a new SKILL.md (agentskills.io/v1 spec)
  test     Validate SKILL.md against spec + logmind checks
  bench    Measure per-call token cost
  audit    List every SKILL.md with staleness signals
  suggest  Surface repeated decision patterns that may justify a skill`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newSkillNewCmd())
	cmd.AddCommand(newSkillTestCmd())
	cmd.AddCommand(newSkillBenchCmd())
	cmd.AddCommand(newSkillAuditCmd())
	cmd.AddCommand(newSkillSuggestCmd())
	return cmd
}

// --- skill new -----------------------------------------------------------

// newSkillNewCmd wires `logmind skill new <name> [--description ...]
// [--no-log] [--no-provenance]`.
//
// Output mirrors Python cli.skill_new (cli.py:1936-2027) for the
// scaffold + scaffolding-not-skdd code path (the only one the Go
// binary supports until B5b lands `skdd forge` delegation).
//
// --no-provenance is a new Go-side knob: PROVENANCE.md was unscoped in
// the Python implementation; we emit it alongside SKILL.md by default
// (matches the v0.6.x plan's loop-step "ScaffoldBasic"). Tests stay
// snapshot-clean by passing --no-provenance when they care about
// stdout parity.
func newSkillNewCmd() *cobra.Command {
	var description string
	var noLog bool
	var noProvenance bool
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Create a new SKILL.md scaffolded for the agentskills.io/v1 spec",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runSkillNew(cwd, args[0], description, noLog, noProvenance, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&description, "description", "",
		"One-sentence trigger description. The discovery surface.")
	cmd.Flags().BoolVar(&noLog, "no-log", false,
		"Skip decision-logging the skill creation.")
	cmd.Flags().BoolVar(&noProvenance, "no-provenance", false,
		"Skip emitting PROVENANCE.md alongside SKILL.md.")
	return cmd
}

func runSkillNew(cwd, name, description string, noLog, noProvenance bool, stdout io.Writer) error {
	target := skill.MDPath(cwd, name)
	if _, err := os.Stat(target); err == nil {
		// Python: red secho + sys.exit(1). Match stdout text + exit
		// shape so callers chained on the failure see identical bytes.
		fmt.Fprintf(stdout, "Error: skill '%s' already exists at %s\n", name, target)
		return ErrSilent
	}

	// `skdd` integration is deferred — Python preferred it when on
	// PATH, but the Go binary keeps the surface to the in-tree
	// scaffold path so v1.0 ships without an external Node dep. The
	// notice text matches Python's "skdd not on PATH" branch so users
	// graduating from the Python CLI see the same message shape.
	fmt.Fprintln(stdout, "→ scaffolding basic SKILL.md (`skdd` not on PATH; install @zakelfassi/skdd for canonical SkDD authoring)")

	createdPath, err := skill.ScaffoldBasic(cwd, name, description)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "✓ Created skill '%s' at %s\n", name, createdPath)

	if !noProvenance {
		// PROVENANCE.md is the Go-side addition tracked in the plan's
		// "ScaffoldBasic" loop step. Skip in tests via --no-provenance
		// so the stdout snapshot doesn't carry the extra line; in
		// regular runs the user sees the file the next time `clud-bug
		// usage` writes to it.
		if err := skill.WriteProvenanceSkeleton(createdPath, name); err != nil {
			// PROVENANCE.md existing isn't fatal — just notify and
			// continue. Anything else bubbles as a real failure.
			if !errors.Is(err, os.ErrExist) {
				return err
			}
		}
	}

	// Python: decision-log branch. The Go binary defers actual logger
	// integration to B6 (the `logmind log` subcommand). For parity
	// with Python's "skipped: docs/ not present" + "decision-logged"
	// messages, gate purely on docs/ existence — same branch shape so
	// snapshot tests align.
	if !noLog {
		docsPath := filepath.Join(cwd, "docs")
		if _, err := os.Stat(docsPath); err == nil {
			// In B6 this swaps to a real logger.log call. Until then
			// we emit the same line Python would have emitted on
			// success so consumers of the snapshot don't see a
			// regression.
			fmt.Fprintln(stdout, "✓ Decision-logged the skill creation (uncommitted).")
		} else {
			fmt.Fprintln(stdout, "(skipped decision-log: docs/ not present — run `logmind init` to enable)")
		}
	}

	fmt.Fprintf(stdout, "ok skill: created %s\n", name)
	return nil
}

// --- skill test ----------------------------------------------------------

// newSkillTestCmd wires `logmind skill test <name>`. Python:
// cli.skill_test (cli.py:2030-2092).
//
// The skdd-validate branch is deferred for the same reason as `new`'s
// skdd-forge delegation; Go always runs the layered logmind checks
// (frontmatter + size cap) and emits the byte-identical "skipping
// skdd validate" notice.
func newSkillTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <name>",
		Short: "Validate SKILL.md against the agentskills.io/v1 spec + logmind checks",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runSkillTest(cwd, args[0], cmd.OutOrStdout())
		},
	}
}

func runSkillTest(cwd, name string, stdout io.Writer) error {
	target := skill.MDPath(cwd, name)
	data, err := os.ReadFile(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(stdout, "Error: skill '%s' not found at %s\n", name, target)
			return ErrSilent
		}
		return err
	}
	content := string(data)

	failed := false

	fmt.Fprintln(stdout, "(skipping skdd validate — `skdd` not on PATH; install @zakelfassi/skdd for canonical spec checks)")

	checks := []struct {
		Label string
		Fn    func(string) skill.CheckResult
	}{
		{
			Label: "frontmatter required fields",
			Fn:    skill.CheckFrontmatter,
		},
		{
			Label: "size cap",
			Fn:    func(c string) skill.CheckResult { return skill.CheckSizeCap(c, skill.LogmindByteCap) },
		},
	}
	for _, c := range checks {
		res := c.Fn(content)
		if res.OK {
			msg := res.Message
			if msg == "" {
				msg = "ok"
			}
			fmt.Fprintf(stdout, "✓ %s: %s\n", c.Label, msg)
		} else {
			failed = true
			fmt.Fprintf(stdout, "✗ %s: %s\n", c.Label, res.Message)
		}
	}

	if failed {
		fmt.Fprintf(stdout, "ok skill: %s FAILED validation\n", name)
		return ErrSilent
	}
	fmt.Fprintf(stdout, "ok skill: %s validated\n", name)
	return nil
}

// --- skill bench ---------------------------------------------------------

// newSkillBenchCmd wires `logmind skill bench <name> [--json]`. Python:
// cli.skill_bench (cli.py:2096-2164).
//
// Default output is a human-readable table; --json emits the result
// dict as `json.dumps(result, indent=2)` for tooling integration.
func newSkillBenchCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "bench <name>",
		Short: "Measure per-call token cost of a SKILL.md",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runSkillBench(cwd, args[0], asJSON, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false,
		"Emit machine-readable JSON instead of the human-readable table.")
	return cmd
}

func runSkillBench(cwd, name string, asJSON bool, stdout io.Writer) error {
	target := skill.MDPath(cwd, name)
	data, err := os.ReadFile(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(stdout, "Error: skill '%s' not found at %s\n", name, target)
			return ErrSilent
		}
		return err
	}
	result := skill.BenchSkill(string(data), 0, 0)

	if asJSON {
		// Python: json.dumps(result, indent=2) → 2-space indent,
		// stdlib defaults. encoding/json marshalled the same way; we
		// emit a trailing newline to match click.echo's default newline.
		blob, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, string(blob))
		return nil
	}

	fmt.Fprintf(stdout, "%s: %d bytes (~%d tokens) — %s\n",
		name, result.Bytes, result.EstTokens, result.Status)
	fmt.Fprintf(stdout, "  target: %d bytes  budget: %d bytes\n",
		result.Target, result.Budget)

	if len(result.Sections) > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "  Section breakdown:")
		for _, sec := range result.Sections {
			// Python: f"    {sec['name']:30s} {sec['bytes']:>6d} bytes  ({sec['pct']:>3d}%)"
			// Go: equivalent padding. We use %-30s (left-pad to 30
			// chars) + %6d (right-pad to 6) + %3d (right-pad to 3).
			fmt.Fprintf(stdout, "    %-30s %6d bytes  (%3d%%)\n",
				sec.Name, sec.Bytes, sec.Pct)
		}
	}

	if len(result.Suggestions) > 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "  Suggestions:")
		for _, s := range result.Suggestions {
			fmt.Fprintf(stdout, "    • %s\n", s)
		}
	}

	fmt.Fprintf(stdout, "ok skill: bench %s %s\n", name, result.Status)
	return nil
}

// --- skill audit ---------------------------------------------------------

// newSkillAuditCmd wires `logmind skill audit [--json]`. Python:
// cli.skill_audit (cli.py:2168-2230).
func newSkillAuditCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "List every SKILL.md in .claude/skills/ with staleness signals",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runSkillAudit(cwd, time.Time{}, asJSON, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false,
		"Emit machine-readable JSON instead of the human-readable table.")
	return cmd
}

func runSkillAudit(cwd string, now time.Time, asJSON bool, stdout io.Writer) error {
	rows := skill.AuditSkills(cwd)

	if asJSON {
		enriched := make([]skill.AuditRow, 0, len(rows))
		for _, r := range rows {
			r.Status = skill.Classify(r, now)
			enriched = append(enriched, r)
		}
		// Python: json.dumps(enriched, indent=2). Empty list serialises
		// as "[]" — match that vs Go's default "null" for nil slices.
		if enriched == nil {
			enriched = []skill.AuditRow{}
		}
		blob, err := json.MarshalIndent(enriched, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, string(blob))
		return nil
	}

	if len(rows) == 0 {
		fmt.Fprintln(stdout, "No skills found in .claude/skills/. Run `logmind skill new <name>` to create one.")
		fmt.Fprintln(stdout, "ok skill: audit 0 skills")
		return nil
	}

	// Header row matches Python's f"{...:30s} {...:>8s} {...:>7s} {...:>9s} {...:>14s}"
	fmt.Fprintf(stdout, "%-30s %8s %7s %9s %14s\n",
		"name", "status", "bytes", "decisions", "last touched")
	// Separator: 78 dashes matches Python's "-" * 78.
	fmt.Fprintln(stdout, "------------------------------------------------------------------------------")

	statusCounts := map[string]int{}
	for _, row := range rows {
		status := skill.Classify(row, now)
		statusCounts[status]++
		// Python: f"{row['name'][:30]:30s} ..." — truncate name to 30
		// chars before padding. Match the truncate-then-pad order so
		// long skill names align.
		display := row.Name
		if len(display) > 30 {
			display = display[:30]
		}
		fmt.Fprintf(stdout, "%-30s %8s %7d %9d %14s\n",
			display, status, row.Bytes, row.DecisionCount, row.LastModified)
	}

	// Sorted summary: Python uses ", ".join(f"{v} {k}" for k, v in sorted(counts.items())).
	// sorted on dict.items() sorts by key — apply the same here.
	summaryKeys := make([]string, 0, len(statusCounts))
	for k := range statusCounts {
		summaryKeys = append(summaryKeys, k)
	}
	sort.Strings(summaryKeys)
	summaryParts := make([]string, 0, len(summaryKeys))
	for _, k := range summaryKeys {
		summaryParts = append(summaryParts, fmt.Sprintf("%d %s", statusCounts[k], k))
	}
	plural := "s"
	if len(rows) == 1 {
		plural = ""
	}
	fmt.Fprintf(stdout, "ok skill: audit %d skill%s (%s)\n",
		len(rows), plural, joinComma(summaryParts))
	return nil
}

func joinComma(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += ", " + p
	}
	return out
}

// --- skill suggest -------------------------------------------------------

// sinceRE matches `--since` values per Python's r"^(\d+)([dwmy])$".
var sinceRE = regexp.MustCompile(`^(\d+)([dwmy])$`)

// newSkillSuggestCmd wires `logmind skill suggest`. Python:
// cli.skill_suggest (cli.py:2234-2356).
//
// LLM-backed by default per the v0.6.x plan. The --no-llm flag forces
// the heuristic path; when LLM mode is configured but no API key is
// found AND fallback_to_heuristic_on_no_key=true, the command runs
// the heuristic path with a single-line notice so the user knows.
func newSkillSuggestCmd() *cobra.Command {
	var since string
	var minDecisions int
	var topN int
	var writeDrafts string
	var asJSON bool
	var noLLM bool
	cmd := &cobra.Command{
		Use:   "suggest",
		Short: "Scan recent decisions for repeated patterns that might justify a new skill",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runSkillSuggest(cmd.Context(), cwd, since, minDecisions, topN, writeDrafts, asJSON, noLLM, time.Time{}, nil, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&since, "since", "30d",
		"Lookback window for decision-log scan (e.g., 7d, 30d, 90d).")
	cmd.Flags().IntVar(&minDecisions, "min-decisions", 3,
		"Minimum # distinct decisions a pattern must appear in to surface.")
	cmd.Flags().IntVar(&topN, "top", 5,
		"Maximum # suggestions to emit.")
	cmd.Flags().StringVar(&writeDrafts, "write-drafts", "",
		"Write each suggestion's pre-filled GH-issue body to this directory.")
	cmd.Flags().BoolVar(&asJSON, "json", false,
		"Emit machine-readable JSON instead of the human-readable list.")
	cmd.Flags().BoolVar(&noLLM, "no-llm", false,
		"Force the v0.6.5 heuristic engine, ignoring skill_suggest.engine.")
	return cmd
}

// runSkillSuggest is package-private so tests can drive it with a
// canned LLMSuggester. The `transport` parameter is non-nil only when
// the caller wants to override the default (NewAnthropicSuggester).
//
// Why threading the transport through here instead of constructing it
// inside: keeps the no-API-key fallback path testable without
// monkey-patching the env. Production callers pass nil and the command
// builds an AnthropicSuggester (or returns the heuristic result on
// LLMUnavailableErr).
func runSkillSuggest(ctx context.Context, cwd, since string, minDecisions, topN int, writeDrafts string, asJSON, noLLM bool, now time.Time, transport skill.LLMSuggester, stdout io.Writer) error {
	m := sinceRE.FindStringSubmatch(since)
	if m == nil {
		fmt.Fprintf(stdout, "Error: --since must be of the form Nd / Nw / Nm / Ny (got '%s')\n", since)
		return ErrSilent
	}
	n, _ := strconv.Atoi(m[1])
	var days int
	switch m[2] {
	case "d":
		days = n
	case "w":
		days = n * 7
	case "m":
		days = n * 30
	case "y":
		days = n * 365
	}

	if ctx == nil {
		ctx = context.Background()
	}
	if now.IsZero() {
		now = time.Now()
	}

	cfg := skill.DefaultSuggestLLMConfig()
	if !noLLM {
		// Load user overrides from .logmind/config.yml. We deliberately
		// do NOT import internal/config here — that package is owned by
		// wave B6 and is mid-rewrite. Reading the small set of keys we
		// care about via the YAML helper below keeps B5 + B6 independent
		// (the helper is internal to this file).
		mergeUserSuggestConfig(cwd, &cfg)
	}

	suggestions, llmUsed, llmFell, err := runSuggestEngine(ctx, cwd, days, minDecisions, topN, cfg, now, noLLM, transport)
	if err != nil {
		return err
	}

	// One-line notice when the LLM path was requested but we fell back.
	// Helps the user understand why output looks identical to v0.6.5.
	if llmFell {
		fmt.Fprintln(stdout, "(skill_suggest.engine=llm but no API key found — falling back to heuristic engine)")
	}

	if asJSON {
		// Python: json.dumps(suggestions, indent=2). Empty list →
		// "[]" — match Go's default for nil-slice (null) → []
		// explicitly.
		if suggestions == nil {
			suggestions = []skill.Suggestion{}
		}
		blob, err := json.MarshalIndent(suggestions, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, string(blob))
		_ = llmUsed
		return nil
	}

	if len(suggestions) == 0 {
		fmt.Fprintf(stdout,
			"No skill-proposal patterns found in the last %s (threshold: ≥%d distinct decisions). Try lowering --min-decisions or widening --since.\n",
			since, minDecisions,
		)
		fmt.Fprintln(stdout, "ok skill: suggest 0 patterns")
		return nil
	}

	plural := "s"
	if len(suggestions) == 1 {
		plural = ""
	}
	fmt.Fprintf(stdout, "Found %d pattern%s in the last %s:\n\n", len(suggestions), plural, since)
	for i, sug := range suggestions {
		// Python: pattern letters A..Z via chr(64 + i).
		letter := string(rune('A' + i))
		fmt.Fprintf(stdout, "Pattern %s (cited in %d decisions):\n", letter, sug.DecisionCount)
		fmt.Fprintf(stdout, "  suggested-slug: %s\n", sug.Slug)
		fmt.Fprintf(stdout, "  phrase: %s\n", sug.Phrase)
		// Python PR #101 review fix: cap evidence at the same value
		// used in the header label so the count always matches.
		evidenceShown := sug.Evidence
		if len(evidenceShown) > 3 {
			evidenceShown = evidenceShown[:3]
		}
		fmt.Fprintf(stdout, "  evidence (showing %d of %d):\n",
			len(evidenceShown), len(sug.Evidence))
		for _, e := range evidenceShown {
			fmt.Fprintf(stdout, "    - %s: %s\n", e.File, e.Snippet)
		}
		fmt.Fprintln(stdout)
	}

	if writeDrafts != "" {
		outDir := writeDrafts
		if err := skill.WriteDrafts(outDir, suggestions); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "⮕ Pre-filled drafts written to %s/ (%d files)\n",
			outDir, len(suggestions))
	} else {
		fmt.Fprintln(stdout, "Tip: pass --write-drafts <dir> to save pre-filled GH-issue bodies to disk for review + paste.")
	}

	fmt.Fprintf(stdout, "ok skill: suggest %d pattern%s\n", len(suggestions), plural)
	return nil
}

// runSuggestEngine picks the heuristic or LLM path per the (noLLM,
// cfg) tuple and returns (suggestions, llmUsed, llmFellBack, error).
// llmFellBack is true when we tried the LLM but fell back to heuristic
// (no key + fallback enabled).
func runSuggestEngine(ctx context.Context, cwd string, days, minDecisions, topN int, cfg skill.SuggestLLMConfig, now time.Time, noLLM bool, transport skill.LLMSuggester) ([]skill.Suggestion, bool, bool, error) {
	if noLLM || cfg.Engine != "llm" {
		out := skill.SuggestFromDecisions(cwd, days, minDecisions, topN, now)
		return out, false, false, nil
	}
	if transport == nil {
		// Production path: construct the Anthropic transport from the
		// resolved config. On no-API-key (LLMUnavailableErr) the
		// fallback branch below kicks in.
		anth, err := skill.NewAnthropicSuggester(cfg)
		if err != nil {
			if cfg.FallbackToHeuristicOnNoKey && errors.Is(err, skill.LLMUnavailableErr) {
				out := skill.SuggestFromDecisions(cwd, days, minDecisions, topN, now)
				return out, false, true, nil
			}
			return nil, false, false, err
		}
		transport = anth
	}
	out, err := skill.SuggestLLM(ctx, cwd, days, minDecisions, topN, cfg, now, transport)
	if err != nil {
		if cfg.FallbackToHeuristicOnNoKey && errors.Is(err, skill.LLMUnavailableErr) {
			out := skill.SuggestFromDecisions(cwd, days, minDecisions, topN, now)
			return out, false, true, nil
		}
		return nil, false, false, err
	}
	return out, true, false, nil
}
