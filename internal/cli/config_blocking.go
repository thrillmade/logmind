// config_blocking.go — SPEC §1.6's refusal, and the rule logmind uses to tell
// a person from an agent.
//
// §1.6: "A setting that decides whether something blocks is a person's to
// change. review.strict_mode, git.enforce_commits and review.auto_fix MUST NOT
// be written by an agent — not through the command, not by editing the file.
// A tool asked to weaken one of these by an agent MUST refuse and say why."
//
// WHICH keys and WHICH direction is config.BlockingSettings — one owner, read
// by this refusal and by doctor's hand-edit advisory alike. This file owns
// only the second question: is a person asking?
//
// # How logmind tells a person from an agent
//
// A weakening write is permitted only when a person types the key back. Two
// things have to be true before logmind will even ask:
//
//  1. No agent marker in the environment (agentEnvMarkers below). A marker is
//     a positive identification, so it refuses outright — even at a TTY.
//  2. stderr is a terminal AND stdin can block-and-wait (the same isatty /
//     stdinReadable pair `logmind log`'s prompts use). An agent driving
//     logmind as a subprocess over pipes cannot satisfy this, whatever
//     harness it is, whether or not logmind has heard of it.
//
// (2) is the guard. (1) is a courtesy: it produces a message that names the
// reason, and it catches an agent that happens to run inside a PTY. The
// marker list is therefore NON-EXHAUSTIVE ON PURPOSE and does not need to
// keep up with every harness — nothing rests on it being complete.
//
// # What this does not do
//
// It is a speed bump and a signal, not a boundary. An agent with a shell can
// defeat it: unset the marker AND allocate a PTY AND answer the prompt, or
// skip all of that and edit .logmind/config.yml directly — which §1.6
// addresses to the agent precisely because no local tool can prevent it.
// `logmind doctor` reports a blocking setting it finds already weakened
// (internal/doctor collectGateAdvisories), and SPEC §6.3 keeps a weakening
// inside a pull request from affecting the gate judging it. Those are the
// backstops; this refusal is the front door, and it is honest about being
// only the front door.
package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thrillmade/logmind/internal/config"
)

// agentEnvMarkers are environment variables whose presence identifies an
// agent harness driving this command.
//
// Non-exhaustive by construction — see the file comment. LOGMIND_AGENT is
// first because it is the one any harness can set for itself without logmind
// shipping a new release to recognise it.
var agentEnvMarkers = []string{
	"LOGMIND_AGENT", // any harness may self-declare
	"CLAUDECODE",    // Claude Code
	"CLAUDE_CODE",   // Claude Code, alternate spelling
	"CURSOR_AGENT",  // Cursor agent mode
	"CODEX_SANDBOX", // OpenAI Codex CLI
}

// detectAgentMarker returns the first marker set to a non-empty value.
//
// Non-empty, not merely present: a wrapper that exports `CLAUDECODE=` with no
// value has said nothing, and reading that as "an agent is here" would refuse
// a person's write for no reason. An agent that wants to hide unsets the
// variable anyway, so presence-only buys no strength.
func detectAgentMarker() (string, bool) {
	for _, name := range agentEnvMarkers {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			return name, true
		}
	}
	return "", false
}

// guardBlockingWrite refuses an agent-initiated weakening of a §1.6 blocking
// setting, and returns nil for everything else.
//
// Everything else is most of the surface, and deliberately so: §1.6's next
// paragraph — "Every other setting an agent MAY write through the command,
// because setup and registering a skill it just wrote are legitimate work and
// blocking them helps nobody" — makes over-blocking the worse failure. The
// two early returns below are what keep that true: an unprotected key and a
// STRENGTHENING write both leave immediately.
func guardBlockingWrite(cmd *cobra.Command, key string, parsed any) error {
	setting, ok := config.LookupBlocking(key)
	if !ok {
		return nil
	}
	if !setting.Weakens(parsed) {
		return nil
	}

	errOut := cmd.ErrOrStderr()
	if marker, isAgent := detectAgentMarker(); isAgent {
		writeBlockingRefusal(errOut, setting,
			"$"+marker+" is set in this environment, so logmind reads this write as agent-initiated")
		return ErrSilent
	}
	if !isTerminalFunc() {
		writeBlockingRefusal(errOut, setting,
			"stderr is not a terminal, so logmind cannot put this question to a person")
		return ErrSilent
	}
	if !stdinReadable(cmd.InOrStdin()) {
		writeBlockingRefusal(errOut, setting,
			"stdin is a pipe, so logmind cannot put this question to a person")
		return ErrSilent
	}

	// A person is plausibly here. Ask them to prove it by retyping the key.
	// Retyping the KEY, not "y": an agent answering prompts by piping "y"
	// (which `logmind agents remove` accepts, by design, for a reversible
	// action) does not get this one, and a person who typed the command
	// three seconds ago can type the key without thinking twice.
	fmt.Fprintf(errOut, "About to write %s.\n", setting.Key)
	fmt.Fprint(errOut, blockingRefusalBody(setting,
		"logmind sees a terminal and no agent marker, so it is asking rather\n  than refusing"))
	fmt.Fprintf(errOut, "\n  Retype the key to confirm (%s): ", setting.Key)
	if !confirmTypedKey(cmd.InOrStdin(), setting.Key) {
		fmt.Fprintf(errOut, "\nNot confirmed — %s was not written.\n", setting.Key)
		return ErrSilent
	}
	return nil
}

// writeBlockingRefusal emits the refusal: what was refused, what it governs,
// who may change it and how, and why this particular invocation did not
// qualify. §1.6 requires the refusal to "say why"; naming only the rule and
// not the reason leaves the caller guessing which of the two conditions it
// failed.
func writeBlockingRefusal(w io.Writer, setting config.BlockingSetting, reason string) {
	fmt.Fprintf(w, "Refusing to write %s.\n", setting.Key)
	fmt.Fprint(w, blockingRefusalBody(setting, reason))
}

func blockingRefusalBody(setting config.BlockingSetting, reason string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n  It governs %s\n", setting.Governs)
	fmt.Fprintf(&b, "  (%s), and SPEC §1.6 makes a setting that decides\n", setting.File)
	fmt.Fprintf(&b, "  whether something blocks a person's to change.\n")
	fmt.Fprintf(&b, "\n  Who may change it: a person, running this same command at a terminal —\n")
	fmt.Fprintf(&b, "  logmind asks them to retype the key before it writes.\n")
	fmt.Fprintf(&b, "  Here: %s.\n", reason)
	return b.String()
}

// confirmTypedKey reads one line and returns true only when it is exactly the
// key, ignoring surrounding whitespace. Case-sensitive: config keys are
// snake_case (§1.6) and there is no near-miss worth accepting on a setting
// this one guards.
func confirmTypedKey(stdin io.Reader, key string) bool {
	if stdin == nil {
		return false
	}
	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		return false
	}
	return strings.TrimSpace(scanner.Text()) == key
}
