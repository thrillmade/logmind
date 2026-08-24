// blocking.go — SPEC §1.6's person-only settings: which keys they are, and
// which direction of write counts as weakening one. Nothing here knows who is
// asking; that question belongs to the command surface.
//
// This is the one owner of that fact. `logmind config set` refuses an
// agent-initiated weakening (internal/cli/config.go) and `logmind doctor`
// reports one that arrived by hand edit (internal/doctor); both read this
// table, so the set of protected keys and the direction that counts as
// weakening can never disagree between the refusal and the report.
package config

// BlockingSetting is one of the three settings SPEC §1.6 names as a person's
// to change: "A setting that decides whether something blocks is a person's
// to change. review.strict_mode, git.enforce_commits and review.auto_fix MUST
// NOT be written by an agent — not through the command, not by editing the
// file."
//
// Every OTHER setting is deliberately absent. §1.6's next paragraph is just
// as binding — "Every other setting an agent MAY write through the command,
// because setup and registering a skill it just wrote are legitimate work and
// blocking them helps nobody" — so growing this table is a SPEC change, not a
// judgement call.
type BlockingSetting struct {
	// Key is the dotted key as typed on the command line.
	Key string
	// File is where §1.6 puts the setting. Named in the refusal so a
	// person knows which file to open if they choose to.
	File string
	// Governs is a one-clause summary of what turns off, for the message.
	Governs string
	// PersonInLoop is the value that keeps a person in the loop. It is the
	// anchor for "weaken": §1.6 says a tool asked to WEAKEN one of these
	// must refuse, and the uniform reading of weaken across all three is
	// "moves the setting away from the value that keeps a person in the
	// loop".
	//
	//   git.enforce_commits  true  — a substantive commit carries a decision
	//   review.strict_mode   true  — a critical finding blocks the merge
	//   review.auto_fix      false — a person, not a bot, pushes the fix
	//
	// Writing PersonInLoop is STRENGTHENING and stays agent-writable: the
	// refusal exists to stop oversight being removed, and refusing
	// `enforce_commits true` would block the setup §1.6 protects.
	PersonInLoop any
}

// blockingSettings is the table. Package-level and returned by value through
// BlockingSettings() so no caller can scribble on it.
var blockingSettings = []BlockingSetting{
	{
		Key:          "git.enforce_commits",
		File:         ".logmind/config.yml",
		Governs:      "whether a substantive commit must carry a decision",
		PersonInLoop: true,
	},
	{
		Key:          "review.strict_mode",
		File:         ".claude/skills/.clud-bug.json",
		Governs:      "whether a critical review finding blocks the merge",
		PersonInLoop: true,
	},
	{
		Key:          "review.auto_fix",
		File:         ".claude/skills/.clud-bug.json",
		Governs:      "whether a reviewer may push a fix without a person",
		PersonInLoop: false,
	},
}

// BlockingSettings returns the §1.6 table in a fresh slice.
func BlockingSettings() []BlockingSetting {
	out := make([]BlockingSetting, len(blockingSettings))
	copy(out, blockingSettings)
	return out
}

// LookupBlocking returns the blocking setting for an exact dotted key.
//
// Exact match, never a prefix: `git.enforce_commits.x` and `enforce_commits`
// are different keys and neither is the protected one. A prefix match would
// over-block, which §1.6 calls out as the worse failure.
func LookupBlocking(key string) (BlockingSetting, bool) {
	for _, b := range blockingSettings {
		if b.Key == key {
			return b, true
		}
	}
	return BlockingSetting{}, false
}

// Weakens reports whether writing v to this setting moves it away from
// PersonInLoop — the direction §1.6 requires a tool to refuse.
//
// Anything that is not the PersonInLoop boolean weakens, including a non-bool
// (`config set git.enforce_commits 0` coerces to int). That is deliberate and
// is not merely a fallback: a non-bool in one of these keys makes the typed
// load fail, and LoadPath answers a failed unmarshal with the whole DEFAULT
// config — so a stray `0` silently reverts every other setting in the file
// too. Refusing it is right for both reasons.
//
// The comparison is bool-only rather than `==` on `any`, because `==` panics
// on an uncomparable dynamic type (a coerced list would take down the
// command it was supposed to refuse).
func (b BlockingSetting) Weakens(v any) bool {
	want, ok := b.PersonInLoop.(bool)
	if !ok {
		// Unreachable with today's table; a non-bool PersonInLoop would
		// need its own comparison, so refuse rather than guess.
		return true
	}
	got, ok := v.(bool)
	if !ok {
		return true
	}
	return got != want
}
