// Package cli wires the cobra command tree for the logmind binary.
//
// Wave B1.scaffold only ships the root command + the version subcommand.
// Subsequent waves register additional subcommands here (init, log, show,
// search, ...) by calling rootCmd.AddCommand(...) from their own files.
package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/version"
)

// versionLine returns the canonical `--version` output. Centralised so
// the version subcommand and the cobra root's --version flag both emit
// byte-identical text — the snapshot test pins this exact string.
//
// Format: `logmind <version> (spec <spec-version>)`
//
// This is the protocol contract — downstream tooling (clud-bug,
// tokenomics, agent-skills) greps this line to detect skew. Don't
// change the format without bumping SpecVersion.
func versionLine() string {
	return fmt.Sprintf("logmind %s (spec %s)", version.Version, version.SpecVersion)
}

// NewRootCmd returns a fresh cobra root command. Each call constructs
// an independent tree so tests can exercise the CLI without leaking
// flag state between cases.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "logmind",
		Short: "Decision logging for AI-assisted development",
		Long: "logmind captures architectural and implementation decisions\n" +
			"as they happen, attaches them to the relevant git branch, and\n" +
			"surfaces them to the next human or AI working in the repo.",
		// Disable cobra's auto-generated default --version output so we
		// can render the protocol-contract format ourselves.
		SilenceUsage: true,
		// Don't let cobra re-print errors after the subcommand RunE
		// returned — we already wrote the user-facing message to
		// stdout from inside the RunE (Python pattern). Cobra would
		// otherwise duplicate it to stderr, breaking byte-identical
		// parity with the Python CLI.
		SilenceErrors: true,
	}

	// Persistent --quiet flag (token-killer Phase 1b). Inherited by every
	// subcommand; wired verbs (log, timeline, file-structure, doctor,
	// headline) then suppress progress chatter and emit a single chainable
	// `ok <k=v>` line. Opt-in twin of the LOGMIND_QUIET env var — the default
	// (unset) path stays byte-identical. See quiet.go.
	// NB: no back-quotes in this usage string — pflag's UnquoteUsage treats a
	// back-quoted span as the flag's value placeholder, which would make this
	// boolean flag render misleadingly as `--quiet ok <k=v>` in --help.
	root.PersistentFlags().Bool(quietFlagName, false,
		"Terse machine output: suppress progress chatter, emit one chainable 'ok <k=v>' line per verb (env: LOGMIND_QUIET=1). Errors still go to stderr.")

	root.AddCommand(newVersionCmd())
	// B2: git integration + hooks subcommands.
	root.AddCommand(newInstallHookCmd())
	root.AddCommand(newCheckDecisionsCmd())
	root.AddCommand(newCheckLinksCmd())
	// B3: derived doc generators + rebase wrapper.
	root.AddCommand(newTimelineCmd())
	root.AddCommand(newFileStructureCmd())
	root.AddCommand(newContextCmd())
	root.AddCommand(newTreeCmd())
	root.AddCommand(newRebaseCmd())
	// B4: agent file templating subcommand tree.
	root.AddCommand(newAgentsCmd())
	// B5: skill authoring/validation/bench/audit/suggest tree.
	root.AddCommand(newSkillCmd())
	// B5b / G4.a: loop-closer that folds clud-bug review citations
	// into each cited skill's PROVENANCE.md. See SPEC §3.9 + §6.
	root.AddCommand(newSyncCmd())
	// B6: config + doctor + init + self-update.
	root.AddCommand(newConfigCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newInitCmd())
	root.AddCommand(newSelfUpdateCmd())
	// v1.2.0 (plan §8.7 — deferred from Phase B3): `logmind log` ports
	// the Python shim's decision-logging primitive into Go and bolts
	// Layer 1 of markdown self-healing on top (linkcheck-driven
	// interactive retry loop). See internal/cli/log.go.
	root.AddCommand(newLogCmd())
	// Slice 2: set the branch's one-sentence timeline headline (the
	// main-canonical summary line). Companion to `logmind log --headline`.
	root.AddCommand(newHeadlineCmd())

	// Top-level --version flag mirrors `logmind version` so both
	// `logmind --version` and `logmind version` produce the same line.
	// This matches the Python click ergonomics users are used to.
	root.SetVersionTemplate("{{.Version}}\n")
	root.Version = versionLine()

	return root
}

// Execute runs the root command and returns its exit error. The main
// package wraps this so production callers get the cobra exit-code
// behaviour while tests can drive the tree in-process.
func Execute() error {
	return NewRootCmd().Execute()
}

// newVersionCmd builds the `logmind version` subcommand.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the logmind binary version and protocol spec version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return printVersion(cmd.OutOrStdout())
		},
	}
}

func printVersion(w io.Writer) error {
	_, err := fmt.Fprintln(w, versionLine())
	return err
}
