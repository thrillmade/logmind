// quiet.go — the LOGMIND_QUIET output discipline (token-killer Phase 1b).
//
// Borrowed from clud-bug's proven CLI pattern (CLUD_BUG_QUIET): an agent
// that invokes a verb wants ONE chainable, machine-parseable line — not a
// paragraph of ✓-progress chatter it has to pay tokens to read and skip.
//
// Under QUIET (the `--quiet` persistent root flag OR LOGMIND_QUIET in the
// environment) each wired verb:
//
//   - SUPPRESSES its progress/chatter lines (the ✓ / "already up to date"
//     lines) — chat().
//   - Emits EXACTLY ONE chainable `ok <k=v ...>` summary line to stdout —
//     ok().
//   - Routes errors + recovery hints to stderr, NEVER suppressed — fail()
//     lands them on stderr under QUIET (clud-bug's own regression lesson:
//     a quiet mode that swallows errors is worse than useless).
//
// QUIET is strictly OPT-IN. The default (no flag, no env) path keeps every
// byte of the historical output — chat(), fail(), and the legacy `ok`
// trailers all write to stdout exactly as before — so the timeline / tree /
// cli goldens stay green. The quiet MODE is additive; it never rewrites the
// default MODE.
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// quietEnvVar is the environment opt-in, the output-side twin of clud-bug's
// CLUD_BUG_QUIET. Any non-empty value other than a falsey word enables it,
// so `LOGMIND_QUIET=1` (the documented form) and `LOGMIND_QUIET=true` both
// work while `LOGMIND_QUIET=0` / unset stay in the byte-stable default.
const quietEnvVar = "LOGMIND_QUIET"

// quietFlagName is the persistent root flag registered in root.go.
const quietFlagName = "quiet"

// quietEnabled reports whether QUIET output discipline is active for this
// invocation: the persistent --quiet flag (inherited by every subcommand)
// OR LOGMIND_QUIET in the environment. Opt-in only.
//
// Precedence: an EXPLICIT flag on the command line wins over the env var
// (standard 12-factor precedence — explicit flag > environment). So
// `--quiet=false` deliberately turns quiet OFF even when LOGMIND_QUIET=1,
// and `--quiet` turns it ON regardless of the env. The env var only decides
// when the flag was not passed at all.
func quietEnabled(cmd *cobra.Command) bool {
	if cmd != nil {
		if f := cmd.Flags().Lookup(quietFlagName); f != nil {
			if b, err := cmd.Flags().GetBool(quietFlagName); err == nil {
				if f.Changed {
					return b
				}
				if b {
					return true
				}
			}
		}
	}
	return quietEnvSet()
}

// quietEnvSet reads LOGMIND_QUIET and reports whether it opts in. Falsey
// words ("", "0", "false", "no", "off") stay in the default; anything else
// enables quiet.
func quietEnvSet() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(quietEnvVar))) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// qout bundles a verb's stdout + stderr with the resolved quiet flag and
// gives every wired verb one uniform way to emit the three output classes.
// Threaded in place of raw io.Writer pairs so the quiet/non-quiet split
// lives in one place instead of being re-derived at each call.
type qout struct {
	stdout io.Writer
	stderr io.Writer
	quiet  bool
}

// newQout builds the writer bundle for a verb.
func newQout(quiet bool, stdout, stderr io.Writer) qout {
	return qout{stdout: stdout, stderr: stderr, quiet: quiet}
}

// chat writes a human-facing progress/chatter line to stdout. Suppressed
// entirely under QUIET (that is the whole point); in the default mode it is
// a plain Fprintf to stdout, so byte-parity holds.
func (q qout) chat(format string, a ...any) {
	if q.quiet {
		return
	}
	fmt.Fprintf(q.stdout, format, a...)
}

// ok writes the single chainable `ok <k=v ...>` summary line to stdout. The
// caller passes the trailer WITHOUT the leading "ok " and WITHOUT the
// newline. Always emitted — it is the machine receipt an agent chains on.
func (q qout) ok(format string, a ...any) {
	fmt.Fprintf(q.stdout, "ok "+format+"\n", a...)
}

// fail writes a user-facing error/diagnostic line. In the default mode it
// preserves the historical stdout destination (several logmind verbs print
// their "Error: …" diagnostics to stdout — matching the Python CLI — and
// the goldens/tests pin that). Under QUIET it routes to stderr instead, so
// quiet stdout carries ONLY the ok summary (or nothing, on error) and the
// diagnostic is never swallowed.
func (q qout) fail(format string, a ...any) {
	w := q.stdout
	if q.quiet {
		w = q.stderr
	}
	fmt.Fprintf(w, format, a...)
}
