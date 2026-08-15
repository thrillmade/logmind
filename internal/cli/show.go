// show.go — `logmind show` subcommand.
//
// SKILL.md / AGENTS.md / internal/templates/logmind-section.md all document
// `logmind show` as "recent decisions on the current branch" with `--all` to
// include the archive, but the v2 Go binary never shipped it (a v1.0
// rewrite gap; a retired PR #154 built a version against the pre-branch-aware
// v1 shape). This is the v2 re-implementation: it reads the SAME file `logmind
// log` would write to right now — resolveDecisionsPath's branch-aware target,
// not a hardcoded docs/decisions.md — so `show` always reflects "this branch's
// recent decisions", matching the documented contract.
//
// PROTOCOL SPEC §3.2 additionally specifies --all's "every branch decisions
// file" half (not just the archive), --brief, and --json with a NORMATIVE
// schema. This file now implements all three:
//
//   - default: streams the resolved decision file verbatim (UNCHANGED byte
//     for byte from the pre-§3.2 behavior — existing goldens still pin this).
//   - --all: appends every OTHER docs/decisions-branches/*.md file (the base
//     file, when on a branch, is already the primary body — never
//     duplicated) under BRANCH DECISIONS banners, when each exists.
//   - --brief: title + timestamp only, one line per decision. Under --all,
//     lines are grouped under a "[source]" tag matching the --json source
//     value exactly (main / branch:<name>).
//   - --json: the SPEC §3.2 NORMATIVE schema —
//     {"decisions":[{"title","timestamp","reasoning","alternatives":[],
//     "implications":[],"source":"main|branch:<name>"}]}. Stdout
//     carries ONLY the JSON document — no chatter, no ok trailer, regardless
//     of --quiet, so it is always pipeable into `jq` unmodified.
//   - --brief --json: the schema's keys never change (NORMATIVE — same key
//     names, same nesting), but --brief's "title + timestamp only" contract
//     wins for CONTENT: reasoning/alternatives/implications are present but
//     zeroed ("" / [] / []) rather than parsed out of the entry body, since
//     --brief's whole point is to skip exactly that.
//
// Entry parsing reuses internal/decisions.SplitRaw/SplitRawBytes (the
// header-boundary byte-range splitter added for SPEC §1.3.2 rotation) rather
// than writing a new decisions-file parser. Only the Reasoning /
// Alternatives considered / Implications sub-field extraction (needed for
// --json, unique to this command) is new — see parseDecisionBody.
//
// Quiet discipline (quiet.go): under --quiet/LOGMIND_QUIET the verbatim body
// is suppressed — the deliverable a human wants (the markdown) isn't
// "chatter", but an agent that opted into quiet mode wants a receipt, not the
// payload, matching `logmind repomap`'s stdout-sink precedent. The single `ok`
// line always carries the byte count so a script can tell empty from missing.
// --json ignores --quiet entirely (see above) — the JSON output already IS
// the machine-clean contract quiet mode exists to approximate elsewhere.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/config"
	"github.com/thrillmade/logmind/internal/decisions"
	"github.com/thrillmade/logmind/internal/gitcli"
)

// newShowCmd wires the `logmind show` subcommand.
func newShowCmd() *cobra.Command {
	var all, brief, jsonOut bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show recent decisions on the current branch",
		Long: `Show recent decisions on the current branch.

Streams the decision file "logmind log" would write to right now:
docs/decisions-branches/<branch>.md for the branch you are on — the default
branch included, it is a branch like any other.

Pass --all to also include every OTHER docs/decisions-branches/<branch>.md
file, each appended under its own banner.

Pass --brief for title + timestamp only, one line per decision (under --all,
lines are grouped by source).

Pass --json for structured output matching PROTOCOL SPEC section sec-3-2's
NORMATIVE schema:
    {"decisions":[{"title","timestamp","reasoning","alternatives":[],
    "implications":[],"source":"main|branch:<name>"}]}
--json is machine-clean: stdout carries ONLY the JSON document, safe to pipe
into jq unmodified, regardless of --quiet.

--brief --json together keep the full schema (all keys always present, per
the NORMATIVE contract) but zero out reasoning/alternatives/implications
("" / [] / []) rather than parsing them, honoring --brief's "title +
timestamp only" contract for content while never dropping a schema key.

Examples:
    logmind show
    logmind show --all
    logmind show --brief
    logmind show --all --json
    logmind show --brief --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runShow(cwd, all, brief, jsonOut, quietEnabled(cmd), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&all, "all", false,
		"Also include every other docs/decisions-branches/*.md file, each under its own banner.")
	cmd.Flags().BoolVar(&brief, "brief", false,
		"Title + timestamp only, one line per decision. Combined with --json, zeroes reasoning/alternatives/implications instead of dropping them.")
	cmd.Flags().BoolVar(&jsonOut, "json", false,
		"Structured output matching PROTOCOL SPEC section sec-3-2's NORMATIVE schema. Machine-clean: stdout is JSON only.")
	return cmd
}

// showSource pairs a decisions file path with its NORMATIVE `source` label
// (SPEC section sec-3-2's grammar: "main" | "archive" | "branch:<name>").
type showSource struct {
	path  string
	label string
}

// branchLabelFromFilename reverses logmind log's branch-name sanitization
// (sanitizeBranchName in log.go: "/" and "\\" -> "__", ":" -> "_") well
// enough for display/source-tag purposes: strip ".md", turn "__" back into
// "/". Mirrors decisions.branchLabelFromFilename (unexported in
// internal/decisions, used there for timeline source labels) — duplicated
// here rather than exported across a package boundary for one call site.
func branchLabelFromFilename(name string) string {
	stem := strings.TrimSuffix(name, ".md")
	return strings.ReplaceAll(stem, "__", "/")
}

// listBranchSources returns every docs/decisions-branches/*.md file as a
// showSource labeled "branch:<name>", excluding excludePath so a base file
// already shown as the primary body is never duplicated. Order matches
// decisions.ListBranchFiles (sorted by filename).
func listBranchSources(docsPath, excludePath string) ([]showSource, error) {
	dir := filepath.Join(docsPath, "decisions-branches")
	files, err := decisions.ListBranchFiles(dir)
	if err != nil {
		return nil, err
	}
	var out []showSource
	for _, f := range files {
		if f == excludePath {
			continue
		}
		out = append(out, showSource{
			path:  f,
			label: "branch:" + branchLabelFromFilename(filepath.Base(f)),
		})
	}
	return out, nil
}

// allExtraSources returns the ordered list of sources `--all` contributes
// beyond the base file: every OTHER branch decisions file (see
// listBranchSources). Shared by --brief and --json so both group sources
// identically with the default raw-stream mode below.
//
// It used to append docs/decisions-archive.md last. There is no decision
// archive any more (SPEC §3.2 — nothing rotates, nothing overflows), so the
// set is exactly the branch files.
func allExtraSources(docsPath, excludePath string) ([]showSource, error) {
	return listBranchSources(docsPath, excludePath)
}

// showJSONEntry mirrors SPEC section sec-3-2's NORMATIVE --json schema
// EXACTLY: same key names, same nesting, same source grammar. Do not add,
// rename, or drop keys here — it is a wire contract, not an internal type.
type showJSONEntry struct {
	Title        string   `json:"title"`
	Timestamp    string   `json:"timestamp"`
	Reasoning    string   `json:"reasoning"`
	Alternatives []string `json:"alternatives"`
	Implications []string `json:"implications"`
	Source       string   `json:"source"`
}

// showJSONOutput is the top-level --json document: {"decisions": [...]}.
type showJSONOutput struct {
	Decisions []showJSONEntry `json:"decisions"`
}

// collectShowEntries gathers every decision across basePath (labeled
// baseLabel) plus, when all is true, allExtraSources(docsPath, basePath) —
// the SPEC section sec-3-2 "--all: include archive and every branch
// decisions file" set. withBody controls whether the Reasoning /
// Alternatives / Implications sub-fields are parsed out of each entry's raw
// text (skipped under --brief --json: those fields stay zeroed either way,
// so parsing them would be wasted work).
//
// Entries within each source preserve on-disk (chronological, oldest-first)
// order — every decision file is append-only — and sources are concatenated
// in the order they're visited (base, then branches).
func collectShowEntries(docsPath, basePath, baseLabel string, all, withBody bool) ([]showJSONEntry, error) {
	var out []showJSONEntry

	appendFile := func(path, label string) error {
		_, raws, err := decisions.SplitRaw(path)
		if err != nil {
			return err
		}
		for _, r := range raws {
			e := showJSONEntry{
				Title:        r.Title,
				Timestamp:    r.Date.Format("2006-01-02 15:04"),
				Alternatives: []string{},
				Implications: []string{},
				Source:       label,
			}
			if withBody {
				reasoning, alts, impls := parseDecisionBody(r.Raw)
				e.Reasoning = reasoning
				if len(alts) > 0 {
					e.Alternatives = alts
				}
				if len(impls) > 0 {
					e.Implications = impls
				}
			}
			out = append(out, e)
		}
		return nil
	}

	if err := appendFile(basePath, baseLabel); err != nil {
		return nil, err
	}
	if all {
		extra, err := allExtraSources(docsPath, basePath)
		if err != nil {
			return nil, err
		}
		for _, s := range extra {
			if err := appendFile(s.path, s.label); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// parseDecisionBody extracts the Reasoning / Alternatives considered /
// Implications sections from one decision entry's raw text (a
// decisions.RawEntry.Raw value), matching buildDecisionEntry's layout
// (log.go) byte-for-byte:
//
//	## YYYY-MM-DD HH:MM - <summary>
//
//	**Reasoning:** <reasoning>
//
//	**Alternatives considered:** alt1, alt2, alt3
//
//	**Implications:**
//	- impl1
//	- impl2
//
//	---
//
// Sections are optional (an entry logged without -r/-a/-i, or a merge-commit
// entry using the unrelated **PR:**/**Decisions:**/**Detail:** bullet shape,
// has none of these lines) and line-oriented, so this is a small line-by-line
// scan rather than a single regex — matching the "structural split, not
// regex" style decisions.Iter/SplitRawBytes already use.
//
// KNOWN LOSSY EDGE CASE: buildDecisionEntry joins the alternatives slice with
// ", " into a single stored string (log.go: strings.Join(alternatives, ", "))
// — the original element boundaries are not preserved in the file if any
// alternative's own text contains ", ". Splitting back on ", " here is the
// best available reconstruction of that already-collapsed data; it is not a
// bug introduced by this parser.
func parseDecisionBody(raw string) (reasoning string, alternatives, implications []string) {
	lines := strings.Split(raw, "\n")
	i := 0
	for i < len(lines) {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "**Reasoning:** "):
			reasoning = strings.TrimPrefix(line, "**Reasoning:** ")
			i++
			for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
				reasoning += "\n" + lines[i]
				i++
			}
		case strings.HasPrefix(line, "**Alternatives considered:** "):
			joined := strings.TrimPrefix(line, "**Alternatives considered:** ")
			i++
			for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
				joined += "\n" + lines[i]
				i++
			}
			for _, a := range strings.Split(joined, ", ") {
				if a = strings.TrimSpace(a); a != "" {
					alternatives = append(alternatives, a)
				}
			}
		case strings.HasPrefix(line, "**Implications:**"):
			i++
			for i < len(lines) && strings.HasPrefix(lines[i], "- ") {
				implications = append(implications, strings.TrimPrefix(lines[i], "- "))
				i++
			}
		default:
			i++
		}
	}
	return reasoning, alternatives, implications
}

// writeBriefEntries prints entries in "--brief" text form: one
// "TIMESTAMP - TITLE" line per decision. Under --all (grouped, more than the
// base source), each new source starts a "[source]" tag line matching the
// --json source value exactly (main / archive / branch:<name>), so brief
// text output and --json output agree on source identity.
func writeBriefEntries(stdout io.Writer, entries []showJSONEntry, all bool) {
	if len(entries) == 0 {
		fmt.Fprintln(stdout, "No decisions logged yet on this branch.")
		return
	}
	currentSource := ""
	first := true
	for _, e := range entries {
		if all && e.Source != currentSource {
			currentSource = e.Source
			if !first {
				fmt.Fprintln(stdout)
			}
			fmt.Fprintf(stdout, "[%s]\n", currentSource)
		}
		fmt.Fprintf(stdout, "%s - %s\n", e.Timestamp, e.Title)
		first = false
	}
}

// runShow implements `logmind show`.
func runShow(cwd string, all, brief, jsonOut, quiet bool, stdout, stderr io.Writer) error {
	q := newQout(quiet, stdout, stderr)
	docsPath := filepath.Join(cwd, "docs")
	if !pathExists(docsPath) {
		q.fail("Error: docs/ directory not found. Run 'logmind init' first.\n")
		return ErrSilent
	}

	cfg, _ := config.Load(cwd)
	target, isBranchFile := resolveDecisionsPath(cwd, docsPath, cfg)
	rel := relForOk(cwd, target)

	baseLabel := "main"
	if isBranchFile {
		branch := gitcli.CurrentBranch(cwd)
		if branch == "" {
			// Detached HEAD / unborn repo shouldn't reach here (resolveDecisionsPath
			// would have returned isBranchFile=false), but fall back to reversing
			// the filename's sanitization rather than emitting an empty label.
			branch = branchLabelFromFilename(filepath.Base(target))
		}
		baseLabel = "branch:" + branch
	}

	// --json: SPEC section sec-3-2's NORMATIVE schema. Always the full key
	// set; --brief only zeroes reasoning/alternatives/implications (see
	// collectShowEntries's withBody=!brief). No chatter, no ok trailer,
	// --quiet has no additional effect — stdout is the JSON document, full
	// stop.
	if jsonOut {
		entries, err := collectShowEntries(docsPath, target, baseLabel, all, !brief)
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(showJSONOutput{Decisions: entries}, "", "  ")
		if err != nil {
			return err
		}
		if _, err := stdout.Write(data); err != nil {
			return err
		}
		fmt.Fprintln(stdout)
		return nil
	}

	// --brief (text, non-JSON): title + timestamp only.
	if brief {
		entries, err := collectShowEntries(docsPath, target, baseLabel, all, false)
		if err != nil {
			return err
		}
		if quiet {
			q.ok("show brief=true all=%t decisions=%d", all, len(entries))
			return nil
		}
		writeBriefEntries(stdout, entries, all)
		suffix := ""
		if all {
			suffix = " (--all)"
		}
		fmt.Fprintf(stdout, "ok show: %d decision(s)%s\n", len(entries), suffix)
		return nil
	}

	var body string
	if pathExists(target) {
		data, err := os.ReadFile(target)
		if err != nil {
			return fmt.Errorf("read %s: %w", target, err)
		}
		body = string(data)
	}

	// --all's branch-file half of SPEC section sec-3-2 ("include archive and
	// every branch decisions file"): every OTHER docs/decisions-branches/*.md
	// file, appended verbatim under its own BRANCH DECISIONS banner, ordered
	// before the archive section below (unchanged from before this feature).
	type branchBlock struct {
		label string
		body  string
	}
	var branchBlocks []branchBlock
	if all {
		branchSrcs, err := listBranchSources(docsPath, target)
		if err != nil {
			return err
		}
		for _, s := range branchSrcs {
			data, err := os.ReadFile(s.path)
			if err != nil {
				return fmt.Errorf("read %s: %w", s.path, err)
			}
			branchBlocks = append(branchBlocks, branchBlock{label: s.label, body: string(data)})
		}
	}

	if quiet {
		q.ok("show path=%s bytes=%d all=%t branches=%d", rel, len(body), all, len(branchBlocks))
		return nil
	}

	if body == "" {
		fmt.Fprintln(stdout, "No decisions logged yet on this branch.")
	} else {
		fmt.Fprint(stdout, body)
	}

	for _, b := range branchBlocks {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, strings.Repeat("=", 80))
		fmt.Fprintf(stdout, "BRANCH DECISIONS: %s\n", strings.TrimPrefix(b.label, "branch:"))
		fmt.Fprintln(stdout, strings.Repeat("=", 80))
		fmt.Fprintln(stdout)
		fmt.Fprint(stdout, b.body)
	}

	suffix := ""
	if all {
		if len(branchBlocks) > 0 {
			suffix = fmt.Sprintf(" + %d branch file(s)", len(branchBlocks))
		} else {
			suffix = " (no other branch files)"
		}
	}
	fmt.Fprintf(stdout, "ok show: %s (%d bytes%s)\n", rel, len(body), suffix)
	return nil
}
