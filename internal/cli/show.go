// show.go — `logmind show` subcommand. Go port of src/logmind/cli.py's
// show command (Python `@main.command()` at cli.py:1186-1327) restored
// for v1.2.1 after being dropped in the v1.0 Go rewrite.
//
// SPEC §3.2 / §A.3 requires this command. Behavior mirrors Python
// v0.6.16 verbatim so consumers see the same shape regardless of
// whether they're running the legacy shim or the native Go binary.
//
// Surface (mirrors Python click options):
//
//   - default (no flags) → streams docs/decisions.md verbatim. Adds an
//     `ARCHIVED DECISIONS` block when --all is passed.
//   - --brief → one-line summary per entry (date + title + source).
//   - --limit N → cap to N most-recent entries (forces parsed view).
//   - --json → emit JSON array of {date, title, source} objects.
//     Mutually exclusive with --brief (JSON wins per Python semantics).
//   - --all/-a → include docs/decisions-archive.md alongside decisions.md.
//
// The `ok` trailer line (Python's `_ok()`) is preserved because agents
// chain on its `ok <key-value>` shape. In --json mode the ok line goes
// to stderr so stdout remains valid JSON for piping into `jq`.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/decisions"
)

// showFlags carries the parsed flags for `logmind show`. Default zero
// values match Python click defaults (limit=-1 sentinel for "no cap").
type showFlags struct {
	all   bool
	brief bool
	limit int
	asJSON bool
}

// newShowCmd wires the `logmind show` subcommand.
//
// Help text mirrors Python's docstring so consumers moving between the
// Python shim and the Go binary see identical guidance via `--help`.
func newShowCmd() *cobra.Command {
	f := &showFlags{limit: -1}
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show recent decisions",
		Long: `Show recent decisions.

Default: streams docs/decisions.md verbatim (the current 20 most-recent
entries; older are in decisions-archive.md, surface via --all).

Agent-friendly views (v0.5.2+):

  --brief                one-line summary per entry
  --limit N              cap to N most-recent
  --json                 structured array for parsing

Combinations are allowed: ` + "`logmind show --brief --limit 5`" + ` for a quick
last-5 recall, ` + "`logmind show --json --limit 10 --all`" + ` for parsed access
across main + archive.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runShow(cwd, f, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVarP(&f.all, "all", "a", false,
		"Show all decisions including archived")
	cmd.Flags().BoolVar(&f.brief, "brief", false,
		"One-line summary per decision (date + title) instead of "+
			"full markdown. Reduces ingest cost when agents read prior context.")
	cmd.Flags().IntVarP(&f.limit, "limit", "n", -1,
		"Show at most N most-recent decisions. Default: no limit "+
			"(full file when --brief absent; all parsed entries when --brief set).")
	cmd.Flags().BoolVar(&f.asJSON, "json", false,
		"Emit a JSON array of {date, title, source} objects. Stable schema "+
			"for downstream tools. Mutually exclusive with --brief (JSON wins).")
	return cmd
}

// runShow implements the show command logic. Mirrors Python's
// `def show()` at cli.py:1220-1327.
func runShow(cwd string, f *showFlags, stdout, stderr io.Writer) error {
	docsPath := filepath.Join(cwd, "docs")
	if !pathExists(docsPath) {
		fmt.Fprintln(stdout, "Error: docs/ directory not found. Run 'logmind init' first.")
		return ErrSilent
	}

	decisionsPath := filepath.Join(docsPath, "decisions.md")
	if !pathExists(decisionsPath) {
		if f.asJSON {
			fmt.Fprintln(stdout, "[]")
		} else {
			fmt.Fprintln(stdout, "No decisions logged yet.")
		}
		writeOk(stdout, stderr, f.asJSON, "show: 0 decisions (none logged yet)")
		return nil
	}

	// Default verbatim view (preserves pre-v0.5.2 behavior).
	if !(f.brief || f.asJSON || f.limit >= 0) {
		data, err := os.ReadFile(decisionsPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", decisionsPath, err)
		}
		fmt.Fprint(stdout, string(data))

		archiveShown := false
		if f.all {
			archivePath := filepath.Join(docsPath, "decisions-archive.md")
			if pathExists(archivePath) {
				fmt.Fprintln(stdout)
				fmt.Fprintln(stdout, repeat("=", 80))
				fmt.Fprintln(stdout, "ARCHIVED DECISIONS")
				fmt.Fprintln(stdout, repeat("=", 80))
				fmt.Fprintln(stdout)
				archiveData, err := os.ReadFile(archivePath)
				if err != nil {
					return fmt.Errorf("read %s: %w", archivePath, err)
				}
				fmt.Fprint(stdout, string(archiveData))
				archiveShown = true
			}
		}
		st, err := os.Stat(decisionsPath)
		if err != nil {
			return err
		}
		suffix := ""
		if archiveShown {
			suffix = " + archive"
		}
		fmt.Fprintf(stdout, "ok show: docs/decisions.md (%d bytes%s)\n", st.Size(), suffix)
		return nil
	}

	// Parsed-view paths (brief / limit / json). Load entries with provenance.
	type entry struct {
		Date   string `json:"date"`
		Title  string `json:"title"`
		Source string `json:"source"`
	}
	var entries []entry

	mainEntries, err := decisions.Iter(decisionsPath, stderr)
	if err != nil {
		return err
	}
	for _, e := range mainEntries {
		entries = append(entries, entry{
			Date:   e.Date.Format("2006-01-02T15:04:05"),
			Title:  e.Title,
			Source: "main",
		})
	}
	if f.all {
		archivePath := filepath.Join(docsPath, "decisions-archive.md")
		if pathExists(archivePath) {
			archiveEntries, err := decisions.Iter(archivePath, stderr)
			if err != nil {
				return err
			}
			for _, e := range archiveEntries {
				entries = append(entries, entry{
					Date:   e.Date.Format("2006-01-02T15:04:05"),
					Title:  e.Title,
					Source: "archive",
				})
			}
		}
	}

	// Sort newest-first so --limit N picks the latest entries.
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Date > entries[j].Date
	})
	if f.limit >= 0 && f.limit < len(entries) {
		entries = entries[:f.limit]
	}

	if f.asJSON {
		// ALWAYS emit JSON to stdout (matches Python's _orig_click_echo
		// bypass — JSON is the primary output for downstream parsers,
		// not progress chatter).
		buf, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return err
		}
		// Python's json.dumps emits [] for empty list; mirror exactly.
		if len(entries) == 0 {
			buf = []byte("[]")
		}
		fmt.Fprintln(stdout, string(buf))
	} else {
		// brief OR limit-only: render one-line-per-entry. Mirror Python
		// f-string: `{date strftime %Y-%m-%d %H:%M} — {title} [{source}]`.
		for _, e := range entries {
			// Re-parse the ISO timestamp into the display format.
			// (We could carry the time.Time but the JSON layer wants ISO.)
			displayDate := e.Date[:10] + " " + e.Date[11:16]
			fmt.Fprintf(stdout, "%s — %s [%s]\n", displayDate, e.Title, e.Source)
		}
	}

	// Route the ok line to stderr in JSON mode so stdout is parseable JSON.
	mode := "brief"
	if f.asJSON {
		mode = "json"
	}
	writeOk(stdout, stderr, f.asJSON,
		fmt.Sprintf("show: %d decisions (%s)", len(entries), mode))
	return nil
}

// writeOk prints the `ok <message>` agent-readable summary. Routed to
// stderr in JSON mode so stdout stays valid JSON for piping into `jq`.
// Mirrors Python's `_ok(msg, err=as_json)`.
func writeOk(stdout, stderr io.Writer, asJSON bool, msg string) {
	dest := stdout
	if asJSON {
		dest = stderr
	}
	fmt.Fprintf(dest, "ok %s\n", msg)
}

// repeat returns s repeated n times. Tiny helper to keep the file
// stdlib-pure (avoids pulling strings.Repeat into the show.go header).
func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
