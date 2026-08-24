// auto.go — `logmind auto <profile>` subcommand.
//
// Thin cobra shim over internal/auto: resolve the profile (refusing an
// unknown one by name), apply the setup, and render the result. Every
// decision — what a profile is, what the directive says, what may be
// overwritten — lives in internal/auto so `logmind doctor`'s drift nudge
// reads the same source.
//
// The one rule this file exists to hold: `auto` PRINTS the handover, it
// never performs it. `unattended-operation` starts only from an explicit
// human handover and is never inferred from a clock, a wake, or a tool —
// so a setup command that started the mode would break the policy it had
// just installed. Nothing here spawns a process.
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/auto"
)

func newAutoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "auto <profile>",
		Short: "Set a repository up for unattended agent operation (prints the handover; never starts it)",
		Long: "Set this repository up for a named autonomy profile: write the\n" +
			"standing directive (.logmind/auto.yml), report which required\n" +
			"skills are installed, and print the handover a human must give\n" +
			"to start the mode.\n\n" +
			"`auto` never starts unattended operation. The mode begins only\n" +
			"from an explicit human handover — never from a clock, a\n" +
			"scheduled wake, or a tool.\n\n" +
			"Profiles:\n" + autoProfileHelp(),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runAuto(cmd, cwd, args[0])
		},
	}
}

// autoProfileHelp renders the profile registry for --help. Generated from
// the registry rather than hand-listed, so adding a profile cannot leave
// the help text quietly wrong.
func autoProfileHelp() string {
	var b strings.Builder
	for _, p := range auto.Profiles() {
		fmt.Fprintf(&b, "  %-12s %s\n", p.Name, p.Summary)
	}
	return strings.TrimRight(b.String(), "\n")
}

func runAuto(cmd *cobra.Command, cwd, profileName string) error {
	q := newQout(quietEnabled(cmd), cmd.OutOrStdout(), cmd.ErrOrStderr())

	profile, ok := auto.Lookup(profileName)
	if !ok {
		// REFUSE and name what is known. Never fall back to a default:
		// a setup verb that quietly configured the wrong autonomy policy
		// is the same defect as #267 and #286 in another guise. Errors go
		// to stderr per SPEC §2.7 (and no `ok` receipt is printed, since
		// this exits non-zero).
		stderr := cmd.ErrOrStderr()
		fmt.Fprintf(stderr, "Error: unknown profile %q. Known profiles: %s\n",
			profileName, strings.Join(auto.Names(), ", "))
		if note := auto.RetiredNote(profileName); note != "" {
			fmt.Fprintln(stderr, "note:", note)
		}
		return ErrSilent
	}

	res, err := auto.Apply(cwd, profile)
	if err != nil {
		return fmt.Errorf("auto %s: %w", profile.Name, err)
	}

	q.chat("Setting up %s (profile `%s`) for this repository...\n\n", profile.Mode, profile.Name)
	reportAutoDirective(q, profile, res)
	reportAutoSkills(q, profile, res)
	q.chat("\n%s\n", autoHandoverBlock(profile, res))

	q.ok("auto profile=%s directive=%s checkpoint=%s skills-missing=%d",
		profile.Name, res.Outcome, res.Checkpoint, len(res.SkillsMissing))
	return nil
}

// reportAutoDirective renders what Apply did with `.logmind/auto.yml`.
//
// Every declined outcome is REPORTED, never silent — the file was left
// alone because it carries policy a human authored, and an operator who
// is not told that will assume the setup they just ran is in force.
// Refusals go to stderr for the same reason reportTemplateDowngrades
// puts them there: they survive `--quiet` and any stdout redirect.
func reportAutoDirective(q qout, p auto.Profile, res auto.Result) {
	// q.stderr, not q.fail: a refusal must land on stderr in BOTH modes.
	// q.fail keeps the historical stdout destination in the default mode
	// for the verbs with Python parity to preserve; `auto` has none.
	stderr := q.stderr
	switch res.Outcome {
	case auto.Created:
		q.chat("✓ Created %s — the standing directive (profile: %s, %s)\n",
			res.Path, p.Name, res.Bundled)
	case auto.Current:
		q.chat("  %s already current (profile: %s, %s) — left unchanged.\n",
			res.Path, p.Name, res.Installed)
	case auto.DeclinedStale:
		fmt.Fprintf(stderr,
			"note: %s is at %s; this binary bundles %s — left unchanged, because it carries policy you "+
				"authored (repo hard stops, the wake mechanism). Move it aside and re-run `logmind auto %s` "+
				"to take the new directive, then re-apply your repo-specific slots.\n",
			res.Path, res.Installed, res.Bundled, p.Name)
	case auto.DeclinedNewer:
		fmt.Fprintf(stderr,
			"note: %s left unchanged — installed directive %s is NEWER than the %s this binary bundles; "+
				"refusing to downgrade. Upgrade logmind to move it forward.\n",
			res.Path, res.Installed, res.Bundled)
	case auto.DeclinedMarkerless:
		fmt.Fprintf(stderr,
			"note: %s carries no `# logmind-auto-version:` marker, so it belongs to you — left unchanged. "+
				"Move it aside and re-run `logmind auto %s` if you want the bundled directive.\n",
			res.Path, p.Name)
	case auto.DeclinedOtherProfile:
		fmt.Fprintf(stderr,
			"note: %s is a directive for profile %q — refusing to overwrite it with %q. Move it aside first.\n",
			res.Path, res.InstalledProfile, p.Name)
	}

	q.chat("    checkpoint: %s\n", res.Checkpoint)
	if !res.CheckpointExists {
		// Report, never create: what a plan doc should SAY is a judgment
		// call, the same boundary `doctor --fix` draws for docs/spec.md.
		fmt.Fprintf(stderr,
			"note: %s does not exist yet — the checkpoint has nowhere to land, and a resumed session "+
				"cannot tell paused from died. Create it before entry.\n", res.Checkpoint)
	}
}

// reportAutoSkills lists the profile's required skills and, for any that
// are absent, the command that installs it — printed for the operator to
// run, not run here. logmind does not fetch catalog items; see
// internal/auto's package comment for why.
func reportAutoSkills(q qout, p auto.Profile, res auto.Result) {
	q.chat("\nSkills this profile's policy depends on (load order):\n")
	present := map[string]bool{}
	for _, name := range res.SkillsPresent {
		present[name] = true
	}
	for _, name := range p.Skills {
		if present[name] {
			q.chat("  ✓ %s\n", name)
			continue
		}
		q.chat("  ✗ %s — not installed under .claude/skills/\n", name)
		q.chat("      %s\n", auto.InstallCommand(name))
	}
}

// autoHandoverBlock renders the handover a human pastes to start the
// mode. The five slots are exactly the ones `unattended-operation` says
// a directive must name; the sixth line is the pre-entry check for the
// silent failure that same skill names (scheduler state committed as a
// project artifact).
//
// This is printed. It is never executed, and `auto` deliberately has no
// flag that would execute it.
func autoHandoverBlock(p auto.Profile, res auto.Result) string {
	return fmt.Sprintf(`logmind does NOT start %s, and will not: the mode begins only from an
explicit human handover — never from a clock, a scheduled wake, or a tool.
Start it yourself, naming all five:

  Run unattended until I'm back.
    scope:      <the named plan items or slice range — never "keep going">
    hard stops: <what ends a lane rather than being worked around>
    you may:    <pre-authorized exceptions, by name; anything unnamed is not authorized>
    wake:       <your harness's wake mechanism>; park work in %s
    at a fork:  park it and continue elsewhere

Before entry: confirm your wake mechanism's lock/schedule file is git-ignored.
Committing it is a named silent failure.`, p.Mode, res.Checkpoint)
}
